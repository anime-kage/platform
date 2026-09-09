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
		// Our own .vtt tracks are deliberately NOT attached. Every source we
		// publish is a burned copy -- the hardsub pipeline exists precisely
		// because the hosts cannot carry soft subs -- so adding them here drew
		// a second set of subtitles over the ones already in the picture.
		// Only extract sources reach this endpoint (embeds play in the host's
		// own iframe and never render our <track>s), so this is the one place
		// it could happen. The rows stay published: they are the translation
		// record, and health monitoring counts them.
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

