package handler

// Profile banner — the Letterboxd-style backdrop behind a
// member's profile header.

import (
	"net/http"
	"strings"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/repo"
)

// GET /api/users/me/banner/options
//
// Titles from the member's own lists that have banner art. Scoped to their
// lists deliberately: the backdrop is meant to say something about them, and
// a catalog-wide picker says nothing.
func (h *Handler) myBannerOptions(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFrom(r)
	opts, err := h.repo.BannerCandidates(r.Context(), user.UserID)
	if err != nil {
		httpx.Internal(w, "list banner options", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": opts})
}

// PUT /api/users/me/banner  {"mediaType":"anime","id":7}  — id 0 clears it.
func (h *Handler) setMyBanner(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MediaType string `json:"mediaType"`
		ID        int    `json:"id"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	mediaType := strings.ToLower(strings.TrimSpace(body.MediaType))
	if body.ID != 0 && mediaType != "anime" && mediaType != "manga" {
		httpx.Error(w, http.StatusBadRequest, "Tip de conținut invalid")
		return
	}

	user := middleware.UserFrom(r)
	if err := h.repo.SetUserBanner(r.Context(), user.UserID, mediaType, body.ID); err != nil {
		// the FK is what stops a profile pointing at a title that isn't there
		if repo.IsForeignKeyViolation(err) {
			httpx.Error(w, http.StatusBadRequest, "Titlul nu există în catalog")
			return
		}
		httpx.Internal(w, "set profile banner", err)
		return
	}

	banner, err := h.repo.UserBanner(r.Context(), user.UserID)
	if err != nil {
		httpx.Internal(w, "set profile banner", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": banner})
}
