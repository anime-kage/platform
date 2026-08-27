package handler

// Optional hardsub burn.
//
// Opt-in, and the existing publish flow is untouched: publishing still writes the
// soft .vtt track and links the source exactly as before, with or without a burn
// having happened. This produces one extra artefact — a copy of the video with
// the RO track rendered into the picture — for the case where the host can only
// be embedded, and an <iframe> cannot carry our subtitle.
//
// Concurrency is one. The measured cost is ~13 minutes per 24-minute episode with
// all four cores busy, so a second concurrent burn would starve the API on the
// same box. Jobs queue in the database (FIFO by hardsub_queued_at) and the queue
// position is reported, so a coordinator sees "2nd in line" instead of a spinner.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/repo"
	"animekage/backend/internal/subs"
)

// hardsubPoll is how often the worker looks for queued work when idle. A burn
// takes minutes, so a few seconds of latency before starting one is invisible,
// and polling beats plumbing a notification channel through the handler.
const hardsubPoll = 5 * time.Second

// hardsubProgress is the in-memory half of a job's state: a fraction that moves
// several times a second. Deliberately not in the database — see 0030_hardsub.sql.
type hardsubProgress struct {
	mu       sync.Mutex
	fraction float64
	// cancel kills this job's ffmpeg. Stored per job so "stop" targets one burn
	// rather than the worker: cancelling the worker's own context would take the
	// queue down with it.
	cancel context.CancelFunc
	// stopped distinguishes a user pressing stop from the process shutting down.
	// Both cancel the context, but a shutdown must leave the row 'running' so the
	// next process requeues it, while a stop must clear it and stay cleared.
	stopped bool
}

func (p *hardsubProgress) stop() {
	p.mu.Lock()
	p.stopped = true
	c := p.cancel
	p.mu.Unlock()
	if c != nil {
		c()
	}
}

func (p *hardsubProgress) wasStopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

func (p *hardsubProgress) set(f float64) {
	p.mu.Lock()
	p.fraction = f
	p.mu.Unlock()
}

func (p *hardsubProgress) get() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fraction
}

// StartHardsubWorker runs the single burn worker until ctx is cancelled.
//
// Called once from main. Requeues anything left 'running' by a previous process
// first: such a row has no worker behind it and would otherwise show as in
// progress forever.
func (h *Handler) StartHardsubWorker(ctx context.Context) {
	if n, err := h.repo.RequeueRunningHardsubs(ctx); err != nil {
		slog.Error("requeue interrupted hardsub jobs", "err", err)
	} else if n > 0 {
		slog.Info("requeued hardsub jobs interrupted by restart", "count", n)
	}
	if n, err := h.repo.RequeueRunningRemuxes(ctx); err != nil {
		slog.Error("requeue interrupted remux jobs", "err", err)
	} else if n > 0 {
		slog.Info("requeued remux jobs interrupted by restart", "count", n)
	}

	go func() {
		t := time.NewTicker(hardsubPoll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				// Drain the queue before sleeping again: several approvals in a
				// row should burn back-to-back, not one per tick.
				for {
					ran, err := h.runNextJob(ctx)
					if err != nil {
						slog.Error("media worker", "err", err)
					}
					if !ran || ctx.Err() != nil {
						break
					}
				}
			}
		}
	}()
}

// runNextJob runs one unit of ffmpeg work, rewraps before burns.
//
// The priority is not arbitrary. A rewrap is a stream copy that takes minutes and
// blocks a translator who is staring at a dead preview; a burn re-encodes for
// ~9 minutes and nobody is waiting on it in real time. Draining rewraps first
// means an upload never queues behind an encode.
func (h *Handler) runNextJob(ctx context.Context) (bool, error) {
	if ran, err := h.runNextRemux(ctx); ran || err != nil {
		return ran, err
	}
	return h.runNextHardsub(ctx)
}

