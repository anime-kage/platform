package handler

// Playback progress endpoints, called by our own player: resume
// position on load, throttled saves during playback. Crossing ~90% marks the
// episode watched through the watchlist upsert (auto-complete + history
// deltas live there).

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/repo"
)

// one definition, shared with the continue-watching query — see the comment
// on repo.WatchedFraction
const watchedThreshold = repo.WatchedFraction

// GET /api/episodes/{id}/progress  (auth)
func (h *Handler) getProgress(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}
	u := middleware.UserFrom(r)
	pos, err := h.repo.PlaybackPosition(r.Context(), u.UserID, id)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.JSON(w, http.StatusOK, map[string]any{"data": nil})
		return
	}
	if err != nil {
		httpx.Internal(w, "fetch progress", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": pos})
}

// POST /api/episodes/{id}/view  (auth)
//
// One member opening one episode, which is what the home leaderboards rank by.
// The page calls this once per episode; the primary key on episode_views makes
// repeat calls free, so there is nothing to guard against on the client.
//
// It is a view rather than a watch on purpose. Progress only reaches the server
// from our own player, and the third-party <iframe> fallback cannot report
// anything across origins — so a playback-based rule would leave every
// iframe-served episode on zero forever. Opening the page is the one signal both
// paths share. If the native player ever becomes the default, this can tighten
// to "reported past N%" without touching the table or the leaderboard.
func (h *Handler) recordEpisodeView(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}
	u := middleware.UserFrom(r)
	first, err := h.repo.RecordEpisodeView(r.Context(), u.UserID, id)
	if errors.Is(err, repo.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "Episode not found")
		return
	}
	if err != nil {
		httpx.Internal(w, "record episode view", err)
		return
	}
	// `counted` tells the caller whether this call was the one that counted.
	// Nothing in the UI depends on it today; it makes the endpoint testable
	// without reading the table.
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{"counted": first}})
}

type progressBody struct {
	Position *float64 `json:"position"`
	Duration *float64 `json:"duration"`
}

// PUT /api/episodes/{id}/progress  (auth)
func (h *Handler) saveProgress(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}
	var body progressBody
	if err := httpx.Decode(r, &body); err != nil || body.Position == nil || *body.Position < 0 {
		httpx.Error(w, http.StatusBadRequest, "position (seconds) is required")
		return
	}
	u := middleware.UserFrom(r)
	if err := h.repo.SavePlaybackPosition(r.Context(), u.UserID, id, *body.Position, body.Duration); err != nil {
		// FK violation = unknown episode
		httpx.Error(w, http.StatusNotFound, "Episode not found")
		return
	}

	watched := false
	if body.Duration != nil && *body.Duration > 0 && *body.Position / *body.Duration >= watchedThreshold {
		ok, err := h.repo.MarkEpisodeWatched(r.Context(), u.UserID, id)
		if err != nil {
			// progress is saved; the watchlist bump failing shouldn't 500 playback
			slog.Warn("mark episode watched", "userId", u.UserID, "episodeId", id, "err", err)
		}
		watched = ok
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"position": *body.Position,
		"watched":  watched,
	}})
}
