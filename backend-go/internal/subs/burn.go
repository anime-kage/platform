package subs

// Burning the RO track into a copy of the video.
//
// This is the delivery mechanism for embed-only hosts. A soft track can only be
// shown by a player we control, and an <iframe> from a third-party host is not
// one — so for those sources the subtitle has to be in the pixels or it is not
// seen at all.
//
// Unlike MuxSoftSub and RemuxMP4, which are stream copies and take seconds, this
// re-encodes video and is the most expensive thing this codebase does. Measured
// on the production box (4 aarch64 cores, no GPU): 1080p24, CRF 20, at the
// default veryfast+animation → ~2.8x realtime, i.e. ~9 minutes for a 24-minute
// episode with all four cores busy. It must therefore be a queued background job, never a request, and
// it is niced so the API stays answerable while it runs.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// BurnOptions are the encode knobs. Zero values are filled with the defaults
// that were actually measured, so callers can pass BurnOptions{}.
type BurnOptions struct {
	// CRF is the quality target: lower is better and bigger. 20 is a
	// reasonable floor for anime, which shows banding in gradients before it
	// shows blocking.
	CRF int
	// Preset trades encode time for size. Anything slower than "medium" is not
	// worth it on this hardware.
	Preset string
	// Tune biases x264's decisions towards a content type. "animation" is the
	// one that matters here and is worth more than it looks: measured on
	// anime-like content it made the output *2.1x smaller* than `medium` while
	// still encoding 1.39x faster, because it raises deblocking and adjusts
	// psy-RD for flat cel-shaded areas and hard line art. Empty = no tune.
	Tune string
	// Nice is the scheduler priority the encoder runs at (0..19, higher = more
	// willing to yield). This is the whole CPU-sharing story and it is
	// adaptive for free: the kernel hands x264 every idle core when the site
	// is quiet, and takes them back the moment the API or SSR want to run.
	// A fixed percentage cap could not do that — it would throttle the encode
	// at 3am for no reason and still not guarantee headroom at peak.
	Nice int
	// Threads caps how many cores x264 may occupy at once. Nice governs who
	// wins a contended core; this governs how many it can contend for at all,
	// which is what keeps the box responsive rather than merely fair. 0 lets
	// ffmpeg decide (one thread per core).
	Threads int
}

// Burn writes videoPath with assPath rendered into the picture, to outPath.
//
// onProgress, if non-nil, is called with a 0..1 fraction as the encode advances.
// It is derived from ffmpeg's own -progress stream against the source duration,
// so it reflects encoded time rather than elapsed wall clock.
func Burn(ctx context.Context, videoPath, assPath, outPath string, opt BurnOptions, onProgress func(float64)) error {
	if opt.CRF == 0 {
		opt.CRF = 20
	}
	if opt.Preset == "" {
		opt.Preset = "medium"
	}

	totalMs, err := durationMs(ctx, videoPath)
	if err != nil {
		// Not fatal: without a duration there is no percentage, but the burn is
		// still perfectly runnable. A progress bar is worth less than the file.
		totalMs = 0
	}

	// ass=<path> rather than subtitles=<path>: the styling is already declared in
	// the ASS file (see ass.go), and the ass filter applies it verbatim instead
	// of re-deriving defaults.
	//
	// Filter paths need escaping — a colon inside the filter argument separates
	// filter options, so an unescaped Windows-ish or odd path silently becomes a
	// different option. Staging paths are ours and contain neither, but escaping
	// costs nothing and removes the question.
	filter := "ass=" + escapeFilterPath(assPath)

	if opt.Nice == 0 {
		opt.Nice = 19
	}
	args := []string{
		"-n", strconv.Itoa(opt.Nice), // the API shares these cores
		"ffmpeg", "-nostdin", "-v", "error", "-y",
		"-i", videoPath,
		"-vf", filter,
		// First video and (optional) first audio only. -sn drops the source's
		// own subtitle tracks: they are now in the picture, and a fansub MKV
		// often carries a dozen more that have no business in the output.
		"-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn",
		"-c:v", "libx264", "-preset", opt.Preset, "-crf", strconv.Itoa(opt.CRF),
	}
	if opt.Threads > 0 {
		args = append(args, "-threads", strconv.Itoa(opt.Threads))
	}

	// Tune goes here, next to the other -c:v options, and is appended rather than
	// spliced into the middle of the slice.
	//
	// The first attempt spliced it in before "the last two elements", but the tail
	// is three — "-f", "mp4", outPath — so it landed between -f and its value.
	// ffmpeg then read the output format as "-tune" and the output filename as
	// "animation", and every burn failed with "Requested output format '-tune' is
	// not known". Building the list in order costs nothing and cannot be off by
	// one.
	if opt.Tune != "" {
		args = append(args, "-tune", opt.Tune)
	}

	args = append(args,
		// yuv420p because sources are frequently 10-bit and most host players and
		// browsers cannot decode 10-bit H.264. Skipping this produces a file that
		// plays perfectly on the encoding machine and nowhere else.
		"-pix_fmt", "yuv420p",
		// AAC rather than copy: MP4 cannot carry FLAC or some of what an MKV
		// source may hold, and audio re-encoding is cheap next to the video.
		"-c:a", "aac", "-b:a", "192k",
		"-movflags", "+faststart",
		"-progress", "pipe:1", "-nostats",
		"-f", "mp4", outPath,
	)

	cmd := exec.CommandContext(ctx, "nice", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg burn pipe: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg burn start: %w", err)
	}
	readProgress(stdout, totalMs, onProgress)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg burn: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if onProgress != nil {
		onProgress(1)
	}
	return nil
}

// readProgress consumes ffmpeg's -progress stream (key=value lines) and reports
// out_time_us against the total. It drains to EOF regardless of onProgress being
// nil, because leaving the pipe unread would eventually block ffmpeg.
func readProgress(r io.Reader, totalMs int, onProgress func(float64)) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if onProgress == nil || totalMs <= 0 {
			continue
		}
		v, ok := strings.CutPrefix(line, "out_time_us=")
		if !ok {
			continue
		}
		us, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			continue
		}
		f := float64(us/1000) / float64(totalMs)
		if f < 0 {
			f = 0
		}
		// Never report 1 from here: completion is the process exiting cleanly,
		// not the last progress line, and a bar that hits 100% while the file is
		// still being finalised reads as a hang.
		if f > 0.99 {
			f = 0.99
		}
		onProgress(f)
	}
}

// durationMs asks ffprobe how long the source is, for the progress denominator.
func durationMs(ctx context.Context, path string) (int, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-show_entries", "format=duration", "-of", "default=nw=1:nk=1", path)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe duration: %w", err)
	}
	secs, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", strings.TrimSpace(string(out)), err)
	}
	return int(secs * 1000), nil
}

// escapeFilterPath quotes a path for use inside an ffmpeg filter argument.
func escapeFilterPath(p string) string {
	p = strings.ReplaceAll(p, `\`, `\\`)
	p = strings.ReplaceAll(p, ":", `\:`)
	p = strings.ReplaceAll(p, "'", `\'`)
	return p
}
