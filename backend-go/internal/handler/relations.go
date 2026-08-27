package handler

// Series relations: the season strip on an anime page, and the "same series"
// grid under it.

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/model"
)

// chainKinds are the two relations that put titles in a straight line. Every
// other kind goes to the grid: Fate/stay night's neighbours are ALTERNATIVE
// and SPIN_OFF precisely because there is no season order among them.
var chainKinds = map[string]bool{"SEQUEL": true, "PREQUEL": true}

// GET /api/anime/{id}/relations
//
// Never 404s on an anime with nothing related — an empty chain and an empty
// grid are the normal answer for a standalone film, and the detail page
// renders neither strip in that case.
func (h *Handler) animeRelations(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}

	chain, err := h.repo.SeasonChain(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "season chain", err)
		return
	}
	all, err := h.repo.AnimeRelations(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "anime relations", err)
		return
	}

	// Anything already shown as a season must not appear again in the grid.
	inChain := make(map[int]bool, len(chain))
	for _, c := range chain {
		inChain[c.ID] = true
	}
	related := make([]model.RelatedAnime, 0, len(all))
	for _, rel := range all {
		if chainKinds[rel.Relation] || inChain[rel.ID] {
			continue
		}
		related = append(related, rel)
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": model.AnimeRelations{Chain: chain, Related: related},
	})
}