// runNextHardsub claims one job and burns it. Reports whether it did anything.
func (h *Handler) runNextHardsub(ctx context.Context) (bool, error) {
	job, err := h.repo.ClaimNextHardsub(ctx)
	if err != nil || job == nil {
		return false, err
	}
	if job.StagingPath == nil && (job.R2Key == nil || *job.R2Key == "") {
		// The video was cleaned up between queueing and claiming.
		_ = h.repo.FailHardsub(ctx, job.ReleaseID, "Videoul nu mai există.")
		return true, nil
	}

	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	prog := &hardsubProgress{cancel: cancel}
	h.hardsubbing.Store(job.ReleaseID, prog)
	defer h.hardsubbing.Delete(job.ReleaseID)

	start := time.Now()
	slog.Info("hardsub start", "release", job.ReleaseID)

	outRel, err := h.burnRelease(jobCtx, job, prog)
	if err != nil {
		// A user pressing stop clears the job; a shutdown leaves it 'running' so
		// the next process requeues it. Both look like a cancelled context here,
		// which is why the flag exists.
		if prog.wasStopped() {
			if cerr := h.repo.ClearHardsub(context.WithoutCancel(ctx), job.ReleaseID); cerr != nil {
				slog.Error("clear stopped hardsub", "release", job.ReleaseID, "err", cerr)
			}
			slog.Info("hardsub stopped by user", "release", job.ReleaseID)
			return true, nil
		}
		if ctx.Err() != nil {
			return false, nil
		}
		slog.Error("hardsub failed", "release", job.ReleaseID, "err", err)
		_ = h.repo.FailHardsub(ctx, job.ReleaseID, humanBurnError(err))
		return true, nil
	}
	if err := h.repo.FinishHardsub(ctx, job.ReleaseID, outRel); err != nil {
		return true, err
	}
	slog.Info("hardsub done", "release", job.ReleaseID, "took", time.Since(start).Round(time.Second), "path", outRel)
	return true, nil
}

// burnRelease writes the ASS from the release's RO rows and runs the encode.
// Returns the staging-relative path of the artefact.
func (h *Handler) burnRelease(ctx context.Context, job *repo.HardsubJob, prog *hardsubProgress) (string, error) {
	releaseID := job.ReleaseID
	events, err := h.repo.EventsByRelease(ctx, releaseID)
	if err != nil {
		return "", fmt.Errorf("load events: %w", err)
	}
	cues := make([]subs.Event, 0, len(events))
	for _, e := range events {
		// Only translated rows. An empty RO line must stay off screen rather
		// than burn an empty box into the picture permanently.
		if e.RoText == "" {
			continue
		}
		cues = append(cues, subs.Event{Idx: len(cues), StartMs: e.StartMs, EndMs: e.EndMs, Text: e.RoText})
	}
	if len(cues) == 0 {
		return "", errors.New("no-ro-lines")
	}

	dir := filepath.Join(h.cfg.StagingDir, strconv.Itoa(releaseID))
	assPath := filepath.Join(dir, "hardsub.ass")
	if err := os.WriteFile(assPath, []byte(subs.WriteASS(cues)), 0o644); err != nil {
		return "", fmt.Errorf("write ass: %w", err)
	}

	// The source is either on disk or in the bucket; ffmpeg reads https with
	// range requests, so a presigned URL works as an input unchanged.
	videoPath := ""
	if job.R2Key != nil && *job.R2Key != "" {
		if h.storage == nil {
			return "", errors.New("video is in R2 but R2 is not configured")
		}
		u, uerr := h.storage.PresignGet(ctx, *job.R2Key, presignTTL)
		if uerr != nil {
			return "", fmt.Errorf("presign source: %w", uerr)
		}
		videoPath = u
	} else {
		videoPath = filepath.Join(h.cfg.StagingDir, filepath.FromSlash(*job.StagingPath))
	}
	outName := "hardsub.mp4"
	outPath := filepath.Join(dir, outName)

	opt := subs.BurnOptions{
		Preset: h.cfg.HardsubPreset, Tune: h.cfg.HardsubTune, CRF: h.cfg.HardsubCRF,
		Nice: h.cfg.HardsubNice, Threads: h.cfg.HardsubThreads,
	}
	if err := subs.Burn(ctx, videoPath, assPath, outPath, opt, prog.set); err != nil {
		// A half-written mp4 is worse than none: it would be served as if it
		// were the finished artefact.
		_ = os.Remove(outPath)
		return "", err
	}
	return strconv.Itoa(releaseID) + "/" + outName, nil
}

// humanBurnError turns a worker failure into something a coordinator can act on.
// ffmpeg's stderr is useful in the log and unhelpful in a toast.
func humanBurnError(err error) string {
	if err.Error() == "no-ro-lines" {
		return "Nicio replică tradusă în română — nu e nimic de încrustat."
	}
	return "Încrustarea a eșuat. Vezi logurile serverului pentru detalii ffmpeg."
}

// ── HTTP ─────────────────────────────────────────────────────────────────────

type hardsubView struct {
	State    string  `json:"state"` // idle | queued | running | done | failed
	Progress float64 `json:"progress"`
	Position int     `json:"position"` // 1-based place in the queue, 0 when not queued
	Error    string  `json:"error,omitempty"`
	Ready    bool    `json:"ready"` // an artefact exists and can be downloaded
}

