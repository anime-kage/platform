package handler

// The GIF picker's endpoints — a thin, cached proxy in front of Giphy.
//
// Why this exists at all rather than calling Giphy from the browser: the free
// tier is 100 API calls an hour, and a key handed to every visitor is a quota
// nobody can protect. Here one cached `trending` serves every member for 15
// minutes, repeat searches cost nothing for an hour, and the content rating is
// pinned where a client cannot argue with it.

import (
	"errors"
	"net/http"
	"strings"

	"animekage/backend/internal/giphy"
	"animekage/backend/internal/httpx"
)

// GET /api/gifs?q=  — omit q for trending.
func (h *Handler) searchGifs(w http.ResponseWriter, r *http.Request) {
	if !h.giphy.Enabled() {
		// Not an error the member caused: the key simply isn't configured.
		// The UI reads this as "hide the GIF button" rather than showing a
		// failure on every keystroke.
		httpx.Error(w, http.StatusServiceUnavailable, "Căutarea de GIF-uri nu e configurată")
		return
	}
	limit := httpx.QueryInt(r, "limit", 24, 1, 24)
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	var (
		gifs []giphy.GIF
		err  error
	)
	if q == "" {
		gifs, err = h.giphy.Trending(limit)
	} else {
		gifs, err = h.giphy.Search(q, limit)
	}
	if errors.Is(err, giphy.ErrRateLimited) {
		// Soft failure by design: every GIF already posted keeps rendering,
		// because those are CDN URLs. Only finding a NEW one is affected.
		httpx.Error(w, http.StatusTooManyRequests,
			"Prea multe căutări acum. Încearcă din nou în câteva minute.")
		return
	}
	if err != nil {
		httpx.Internal(w, "giphy search", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": gifs})
}
