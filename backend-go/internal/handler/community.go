package handler

// The /comunitate hub API: real member list, all-users reviews, friends-only
// activity, stats, the team roster, and the persisted forum. Read endpoints
// are public (optionalAuth for personalisation); writing needs auth.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/repo"
)

const (
	communityListLimit = 40
	forumListLimit     = 60
)

// forumCategories is the fixed allowlist (the "Toate" filter is the frontend's
// "no category" and is never stored).
var forumCategories = map[string]bool{
	"Discuții": true, "Recomandări": true, "Teorii": true, "Meta": true, "Ajutor": true,
}

// GET /api/community/members — real accounts, most active first.
func (h *Handler) communityMembers(w http.ResponseWriter, r *http.Request) {
	viewer := 0
	if v := viewerID(r); v != nil {
		viewer = *v
	}
	rows, err := h.repo.CommunityMembers(r.Context(), viewer, communityListLimit)
	if err != nil {
		httpx.Internal(w, "community members", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GET /api/community/reviews — every member's reviews, newest first.
func (h *Handler) communityReviews(w http.ResponseWriter, r *http.Request) {
	rows, err := h.repo.CommunityReviews(r.Context(), communityListLimit)
	if err != nil {
		httpx.Internal(w, "community reviews", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GET /api/community/activity?scope=friends|site — the activity feed.
//
// `friends` (the default) is the /comunitate tab: mutual follows only, and an
// empty friend set returns an empty list rather than an error. `scope=site` is
// what /home shows — the whole community, because a member who has not followed
// anyone yet would otherwise get a permanently empty strip on the front page.
func (h *Handler) communityActivity(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("scope") == "site" {
		rows, err := h.repo.SiteActivity(r.Context(), communityListLimit)
		if err != nil {
			httpx.Internal(w, "site activity", err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"data": rows, "scope": "site"})
		return
	}

	uid := middleware.UserFrom(r).UserID
	friends, err := h.repo.FriendIDs(r.Context(), uid)
	if err != nil {
		httpx.Internal(w, "friend ids", err)
		return
	}
	rows, err := h.repo.FriendsActivity(r.Context(), friends, communityListLimit)
	if err != nil {
		httpx.Internal(w, "friends activity", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows, "friends": len(friends), "scope": "friends"})
}

// GET /api/community/stats
func (h *Handler) communityStats(w http.ResponseWriter, r *http.Request) {
	s, err := h.repo.CommunityStats(r.Context())
	if err != nil {
		httpx.Internal(w, "community stats", err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

// GET /api/community/hall-of-fame?window=month|all — the translator and
// verifier leaderboards, ranked by published releases.
func (h *Handler) communityHallOfFame(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	if window != "month" {
		window = "all"
	}
	translators, verifiers, err := h.repo.HallOfFame(r.Context(), window, 20)
	if err != nil {
		httpx.Internal(w, "hall of fame", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"translators": translators, "verifiers": verifiers, "window": window,
	})
}

// GET /api/community/team
func (h *Handler) communityTeam(w http.ResponseWriter, r *http.Request) {
	rows, err := h.repo.CommunityTeam(r.Context())
	if err != nil {
		httpx.Internal(w, "community team", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// ── Forum ────────────────────────────────────────────────────────────────────

// GET /api/community/forum?category= — thread list (body stripped).
func (h *Handler) listForumThreads(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	if category == "Toate" {
		category = ""
	}
	if category != "" && !forumCategories[category] {
		httpx.Error(w, http.StatusBadRequest, "Categorie necunoscută")
		return
	}
	rows, err := h.repo.ListThreads(r.Context(), category, forumListLimit)
	if err != nil {
		httpx.Internal(w, "list threads", err)
		return
	}
	for i := range rows {
		rows[i].Body = "" // list view doesn't carry the body
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// POST /api/community/forum — open a thread (any authed, non-banned member).
func (h *Handler) createForumThread(w http.ResponseWriter, r *http.Request) {
	if h.blockBanned(w, r) {
		return
	}
	var body struct {
		Category string `json:"category"`
		Title    string `json:"title"`
		Body     string `json:"body"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	title := strings.TrimSpace(body.Title)
	text := strings.TrimSpace(body.Body)
	if len([]rune(title)) < 3 || len([]rune(title)) > 160 {
		httpx.Error(w, http.StatusBadRequest, "Titlul trebuie să aibă între 3 și 160 de caractere")
		return
	}
	if text == "" || len([]rune(text)) > 8000 {
		httpx.Error(w, http.StatusBadRequest, "Mesajul e gol sau prea lung (max 8000)")
		return
	}
	if !forumCategories[body.Category] {
		httpx.Error(w, http.StatusBadRequest, "Alege o categorie validă")
		return
	}
	thread, err := h.repo.CreateThread(r.Context(), middleware.UserFrom(r).UserID, body.Category, title, text)
	if err != nil {
		httpx.Internal(w, "create thread", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": thread})
}

// GET /api/community/forum/{id} — thread + replies.
func (h *Handler) getForumThread(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid thread ID")
		return
	}
	thread, err := h.repo.ThreadByID(r.Context(), id)
	if err != nil {
		notFoundOr(w, err, "Subiectul nu există", "get thread")
		return
	}
	replies, err := h.repo.ThreadReplies(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "thread replies", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{"thread": thread, "replies": replies}})
}

// POST /api/community/forum/{id}/replies — reply (authed, non-banned).
func (h *Handler) createForumReply(w http.ResponseWriter, r *http.Request) {
	if h.blockBanned(w, r) {
		return
	}
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid thread ID")
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	text := strings.TrimSpace(body.Body)
	if text == "" || len([]rune(text)) > 8000 {
		httpx.Error(w, http.StatusBadRequest, "Mesajul e gol sau prea lung (max 8000)")
		return
	}
	reply, err := h.repo.CreateThreadReply(r.Context(), id, middleware.UserFrom(r).UserID, text)
	if errors.Is(err, repo.ErrLocked) {
		httpx.Error(w, http.StatusConflict, "Subiectul e blocat")
		return
	}
	if err != nil {
		notFoundOr(w, err, "Subiectul nu există", "create reply")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": reply})
}

// POST /api/community/forum/{id}/pin  body {pinned} — moderator toggle.
func (h *Handler) pinForumThread(w http.ResponseWriter, r *http.Request) {
	h.threadFlag(w, r, "pinned")
}

// POST /api/community/forum/{id}/lock  body {locked} — moderator toggle.
func (h *Handler) lockForumThread(w http.ResponseWriter, r *http.Request) {
	h.threadFlag(w, r, "locked")
}

func (h *Handler) threadFlag(w http.ResponseWriter, r *http.Request, flag string) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid thread ID")
		return
	}
	var body struct {
		Pinned bool `json:"pinned"`
		Locked bool `json:"locked"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	var err error
	if flag == "pinned" {
		err = h.repo.SetThreadPinned(r.Context(), id, body.Pinned)
	} else {
		err = h.repo.SetThreadLocked(r.Context(), id, body.Locked)
	}
	if err != nil {
		notFoundOr(w, err, "Subiectul nu există", "flag thread")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// DELETE /api/community/forum/{id} — author or moderator-class.
func (h *Handler) deleteForumThread(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid thread ID")
		return
	}
	author, err := h.repo.ThreadAuthor(r.Context(), id)
	if err != nil {
		notFoundOr(w, err, "Subiectul nu există", "delete thread")
		return
	}
	if author != middleware.UserFrom(r).UserID && !canReview(r) {
		httpx.Error(w, http.StatusForbidden, "Doar autorul sau un moderator poate șterge subiectul")
		return
	}
	if err := h.repo.DeleteThread(r.Context(), id); err != nil {
		notFoundOr(w, err, "Subiectul nu există", "delete thread")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Subiect șters"})
}
