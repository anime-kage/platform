package handler

// MKV→MP4 rewrap at ingest.
//
// Translators upload Matroska; browsers do not play Matroska. That breaks the
// preview on the translator page, which is the only way they confirm they
// uploaded the right episode — and it breaks it silently, with a black box and
// no error. So an MKV upload queues a rewrap and the release points at the MP4
// once it lands.
//
// This shares the hardsub worker's goroutine rather than running its own. Both
// jobs are ffmpeg on a four-core box that also has to answer HTTP, and letting a
// rewrap start while a burn is running would mean two encoders competing for the
// cores the API needs. One worker, two queues, rewraps first — see runNextJob.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/repo"
	"animekage/backend/internal/subs"
)

// queueRemuxIfNeeded starts a rewrap when the uploaded video is in a container
// the browser cannot play. Best-effort by design: the release itself is already
// created and usable, so a queueing failure is logged rather than failing the
// upload the translator just spent twenty minutes on.
func (h *Handler) queueRemuxIfNeeded(ctx context.Context, releaseID int, videoExt string) {
	if !remuxExts[strings.ToLower(videoExt)] {
		return
	}
	if !subs.HasFFmpeg() {
		slog.Warn("mkv uploaded but ffmpeg is missing — preview will not play", "release", releaseID)
		return
	}
	if _, err := h.repo.QueueRemux(ctx, releaseID); err != nil {
		slog.Error("queue remux", "release", releaseID, "err", err)
	}
}

// remuxExts are the containers worth rewrapping. .webm is playable and .mp4 is
// already the target, so neither is here.
var remuxExts = map[string]bool{".mkv": true}

// runNextRemux claims one rewrap and runs it. Reports whether it did anything.
func (h *Handler) runNextRemux(ctx context.Context) (bool, error) {
	job, err := h.repo.ClaimNextRemux(ctx)
	if err != nil || job == nil {
		return false, err
	}
	if job.StagingPath == nil && (job.R2Key == nil || *job.R2Key == "") {
		// The release was cancelled between queueing and claiming.
		_ = h.repo.FailRemux(ctx, job.ReleaseID, "Videoul nu mai există.")
		return true, nil
	}

	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	prog := &hardsubProgress{cancel: cancel}
	h.remuxing.Store(job.ReleaseID, prog)
	defer h.remuxing.Delete(job.ReleaseID)

	start := time.Now()
	slog.Info("remux start", "release", job.ReleaseID)

	if err := h.remuxRelease(jobCtx, job, prog); err != nil {
		// A shutdown must leave the row 'running' so the next process requeues it.
		if ctx.Err() != nil {
			return false, nil
		}
		slog.Error("remux failed", "release", job.ReleaseID, "err", err)
		_ = h.repo.FailRemux(ctx, job.ReleaseID,
			"Conversia în MP4 a eșuat. Videoul rămâne încărcat, dar previzualizarea nu va funcționa.")
		return true, nil
	}
	if err := h.repo.FinishRemux(ctx, job.ReleaseID); err != nil {
		return true, err
	}
	slog.Info("remux done", "release", job.ReleaseID, "took", time.Since(start).Round(time.Second))
	return true, nil
}

// remuxRelease rewraps the release's video as MP4 and repoints the release at it.
//
// The order of the last three steps is the part that matters: write the new file,
// *then* update the row, *then* delete the old one. A crash at any point leaves
// the release pointing at a file that exists — the worst case is an orphaned
// object, which the publish purge and the bucket lifecycle rule both sweep up.
func (h *Handler) remuxRelease(ctx context.Context, job *repo.RemuxJob, prog *hardsubProgress) error {
	dir := filepath.Join(h.cfg.StagingDir, strconv.Itoa(job.ReleaseID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	// Written locally either way: ffmpeg needs a seekable output for +faststart,
	// so this cannot stream straight into the bucket.
	outPath := filepath.Join(dir, "remux.mp4")

	src := ""
	inR2 := job.R2Key != nil && *job.R2Key != ""
	if inR2 {
		if h.storage == nil {
			return errors.New("video is in R2 but R2 is not configured")
		}
		u, uerr := h.storage.PresignGet(ctx, *job.R2Key, presignTTL)
		if uerr != nil {
			return fmt.Errorf("presign source: %w", uerr)
		}
		src = u
	} else {
		src = filepath.Join(h.cfg.StagingDir, filepath.FromSlash(*job.StagingPath))
	}

	if err := subs.RemuxToMP4(ctx, src, outPath, prog.set); err != nil {
		// A half-written MP4 is worse than none: the release would be repointed at
		// a truncated file that plays for ten seconds and stops.
		_ = os.Remove(outPath)
		return err
	}

	if !inR2 {
		// Local staging: swap the path, then drop the MKV.
		newRel := strconv.Itoa(job.ReleaseID) + "/video.mp4"
		final := filepath.Join(h.cfg.StagingDir, filepath.FromSlash(newRel))
		if err := os.Rename(outPath, final); err != nil {
			return fmt.Errorf("place remuxed video: %w", err)
		}
		old := src
		if err := h.repo.SetReleaseStagingPath(ctx, job.ReleaseID, &newRel); err != nil {
			return fmt.Errorf("repoint staging path: %w", err)
		}
		if old != final {
			_ = os.Remove(old)
		}
		return nil
	}

	// R2: upload the MP4 beside the MKV, repoint, then delete the MKV.
	oldKey := *job.R2Key
	newKey := strings.TrimSuffix(oldKey, filepath.Ext(oldKey)) + ".mp4"
	f, err := os.Open(outPath)
	if err != nil {
		return fmt.Errorf("open remuxed video: %w", err)
	}
	err = h.storage.PutStream(ctx, newKey, "video/mp4", f)
	f.Close()
	if err != nil {
		_ = os.Remove(outPath)
		return fmt.Errorf("upload remuxed video: %w", err)
	}
	// The local copy has served its purpose — the bucket is the source of truth.
	_ = os.Remove(outPath)

	if err := h.repo.SetReleaseR2Key(ctx, job.ReleaseID, &newKey); err != nil {
		// The MP4 is in the bucket but unreferenced. Leave it: the lifecycle rule
		// on the prefix reclaims it, and deleting it here could race a retry.
		return fmt.Errorf("repoint r2 key: %w", err)
	}
	if err := h.storage.Delete(ctx, oldKey); err != nil {
		// Not fatal — the release is already correct, this is only space.
		slog.Warn("delete source mkv after remux", "release", job.ReleaseID, "key", oldKey, "err", err)
	}
	return nil
}

// ── HTTP ─────────────────────────────────────────────────────────────────────

type remuxView struct {
	State    string  `json:"state"` // idle | queued | running | done | failed
	Progress float64 `json:"progress"`
	Position int     `json:"position"`
	Error    string  `json:"error,omitempty"`
}

// GET /api/releases/{id}/remux — state + progress, for polling.
//
// Readable by the uploader as well as coordinators: this is what the translator
// page waits on before it will show a preview, and the translator is usually not
// a coordinator.
func (h *Handler) remuxStatus(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	st, err := h.repo.RemuxStatus(r.Context(), rel.ID)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "Release not found")
		return
	}
	if err != nil {
		httpx.Internal(w, "remux status", err)
		return
	}

	view := remuxView{State: "idle"}
	if st.State != nil {
		view.State = *st.State
	}
	view.Position = st.Position
	if st.Error != nil {
		view.Error = *st.Error
	}
	if v, ok := h.remuxing.Load(rel.ID); ok {
		view.Progress = v.(*hardsubProgress).get()
	} else if view.State == "done" {
		view.Progress = 1
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": view})
}
