package handler

// Direct-to-R2 video upload for the release pipeline.
//
// The browser PUTs the video straight into the bucket with a presigned URL, so
// the bytes never touch this server and never meet Cloudflare's 100 MiB proxy
// body limit. The release form then names the object and the video stays there
// for the life of the release.
//
// This replaced a chunked-upload path (sub-limit chunks appended to a file in
// staging, reassembled server-side). That existed only to squeeze a 3 GB upload
// through the edge body limit, and a presigned PUT sidesteps the limit entirely
// rather than working around it — so the chunking, the session directory, the
// 24h sweep and the resume bookkeeping are all gone. Faster, and several hundred
// lines less to be wrong.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/model"
)

const (

	// uploadReadBudget lifts the server's 30s ReadTimeout for the one handler
	// that still accepts a large body (the legacy multipart release POST).
	// Kept generous: the failure mode of being too tight is an upload that can
	// never finish.
	uploadReadBudget = 15 * time.Minute

	// presignTTL bounds how long a minted upload URL stays usable. Generous
	// enough for a multi-gigabyte PUT on a domestic uplink, short enough that a
	// leaked URL is a time-boxed capability rather than an open door.
	presignTTL = 2 * time.Hour
)

// that does not match is either a bug or someone probing for traversal.

// r2VideoPrefix namespaces ingest objects, and the uploader id in the path is
// load-bearing: it is what proves ownership when a release later claims the
// object. No database row is needed for that — the key itself carries it, and a
// key that does not start with the caller's own prefix is refused.
const r2VideoPrefix = "video-uploads"

var safeR2Key = regexp.MustCompile(`^video-uploads/[0-9]+/[a-f0-9]{32}\.[a-z0-9]{2,4}$`)

// POST /api/uploads/video/presign  (contentRole)
//
// Hands back a URL the browser PUTs the video straight to, bypassing this server
// entirely — and with it Cloudflare's 100 MiB body limit, which is what forced
// the chunked path below. Same guards as opening a chunked session: extension,
// size ceiling, and the release quota, because the cheapest place to refuse an
// upload is before it starts.
func (h *Handler) presignVideoUpload(w http.ResponseWriter, r *http.Request) {
	if h.storage == nil {
		httpx.Error(w, http.StatusServiceUnavailable,
			"Încărcarea directă nu e configurată (lipsesc variabilele R2_*).")
		return
	}
	var body struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "filename and size are required")
		return
	}
	ext := strings.ToLower(filepath.Ext(body.Filename))
	if !videoExts[ext] {
		httpx.Error(w, http.StatusBadRequest, "Video must be .mp4, .webm, .mkv or .m4v")
		return
	}
	if body.Size <= 0 || body.Size > maxVideoBytes {
		httpx.Error(w, http.StatusBadRequest, fmt.Sprintf(
			"Fișierul trebuie să fie între 1 byte și %d GB.", maxVideoBytes>>30))
		return
	}
	if used, limit, _, ok := h.releaseQuota(r); !ok {
		httpx.Error(w, http.StatusConflict, fmt.Sprintf(
			"Ai %d din %d episoade neterminate. Așteaptă să fie verificat și publicat unul "+
				"înainte de a urca altul — spațiul de lucru de pe server e limitat.", used, limit))
		return
	}

	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		httpx.Internal(w, "mint upload key", err)
		return
	}
	uploader := middleware.UserFrom(r).UserID
	key := fmt.Sprintf("%s/%d/%s%s", r2VideoPrefix, uploader, hex.EncodeToString(raw[:]), ext)

	url, err := h.storage.PresignPut(r.Context(), key, "application/octet-stream", presignTTL)
	if err != nil {
		httpx.Internal(w, "presign upload", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"key":       key,
		"url":       url,
		"expiresIn": int(presignTTL.Seconds()),
	}})
}

// videoSourceURL returns something ffmpeg (or a browser) can read the release's
// video from: a presigned GET when it lives in the bucket, else the local path.
//
// ffmpeg speaks https with range requests, which is what makes keeping the video
// in R2 workable at all — probing, subtitle extraction and the burn each pull
// only what they need instead of requiring a local copy.
func (h *Handler) videoSourceURL(ctx context.Context, rel *model.Release) (string, error) {
	if rel.R2Key != nil && *rel.R2Key != "" {
		if h.storage == nil {
			return "", errors.New("video is in R2 but R2 is not configured")
		}
		return h.storage.PresignGet(ctx, *rel.R2Key, presignTTL)
	}
	if rel.StagingPath == nil {
		return "", errors.New("release has no video")
	}
	return filepath.Join(h.cfg.StagingDir, filepath.FromSlash(*rel.StagingPath)), nil
}

// verifyR2Upload checks a direct-to-R2 upload without downloading it.
//
// Same two guards the copy path had — the key must sit under the caller's own
// prefix, and the object's length must match what the client declared — but the
// bytes stay in the bucket. That is the whole point: staging no longer has to
// hold every in-flight release, so the release cap stops being a disk limit.
func (h *Handler) verifyR2Upload(r *http.Request, key string, declared int64) error {
	if h.storage == nil {
		return errors.New("direct upload is not configured")
	}
	if !safeR2Key.MatchString(key) {
		return fmt.Errorf("invalid upload key")
	}
	uploader := middleware.UserFrom(r).UserID
	if !strings.HasPrefix(key, fmt.Sprintf("%s/%d/", r2VideoPrefix, uploader)) {
		return fmt.Errorf("upload key does not belong to this user")
	}
	size, err := h.storage.Size(r.Context(), key)
	if err != nil {
		return err
	}
	if declared > 0 && size != declared {
		return fmt.Errorf("upload incomplete: %d of %d bytes in R2", size, declared)
	}
	if size <= 0 {
		return fmt.Errorf("uploaded object is empty")
	}
	return nil
}
