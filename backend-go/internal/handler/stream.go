package handler

// The stream resolve endpoint: picks the best extract source for an
// episode and asks the resolver for a playable manifest. The player calls this;
// on 404 it falls back to the iframe embed sources it already has.

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/resolver"
)

// GET /api/episodes/{id}/stream
func (h *Handler) episodeStream(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}

	sources, err := h.repo.ExtractSources(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "fetch sources", err)
		return
	}
	if len(sources) == 0 {
		httpx.Error(w, http.StatusNotFound, "No stream source for this episode")
		return
	}

	for _, src := range sources {
		if src.Provider == nil || src.ProviderRef == nil {
			continue // the DB check constraint should make this impossible
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		res, err := h.resolver.Resolve(ctx, *src.Provider, *src.ProviderRef)
		cancel()
		if err != nil {
			slog.Warn("resolve failed, trying next source",
				"episodeId", id, "sourceId", src.ID, "provider", *src.Provider, "err", err)
			continue
		}
		h.mergeOwnSubtitles(r, id, res)
		httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"stream": res,
			"source": map[string]any{
				"id":       src.ID,
				"provider": *src.Provider,
				"quality":  src.Quality,
				"language": src.Language,
			},
		}})
		return
	}
	httpx.Error(w, http.StatusNotFound, "No stream source for this episode")
}

// mergeOwnSubtitles prepends our published .vtt tracks to whatever the
// provider supplied — ours are the product, so they come first
// and duplicate provider tracks for the same language are dropped. Subtitle
// lookup failures are logged, not fatal: the stream still plays.
func (h *Handler) mergeOwnSubtitles(r *http.Request, episodeID int, res *resolver.Result) {
	subs, err := h.repo.PublishedSubtitles(r.Context(), episodeID)
	if err != nil {
		slog.Warn("fetch subtitles for stream", "episodeId", episodeID, "err", err)
		return
	}
	own := make([]resolver.Subtitle, 0, len(subs))
	seen := map[string]bool{}
	for _, s := range subs {
		if s.Format != "vtt" { // <track> only renders WebVTT
			continue
		}
		label := s.Language
		if s.Label != nil && *s.Label != "" {
			label = *s.Label
		}
		own = append(own, resolver.Subtitle{URL: s.URL, Language: s.Language, Label: label})
		seen[s.Language] = true
	}
	for _, t := range res.Subtitles {
		if !seen[t.Language] {
			own = append(own, t)
		}
	}
	res.Subtitles = own
}
