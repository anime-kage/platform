package subs

// Rewrapping an uploaded MKV as MP4.
//
// Most of what translators upload is Matroska, and no browser will play it:
// Firefox refuses the container outright and Chrome is unreliable with H.264 in
// MKV. That breaks the preview on the translator page, and the preview is how
// they check they uploaded the right episode at all.
//
// The fix is a rewrap, not a re-encode. The video stream is copied bit for bit,
// so this costs seconds-to-minutes of I/O rather than the ~9 minutes of CPU a
// burn costs — the expensive part is moving the bytes, not compressing them.
// Nothing is lost in the swap: the EN subtitle is lifted out of the MKV into
// subtitle_events at ingest, before this runs, and the RO track is ours.

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// mp4AudioOK is the audio this rewrap can copy straight through.
//
// Deliberately narrow. MP4 can legally carry AC-3, E-AC-3 and ALAC, but no
// browser decodes them from an MP4 — and a "converted" file that still doesn't
// play in the preview would defeat the entire point of converting it. Anything
// outside this set is transcoded to AAC, which is cheap next to the video.
var mp4AudioOK = map[string]bool{"aac": true, "mp3": true}

// RemuxToMP4 rewraps src into an MP4 at outPath, copying the video stream.
//
// src may be a local path or an https URL — ffmpeg reads range requests, so a
// presigned R2 URL works as an input unchanged and the source never has to be
// downloaded to disk first.
//
// onProgress, if non-nil, gets a 0..1 fraction from ffmpeg's own -progress
// stream. A rewrap is fast, but "fast" on a 1.5 GB file pulled from a bucket is
// still minutes, and an unmoving bar reads as a hang.
func RemuxToMP4(ctx context.Context, src, outPath string, onProgress func(float64)) error {
	totalMs, err := durationMs(ctx, src)
	if err != nil {
		// No denominator means no percentage, which is worth less than the file.
		totalMs = 0
	}

	audio := "copy"
	if codec, cerr := probeAudioCodec(ctx, src); cerr != nil || !mp4AudioOK[codec] {
		audio = "aac"
	}

	args := []string{
		"-n", "10", // nice: the API shares these four cores
		"ffmpeg", "-nostdin", "-v", "error", "-y",
		"-i", src,
		// First video and (optional) first audio. -sn drops the source's own
		// subtitle tracks — a fansub MKV typically carries a dozen ASS tracks and
		// as many font attachments, none of which belong in the preview copy, and
		// the one we care about is already in subtitle_events.
		"-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn",
		"-c:v", "copy",
	}
	if audio == "copy" {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	}
	args = append(args,
		// +faststart moves the moov atom to the front in a second pass. Without it
		// the player must fetch the whole file before it can start and seeking
		// breaks — which is exactly the preview experience we are here to fix.
		"-movflags", "+faststart",
		"-progress", "pipe:1", "-nostats",
		"-f", "mp4", outPath,
	)

	cmd := exec.CommandContext(ctx, "nice", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg remux pipe: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg remux start: %w", err)
	}
	readProgress(stdout, totalMs, onProgress)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg remux: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if onProgress != nil {
		onProgress(1)
	}
	return nil
}

// probeAudioCodec reports the codec of the first audio stream, or "" when the
// source has no audio (in which case there is nothing to transcode and the
// caller's fallback to AAC is harmless — -map 0:a:0? simply matches nothing).
func probeAudioCodec(ctx context.Context, src string) (string, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-select_streams", "a:0", "-show_entries", "stream=codec_name",
		"-of", "json", src).Output()
	if err != nil {
		return "", fmt.Errorf("ffprobe audio: %w", err)
	}
	var parsed struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return "", fmt.Errorf("ffprobe audio json: %w", err)
	}
	if len(parsed.Streams) == 0 {
		return "", nil
	}
	return strings.ToLower(parsed.Streams[0].CodecName), nil
}
