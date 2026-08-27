package handler

// Moderation endpoints: the reported-comments queue and user
// sanctions. Queue actions are moderator+; role changes are admin-only.

import (
	"strings"
	"net/http"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
)

// GET /api/admin/reports  (admin/moderator)
func (h *Handler) listReports(w http.ResponseWriter, r *http.Request) {
	limit := httpx.QueryInt(r, "limit", 50, 1, 200)
	offset := httpx.QueryInt(r, "offset", 0, 0, 1_000_000)
	rows, total, err := h.repo.ReportedComments(r.Context(), limit, offset)
	if err != nil {
		httpx.Internal(w, "fetch reports", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows, "total": total})
}

// POST /api/admin/comments/{id}/dismiss  (admin/moderator) — keep the comment
func (h *Handler) dismissReport(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid comment ID")
		return
	}
	if err := h.repo.DismissReport(r.Context(), id); err != nil {
		notFoundOr(w, err, "No open report on this comment", "dismiss report")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Report dismissed"})
}

// DELETE /api/admin/comments/{id}  (admin/moderator) — any comment, not just own
func (h *Handler) modDeleteComment(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid comment ID")
		return
	}
	if err := h.repo.ModDeleteComment(r.Context(), id); err != nil {
		notFoundOr(w, err, "Comment not found", "moderator delete comment")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Comment deleted"})
}

// GET /api/admin/users?q=  (admin/moderator)
func (h *Handler) findUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		httpx.Error(w, http.StatusBadRequest, "q is required")
		return
	}
	limit := httpx.QueryInt(r, "limit", 20, 1, 50)
	rows, err := h.repo.FindUsers(r.Context(), q, limit)
	if err != nil {
		httpx.Internal(w, "search users", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// POST /api/admin/users/{id}/ban  (admin/moderator)  body: {"banned": bool}
func (h *Handler) banUser(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	var body struct {
		Banned *bool `json:"banned"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.Banned == nil {
		httpx.Error(w, http.StatusBadRequest, "banned (true/false) is required")
		return
	}
	target, err := h.repo.UserSanction(r.Context(), id)
	if err != nil {
		notFoundOr(w, err, "User not found", "ban user")
		return
	}
	if target.Role == "admin" || target.Role == "moderator" {
		httpx.Error(w, http.StatusBadRequest, "Admins and moderators cannot be banned — demote first")
		return
	}
	if id == middleware.UserFrom(r).UserID {
		httpx.Error(w, http.StatusBadRequest, "You cannot ban yourself")
		return
	}
	if err := h.repo.SetUserBanned(r.Context(), id, *body.Banned); err != nil {
		httpx.Internal(w, "ban user", err)
		return
	}
	msg := "User unbanned"
	if *body.Banned {
		msg = "User banned"
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": msg})
}

var validRoles = map[string]bool{
	"user": true, "translator": true, "verifier": true, "coordinator": true, "moderator": true, "admin": true,
}

// PUT /api/admin/users/{id}/role  (admin)  body: {"role": "..."}
func (h *Handler) changeUserRole(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := httpx.Decode(r, &body); err != nil || !validRoles[body.Role] {
		httpx.Error(w, http.StatusBadRequest, "role must be one of user, translator, verifier, coordinator, moderator, admin")
		return
	}
	if id == middleware.UserFrom(r).UserID {
		// the classic lockout: the only admin demoting themselves
		httpx.Error(w, http.StatusBadRequest, "You cannot change your own role")
		return
	}
	if err := h.repo.SetUserRole(r.Context(), id, body.Role); err != nil {
		notFoundOr(w, err, "User not found", "change role")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Role updated"})
}

// PUT /api/admin/users/{id}/release-cap  (admin) — raise or lower one person's
// in-flight cap. A null cap restores the TRANSLATOR_RELEASE_CAP default; 0
// removes the cap for that user entirely.
func (h *Handler) setUserReleaseCap(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	var body struct {
		Cap *int `json:"cap"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "cap must be a number or null")
		return
	}
	if body.Cap != nil && (*body.Cap < 0 || *body.Cap > 1000) {
		httpx.Error(w, http.StatusBadRequest, "cap must be between 0 and 1000 (0 = fără limită)")
		return
	}
	if err := h.repo.SetReleaseCap(r.Context(), id, body.Cap); err != nil {
		notFoundOr(w, err, "User not found", "set release cap")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Release cap updated"})
}

// blockBanned rejects writes from banned users. Bans bite at the next write,
// not by token revocation — JWTs stay stateless.
func (h *Handler) blockBanned(w http.ResponseWriter, r *http.Request) bool {
	banned, err := h.repo.IsUserBanned(r.Context(), middleware.UserFrom(r).UserID)
	if err != nil {
		httpx.Internal(w, "check ban", err)
		return true
	}
	if banned {
		httpx.Error(w, http.StatusForbidden, "Contul este suspendat.")
		return true
	}
	return false
}

// ── episode reports ───────────────────────────────────────────────────────────

type episodeReportBody struct {
	Body string `json:"body"`
}

// POST /api/episodes/{id}/report — any signed-in member.
func (h *Handler) reportEpisode(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}
	var body episodeReportBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	text := strings.TrimSpace(body.Body)
	if text == "" {
		httpx.Error(w, http.StatusBadRequest, "Scrie câteva cuvinte despre problemă.")
		return
	}
	// Mirrors the DB constraint. Counted in runes, not bytes: a Romanian
	// report full of diacritics must not be rejected for being "too long"
	// when it is well under the limit as the member sees it.
	if len([]rune(text)) > 2000 {
		httpx.Error(w, http.StatusBadRequest, "Raportul e prea lung (maxim 2000 de caractere).")
		return
	}
	claims := middleware.UserFrom(r)
	if _, err := h.repo.CreateEpisodeReport(r.Context(), id, claims.UserID, text); err != nil {
		notFoundOr(w, err, "Episode not found", "create episode report")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"message": "Raport trimis"})
}

// GET /api/admin/episode-reports  (admin/moderator)
func (h *Handler) listEpisodeReports(w http.ResponseWriter, r *http.Request) {
	limit := httpx.QueryInt(r, "limit", 50, 1, 200)
	offset := httpx.QueryInt(r, "offset", 0, 0, 1_000_000)
	rows, total, err := h.repo.EpisodeReports(r.Context(), r.URL.Query().Get("status"), limit, offset)
	if err != nil {
		httpx.Internal(w, "fetch episode reports", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows, "total": total})
}

// POST /api/admin/episode-reports/{id}/resolve  (admin/moderator)
func (h *Handler) resolveEpisodeReport(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid report ID")
		return
	}
	claims := middleware.UserFrom(r)
	if err := h.repo.ResolveEpisodeReport(r.Context(), id, claims.UserID); err != nil {
		notFoundOr(w, err, "No open report with that id", "resolve episode report")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Raport rezolvat"})
}
