package handler

// Custom user lists ("Liste") — curated collections, the real backend behind
// the /liste page's "Ale mele" tab. Public lists are readable by anyone
// (including guests); everything else is owner-only.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
)

// loadOwnList fetches the list and enforces ownership. Writes the error
// response itself on failure.
func (h *Handler) loadOwnList(w http.ResponseWriter, r *http.Request) *model.UserList {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid list ID")
		return nil
	}
	ul, err := h.repo.UserListByID(r.Context(), id, middleware.UserFrom(r).UserID)
	if err != nil {
		notFoundOr(w, err, "Lista nu există", "fetch list")
		return nil
	}
	if ul.UserID != middleware.UserFrom(r).UserID {
		httpx.Error(w, http.StatusForbidden, "Nu e lista ta")
		return nil
	}
	return ul
}

type userListBody struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	IsPublic    *bool   `json:"isPublic"`
}

func (b *userListBody) clean(w http.ResponseWriter) bool {
	b.Title = strings.TrimSpace(b.Title)
	if b.Title == "" || len(b.Title) > 120 {
		httpx.Error(w, http.StatusBadRequest, "Titlul e obligatoriu (max 120 caractere)")
		return false
	}
	if b.Description != nil {
		d := strings.TrimSpace(*b.Description)
		if d == "" {
			b.Description = nil
		} else {
			if len(d) > 1000 {
				d = d[:1000]
			}
			b.Description = &d
		}
	}
	return true
}