// POST /api/releases/{id}/hardsub  (content role) — queue a burn.
//
// Explicitly not wired into approveRelease: the burn is optional, and the flow
// that existed before this feature must keep working untouched. A coordinator
// asks for it from the publish page when the host for this episode is one we can
// only embed.
func (h *Handler) queueHardsub(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	if rel.Medium == "manga" {
		httpx.Error(w, http.StatusBadRequest, "Încrustarea e doar pentru episoade video.")
		return
	}
	// Either location counts. Checking only staging_path here was a leftover from
	// before the video stayed in R2 (0031), and it rejected every release uploaded
	// since — the burn itself has read from a presigned URL the whole time.
	if rel.StagingPath == nil && (rel.R2Key == nil || *rel.R2Key == "") {
		httpx.Error(w, http.StatusNotFound, "Nu mai există video pentru acest release.")
		return
	}
	if !subs.HasFFmpeg() {
		httpx.Error(w, http.StatusServiceUnavailable, "ffmpeg lipsește pe server.")
		return
	}
	queued, err := h.repo.QueueHardsub(r.Context(), rel.ID)
	if err != nil {
		httpx.Internal(w, "queue hardsub", err)
		return
	}
	if !queued {
		// Already queued or running — a second click, not an error.
		httpx.JSON(w, http.StatusOK, map[string]string{"message": "Încrustarea e deja în curs."})
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"message": "Încrustare pusă în coadă."})
}

// DELETE /api/releases/{id}/hardsub  (content role) — stop.
//
// Cancels a running burn or drops a queued one. A running job's ffmpeg is killed
// through its own context, and the half-written mp4 is removed by burnRelease's
// cleanup, so the release goes back to having no artefact rather than a truncated
// one.
func (h *Handler) stopHardsub(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	if v, ok := h.hardsubbing.Load(rel.ID); ok {
		v.(*hardsubProgress).stop()
		httpx.JSON(w, http.StatusOK, map[string]string{"message": "Se oprește."})
		return
	}

	// Only a queued job may be dropped from here. Guarding on state matters:
	// without it, "stop" on a *finished* burn cleared the row while leaving the
	// mp4 on disk — the artefact still existed but nothing pointed at it, so the
	// download button vanished and the file leaked. Discarding a finished burn is
	// a different action from stopping one, and this endpoint is not it.
	st, err := h.repo.HardsubStatus(r.Context(), rel.ID)
	if err != nil {
		httpx.Internal(w, "hardsub status", err)
		return
	}
	if st.State == nil || *st.State != "queued" {
		httpx.Error(w, http.StatusConflict, "Nu e nimic de oprit.")
		return
	}
	if err := h.repo.ClearHardsub(r.Context(), rel.ID); err != nil {
		httpx.Internal(w, "clear hardsub", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Oprit."})
}

// GET /api/releases/{id}/hardsub  (content role) — state + progress, for polling.
func (h *Handler) hardsubStatus(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	st, err := h.repo.HardsubStatus(r.Context(), rel.ID)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "Release not found")
		return
	}
	if err != nil {
		httpx.Internal(w, "hardsub status", err)
		return
	}

	view := hardsubView{State: "idle"}
	if st.State != nil {
		view.State = *st.State
	}
	view.Position = st.Position
	view.Ready = st.Path != nil && view.State == "done"
	if st.Error != nil {
		view.Error = *st.Error
	}
	// Merge the worker's live fraction. Only meaningful while running.
	if v, ok := h.hardsubbing.Load(rel.ID); ok {
		view.Progress = v.(*hardsubProgress).get()
	} else if view.State == "done" {
		view.Progress = 1
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": view})
}

// GET /api/releases/{id}/download.hardsub.mp4  (content role, TokenFromQuery)
//
// The artefact to upload to the host. Served from disk rather than piped so the
// download is resumable — these are gigabyte-sized.
func (h *Handler) releaseDownloadHardsub(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	st, err := h.repo.HardsubStatus(r.Context(), rel.ID)
	if err != nil || st.Path == nil || st.State == nil || *st.State != "done" {
		httpx.Error(w, http.StatusNotFound, "Nu există încă un video cu subtitrarea încrustată.")
		return
	}
	path := filepath.Join(h.cfg.StagingDir, filepath.FromSlash(*st.Path))
	f, err := os.Open(path)
	if err != nil {
		// The row says done but the file is gone — staging was cleaned.
		httpx.Error(w, http.StatusNotFound, "Fișierul încrustat nu mai există în staging.")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		httpx.Internal(w, "stat hardsub", err)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s.ro-hardsub.mp4"`, downloadBase(rel)))
	http.ServeContent(w, r, "hardsub.mp4", fi.ModTime(), f)
}
