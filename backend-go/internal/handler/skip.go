package handler

// Skip intro/outro endpoints. Resolution: our skip_marks → AniSkip
// (cached back into skip_marks) → nothing. A short-lived in-memory miss cache
// keeps repeated plays of an unknown episode from hammering AniSkip.

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
)

const aniskipMissTTL = 6 * time.Hour

type skipMissCache struct {
	mu     sync.Mutex
	misses map[int]time.Time
}

func (c *skipMissCache) recentMiss(episodeID int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	at, ok := c.misses[episodeID]
	return ok && time.Since(at) < aniskipMissTTL
}

func (c *skipMissCache) markMiss(episodeID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.misses == nil {
		c.misses = map[int]time.Time{}
	}
	c.misses[episodeID] = time.Now()
}

type skipRange struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// GET /api/episodes/{id}/skip
func (h *Handler) episodeSkip(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}
	marks, err := h.repo.SkipMarks(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "fetch skip marks", err)
		return
	}
	if len(marks) == 0 && !h.skipMisses.recentMiss(id) {
		marks = h.fetchAniskip(r.Context(), id)
	}
	out := map[string]any{"intro": nil, "outro": nil}
	for _, m := range marks {
		out[m.Kind] = skipRange{Start: m.StartS, End: m.EndS}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

// fetchAniskip asks AniSkip for an episode with no marks of our own and
// caches any hit into skip_marks. Every failure path degrades to "no marks" —
// skip buttons are never worth failing a playback for.
func (h *Handler) fetchAniskip(ctx context.Context, episodeID int) []model.SkipMark {
	malID, epNum, err := h.repo.EpisodeMALRef(ctx, episodeID)
	if err != nil || malID == nil {
		h.skipMisses.markMiss(episodeID)
		return nil
	}
	times, err := h.aniskip.SkipTimes(ctx, *malID, epNum)
	if err != nil {
		slog.Warn("aniskip lookup failed", "episodeId", episodeID, "malId", *malID, "err", err)
		h.skipMisses.markMiss(episodeID)
		return nil
	}
	if len(times) == 0 {
		h.skipMisses.markMiss(episodeID)
		return nil
	}
	marks := make([]model.SkipMark, 0, len(times))
	for _, t := range times {
		m, err := h.repo.UpsertSkipMark(ctx, repo.SkipMarkInput{
			EpisodeID: episodeID, Kind: t.Kind, StartS: t.Start, EndS: t.End, Source: "aniskip",
		})
		if err != nil {
			slog.Warn("cache aniskip mark", "episodeId", episodeID, "err", err)
			continue
		}
		marks = append(marks, *m)
	}
	return marks
}

type skipBody struct {
	Kind  string   `json:"kind"`
	Start *float64 `json:"start"`
	End   *float64 `json:"end"`
}

// POST /api/episodes/{id}/skip  (admin/translator) — set a mark manually,
// e.g. from the player's current position (admin panel).
func (h *Handler) setSkipMark(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}
	var body skipBody
	if err := httpx.Decode(r, &body); err != nil || body.Start == nil || body.End == nil {
		httpx.Error(w, http.StatusBadRequest, "kind, start and end are required")
		return
	}
	if body.Kind != "intro" && body.Kind != "outro" {
		httpx.Error(w, http.StatusBadRequest, "kind must be 'intro' or 'outro'")
		return
	}
	if *body.Start < 0 || *body.End <= *body.Start {
		httpx.Error(w, http.StatusBadRequest, "end must be after start (both in seconds)")
		return
	}
	mark, err := h.repo.UpsertSkipMark(r.Context(), repo.SkipMarkInput{
		EpisodeID: id, Kind: body.Kind, StartS: *body.Start, EndS: *body.End, Source: "manual",
	})
	if err != nil {
		httpx.Internal(w, "save skip mark", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": mark})
}

// DELETE /api/episodes/{id}/skip/{kind}  (admin/translator)
func (h *Handler) deleteSkipMark(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	kind := chi.URLParam(r, "kind")
	if !ok || (kind != "intro" && kind != "outro") {
		httpx.Error(w, http.StatusBadRequest, "Invalid parameters")
		return
	}
	if err := h.repo.DeleteSkipMark(r.Context(), id, kind); err != nil {
		notFoundOr(w, err, "Skip mark not found", "delete skip mark")
		return
	}
	// without this, an episode left with zero marks would re-fetch (and
	// re-cache) the very AniSkip data that was just deleted on the next play
	h.skipMisses.markMiss(id)
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Skip mark deleted successfully"})
}