// POST /api/lists  (auth) — {title, description?, isPublic?} (public default)
func (h *Handler) createUserList(w http.ResponseWriter, r *http.Request) {
	var body userListBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	if !body.clean(w) {
		return
	}
	isPublic := body.IsPublic == nil || *body.IsPublic
	ul, err := h.repo.CreateUserList(r.Context(), middleware.UserFrom(r).UserID, body.Title, body.Description, isPublic)
	if err != nil {
		httpx.Internal(w, "create list", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": ul})
}

// viewerIDOr0 returns the requesting user's id, or 0 for a guest — used to
// compute the per-viewer `liked` flag without failing on anonymous requests.
func viewerIDOr0(r *http.Request) int {
	if v := viewerID(r); v != nil {
		return *v
	}
	return 0
}

// GET /api/lists/mine  (auth)
func (h *Handler) myUserLists(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserFrom(r).UserID
	lists, err := h.repo.UserListsByOwner(r.Context(), uid, uid)
	if err != nil {
		httpx.Internal(w, "list lists", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": lists})
}

// GET /api/lists  — public lists with content, most-liked first (browse feed).
func (h *Handler) publicUserLists(w http.ResponseWriter, r *http.Request) {
	lists, err := h.repo.PublicUserLists(r.Context(), viewerIDOr0(r), 60)
	if err != nil {
		httpx.Internal(w, "list public lists", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": lists})
}

// POST /api/lists/{id}/like  (auth) — idempotent; returns {likeCount, liked}.
func (h *Handler) likeList(w http.ResponseWriter, r *http.Request) {
	h.setListLike(w, r, true)
}

// DELETE /api/lists/{id}/like  (auth) — idempotent; returns {likeCount, liked}.
func (h *Handler) unlikeList(w http.ResponseWriter, r *http.Request) {
	h.setListLike(w, r, false)
}

func (h *Handler) setListLike(w http.ResponseWriter, r *http.Request, like bool) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid list ID")
		return
	}
	// only public (or owned) lists are likeable — mirror getUserList visibility
	ul, err := h.repo.UserListByID(r.Context(), id, viewerIDOr0(r))
	if err != nil {
		notFoundOr(w, err, "Lista nu există", "fetch list")
		return
	}
	uid := middleware.UserFrom(r).UserID
	if !ul.IsPublic && ul.UserID != uid {
		httpx.Error(w, http.StatusNotFound, "Lista nu există")
		return
	}
	var count int
	if like {
		count, err = h.repo.LikeList(r.Context(), id, uid)
	} else {
		count, err = h.repo.UnlikeList(r.Context(), id, uid)
	}
	if err != nil {
		httpx.Internal(w, "set list like", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"likeCount": count, "liked": like})
}

// GET /api/lists/{id}  — public, or private to its owner. Items included.
func (h *Handler) getUserList(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid list ID")
		return
	}
	ul, err := h.repo.UserListByID(r.Context(), id, viewerIDOr0(r))
	if err != nil {
		notFoundOr(w, err, "Lista nu există", "fetch list")
		return
	}
	u := middleware.UserFrom(r)
	if !ul.IsPublic && (u == nil || u.UserID != ul.UserID) {
		// a private list is indistinguishable from a missing one
		httpx.Error(w, http.StatusNotFound, "Lista nu există")
		return
	}
	items, err := h.repo.UserListItems(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "fetch list items", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": ul, "items": items})
}

// PUT /api/lists/{id}  (owner) — {title, description?, isPublic?}
func (h *Handler) updateUserList(w http.ResponseWriter, r *http.Request) {
	ul := h.loadOwnList(w, r)
	if ul == nil {
		return
	}
	var body userListBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	if !body.clean(w) {
		return
	}
	isPublic := ul.IsPublic
	if body.IsPublic != nil {
		isPublic = *body.IsPublic
	}
	if err := h.repo.UpdateUserList(r.Context(), ul.ID, body.Title, body.Description, isPublic); err != nil {
		notFoundOr(w, err, "Lista nu există", "update list")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Listă actualizată"})
}

// DELETE /api/lists/{id}  (owner)
func (h *Handler) deleteUserList(w http.ResponseWriter, r *http.Request) {
	ul := h.loadOwnList(w, r)
	if ul == nil {
		return
	}
	if err := h.repo.DeleteUserList(r.Context(), ul.ID); err != nil {
		notFoundOr(w, err, "Lista nu există", "delete list")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Listă ștearsă"})
}

// POST /api/lists/{id}/items  (owner) — {mediaType: anime|manga, mediaId, note?}
func (h *Handler) addUserListItem(w http.ResponseWriter, r *http.Request) {
	ul := h.loadOwnList(w, r)
	if ul == nil {
		return
	}
	var body struct {
		MediaType string  `json:"mediaType"`
		MediaID   int     `json:"mediaId"`
		Note      *string `json:"note"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.MediaID < 1 {
		httpx.Error(w, http.StatusBadRequest, "mediaType and mediaId are required")
		return
	}
	var animeID, mangaID *int
	switch body.MediaType {
	case "anime":
		animeID = &body.MediaID
	case "manga":
		mangaID = &body.MediaID
	default:
		httpx.Error(w, http.StatusBadRequest, "mediaType must be 'anime' or 'manga'")
		return
	}
	it, err := h.repo.AddUserListItem(r.Context(), ul.ID, animeID, mangaID, body.Note)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrExists):
			httpx.Error(w, http.StatusConflict, "Titlul e deja pe listă")
		case repo.IsForeignKeyViolation(err):
			httpx.Error(w, http.StatusNotFound, "Title not found")
		default:
			httpx.Internal(w, "add list item", err)
		}
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": it})
}

// PUT /api/lists/{id}/items/{itemId}  (owner) — {note}
func (h *Handler) updateUserListItem(w http.ResponseWriter, r *http.Request) {
	ul := h.loadOwnList(w, r)
	if ul == nil {
		return
	}
	itemID, ok := httpx.IntParam(chi.URLParam(r, "itemId"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid item ID")
		return
	}
	var body struct {
		Note *string `json:"note"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	if body.Note != nil {
		n := strings.TrimSpace(*body.Note)
		if n == "" {
			body.Note = nil
		} else {
			if len(n) > 500 {
				n = n[:500]
			}
			body.Note = &n
		}
	}
	if err := h.repo.UpdateUserListItemNote(r.Context(), ul.ID, itemID, body.Note); err != nil {
		notFoundOr(w, err, "Titlul nu e pe listă", "update list item")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Notă salvată"})
}

// DELETE /api/lists/{id}/items/{itemId}  (owner)
func (h *Handler) removeUserListItem(w http.ResponseWriter, r *http.Request) {
	ul := h.loadOwnList(w, r)
	if ul == nil {
		return
	}
	itemID, ok := httpx.IntParam(chi.URLParam(r, "itemId"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid item ID")
		return
	}
	if err := h.repo.RemoveUserListItem(r.Context(), ul.ID, itemID); err != nil {
		notFoundOr(w, err, "Titlul nu e pe listă", "remove list item")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Titlu scos de pe listă"})
}
