package subs

// Embedded-subtitle extraction. When a translator uploads a
// video without a separate subtitle file, we lift a soft-sub track straight
// out of the container with ffmpeg so they don't have to demux it by hand.
// Only text tracks can be read — hard-subbed (burned-in) video or image-based
// subs (PGS/VobSub) carry no extractable text.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// textSubCodecs are the ffmpeg subtitle codecs we can transcode to SRT text.
var textSubCodecs = map[string]bool{
	"subrip": true, "srt": true, "ass": true, "ssa": true,
	"mov_text": true, "webvtt": true, "text": true,
}

type probeStream struct {
	Index     int    `json:"index"`
	CodecName string `json:"codec_name"`
	Tags      struct {
		Language string `json:"language"`
	} `json:"tags"`
}

// HasFFmpeg reports whether ffmpeg and ffprobe are both on PATH, so callers can
// tell "no embedded track" from "extraction isn't available here".
func HasFFmpeg() bool {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return false
	}
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// ExtractEmbedded pulls the best embedded text subtitle out of a video file and
// parses it into events, preferring an English track. Returns (nil, nil) when
// the file has no extractable subtitle stream (a normal outcome, not an error).
func ExtractEmbedded(ctx context.Context, videoPath string) ([]Event, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	streams, err := probeSubs(ctx, videoPath)
	if err != nil {
		return nil, err
	}
	idx, ok := pickStream(streams)
	if !ok {
		return nil, nil
	}
	out, err := extractStream(ctx, videoPath, idx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil, nil
	}
	return ParseSRT(out)
}

func probeSubs(ctx context.Context, path string) ([]probeStream, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-select_streams", "s",
		"-show_entries", "stream=index,codec_name:stream_tags=language",
		"-of", "json", path).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	var parsed struct {
		Streams []probeStream `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("ffprobe json: %w", err)
	}
	return parsed.Streams, nil
}

// pickStream chooses the absolute stream index to extract: an English text
// track first, else the first text track. Image-based subs are skipped.
func pickStream(streams []probeStream) (int, bool) {
	best := -1
	for _, s := range streams {
		if !textSubCodecs[s.CodecName] {
			continue
		}
		switch strings.ToLower(s.Tags.Language) {
		case "eng", "en":
			return s.Index, true
		}
		if best == -1 {
			best = s.Index
		}
	}
	if best == -1 {
		return 0, false
	}
	return best, true
}

// MuxSoftSub remuxes videoPath with the SRT at srtPath into Matroska on w,
// adding the subtitle as a soft "ron" track — no re-encode, so it's fast even
// for a full episode. The result is one self-contained file the coordinator can
// upload to a host. Requires ffmpeg.
func MuxSoftSub(ctx context.Context, videoPath, srtPath string, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error",
		"-i", videoPath, "-i", srtPath,
		"-map", "0:v?", "-map", "0:a?", "-map", "1:0",
		"-c", "copy", "-c:s", "srt",
		"-metadata:s:s:0", "language=ron", "-disposition:s:0", "default",
		"-f", "matroska", "pipe:1")
	cmd.Stdout = w
	return cmd.Run()
}

// RemuxMP4 rewraps videoPath's H.264/AAC streams into MP4 at outPath — stream
// copy, no re-encode, so a full episode takes seconds. Subtitles are left out
// deliberately: this file goes to a host, and our own player gets the RO track
// from us (that is the whole point of an extract source).
//
// Why MP4 and not the Matroska from MuxSoftSub: hosts that don't transcode
// serve back exactly the container you gave them, and browsers do not play
// Matroska — Firefox refuses outright, Chrome is unreliable with H.264 in MKV.
//
// It writes a file rather than a pipe because +faststart moves the moov atom to
// the front in a second pass, which needs a seekable output. Without it the
// player must download the whole file before it can start, and seeking breaks.
func RemuxMP4(ctx context.Context, videoPath, outPath string) error {
	// -sn/-dn make the intent explicit: a fansub MKV typically carries a dozen
	// ASS tracks and as many font attachments, none of which belong here.
	//
	// The output still shows a third stream, `bin_data` with handler
	// "SubtitleHandler" — that is the source's *chapters*, which MP4 stores as
	// a QuickTime text track (one frame per chapter). Browsers ignore it. Don't
	// "fix" it with -map_chapters -1: on a fansub release those chapters mark
	// the intro and the credits, which is exactly what the skip marks want.
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y",
		"-i", videoPath,
		"-map", "0:v:0", "-map", "0:a?", "-sn", "-dn",
		"-c", "copy", "-movflags", "+faststart",
		"-f", "mp4", outPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg remux mp4: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// extractStream transcodes one subtitle stream to SRT on stdout.
func extractStream(ctx context.Context, path string, streamIdx int) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error",
		"-i", path, "-map", fmt.Sprintf("0:%d", streamIdx),
		"-f", "srt", "pipe:1").Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg extract: %w", err)
	}
	return out, nil
}
