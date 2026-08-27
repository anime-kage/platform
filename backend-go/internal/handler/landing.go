package handler

import (
	"net/http"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/repo"
)

// collageSize is how many cards the landing collage lays out. The layout has
// exactly this many positions and looks broken with fewer, so a short curated
// slot is topped up from the catalog rather than shown short.
const collageSize = 3

// topUpLimit is how deep to go looking for replacements. Titles with no cover
// art are skipped, so asking for exactly the shortfall would routinely come
// back short on a thin catalog.
const topUpLimit = 24

// landingCollageItem is the smallest shape the public front page can render: a
// key, a cover, and the three title fields displayName() falls through.
//
// This is the *only* catalog read left open to unauthenticated callers. The
// catalog proper now requires a session, and this endpoint exists so that
// closing it did not also blank the front door. It stays deliberately narrow
// for that reason — no synopsis, no score, no pagination, no filters. Anything
// added here is something a scraper gets for free.
type landingCollageItem struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	TitleEnglish  *string `json:"titleEnglish,omitempty"`
	TitleRomanian *string `json:"titleRomanian,omitempty"`
	ImageURL      *string `json:"imageUrl,omitempty"`
}

// GET /api/landing — public.
//
// The coordinator's landing_collage picks first, topped up with the
// highest-scored covers. This selection used to live in the SvelteKit load,
// which is why /api/anime and /api/curated both had to stay readable by
// anyone; moving it here is what allowed them to be closed.
func (h *Handler) landing(w http.ResponseWriter, r *http.Request) {
	picks, err := h.repo.CuratedSlot(r.Context(), "landing_collage")
	if err != nil {
		httpx.Internal(w, "landing curated slot", err)
		return
	}

	out := make([]landingCollageItem, 0, collageSize)
	// Keyed on the bare id, not kind+id, on purpose: the page renders this with
	// a keyed {#each ... (a.id)}, and an anime and a manga that happen to share
	// an id would be a duplicate-key crash. Over-de-duplicating costs one card
	// in a rare case; the alternative breaks the front page.
	seen := make(map[int]bool, collageSize)

	add := func(it landingCollageItem) {
		if len(out) >= collageSize || seen[it.ID] {
			return
		}
		// No cover, no card — the collage is nothing but art.
		if it.ImageURL == nil || *it.ImageURL == "" {
			return
		}
		seen[it.ID] = true
		out = append(out, it)
	}

	for _, p := range picks {
		switch {
		case p.Anime != nil:
			img := p.Anime.ImageURL
			// Per-placement artwork overrides the cover for this slot only.
			if p.ImageURL != nil {
				img = p.ImageURL
			}
			add(landingCollageItem{
				ID:            p.Anime.ID,
				Title:         p.Anime.Title,
				TitleEnglish:  p.Anime.TitleEnglish,
				TitleRomanian: p.Anime.TitleRomanian,
				ImageURL:      img,
			})
		case p.Manga != nil:
			img := p.Manga.ImageURL
			if p.ImageURL != nil {
				img = p.ImageURL
			}
			add(landingCollageItem{
				ID:            p.Manga.ID,
				Title:         p.Manga.Title,
				TitleEnglish:  p.Manga.TitleEnglish,
				TitleRomanian: p.Manga.TitleRomanian,
				ImageURL:      img,
			})
		}
	}

	if len(out) < collageSize {
		list, _, err := h.repo.SearchAnime(r.Context(), repo.TitleFilters{Sort: "score", Limit: topUpLimit})
		if err != nil {
			httpx.Internal(w, "landing catalog top-up", err)
			return
		}
		for i := range list {
			add(landingCollageItem{
				ID:            list[i].ID,
				Title:         list[i].Title,
				TitleEnglish:  list[i].TitleEnglish,
				TitleRomanian: list[i].TitleRomanian,
				ImageURL:      list[i].ImageURL,
			})
		}
	}

	// An empty collage is a valid answer, not an error: a fresh deploy has no
	// catalog and the page drops the art rather than failing.
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{"collage": out}})
}
