package handler

// Threaded comments with like/dislike votes, scoped to a series, an episode/
// chapter, or a review thread.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
)

// GET /api/anime/{id}/comments | /api/manga/{id}/comments
func (h *Handler) listAnimeComments(w http.ResponseWriter, r *http.Request) {
	h.listComments(w, r, "anime")
}

func (h *Handler) listMangaComments(w http.ResponseWriter, r *http.Request) {
	h.listComments(w, r, "manga")
}

func (h *Handler) listComments(w http.ResponseWriter, r *http.Request, kind string) {
	titleID, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid "+kind+" ID")
		return
	}
	page := httpx.QueryInt(r, "page", 1, 1, 1<<30)
	limit := httpx.QueryInt(r, "limit", 20, 1, 50)

	scope := repo.CommentScope{}
	if kind == "anime" {
		scope.AnimeID = &titleID
		if id, ok := httpx.IntParam(r.URL.Query().Get("episodeId")); ok && r.URL.Query().Get("episodeId") != "" {
			scope.EpisodeID = &id
		}
	} else {
		scope.MangaID = &titleID
		if id, ok := httpx.IntParam(r.URL.Query().Get("chapterId")); ok && r.URL.Query().Get("chapterId") != "" {
			scope.ChapterID = &id
		}
	}
	if id, ok := httpx.IntParam(r.URL.Query().Get("reviewId")); ok && r.URL.Query().Get("reviewId") != "" {
		scope.ReviewID = &id
	}

	rows, total, err := h.repo.Comments(r.Context(), scope, viewerID(r), page, limit)
	if err != nil {
		httpx.Internal(w, "fetch comments", err)
		return
	}
	httpx.Paginated(w, rows, page, limit, total)
}

// POST /api/anime/{id}/comments | /api/manga/{id}/comments
func (h *Handler) postAnimeComment(w http.ResponseWriter, r *http.Request) {
	h.postComment(w, r, "anime")
}

func (h *Handler) postMangaComment(w http.ResponseWriter, r *http.Request) {
	h.postComment(w, r, "manga")
}

func (h *Handler) postComment(w http.ResponseWriter, r *http.Request, kind string) {
	if h.blockBanned(w, r) {
		return
	}
	titleID, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid "+kind+" ID")
		return
	}
	var body struct {
		Content   string `json:"content"`
		EpisodeID *int   `json:"episodeId"`
		ChapterID *int   `json:"chapterId"`
		ReviewID  *int   `json:"reviewId"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Comment content is required")
		return
	}
	content, ok := validComment(w, body.Content, "Comment")
	if !ok {
		return
	}

	if body.ReviewID != nil {
		belongs, err := h.repo.ReviewBelongsToTitle(r.Context(), kind, *body.ReviewID, titleID)
		if err != nil {
			httpx.Internal(w, "post comment", err)
			return
		}
		if !belongs {
			httpx.Error(w, http.StatusNotFound, "Review not found for this "+kind)
			return
		}
	}

	subID := body.EpisodeID
	if kind == "manga" {
		subID = body.ChapterID
	}
	comment, err := h.repo.CreateComment(r.Context(), kind,
		middleware.UserFrom(r).UserID, titleID, subID, body.ReviewID, content)
	if err != nil {
		httpx.Internal(w, "post comment", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": comment})
}

// GET /api/announcements/{id}/comments
func (h *Handler) listAnnouncementComments(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid announcement ID")
		return
	}
	page := httpx.QueryInt(r, "page", 1, 1, 1<<30)
	limit := httpx.QueryInt(r, "limit", 20, 1, 50)
	rows, total, err := h.repo.Comments(r.Context(),
		repo.CommentScope{AnnouncementID: &id}, viewerID(r), page, limit)
	if err != nil {
		httpx.Internal(w, "fetch announcement comments", err)
		return
	}
	httpx.Paginated(w, rows, page, limit, total)
}

// POST /api/announcements/{id}/comments
func (h *Handler) postAnnouncementComment(w http.ResponseWriter, r *http.Request) {
	if h.blockBanned(w, r) {
		return
	}
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid announcement ID")
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Comment content is required")
		return
	}
	content, ok := validComment(w, body.Content, "Comment")
	if !ok {
		return
	}
	comment, err := h.repo.CreateAnnouncementComment(r.Context(), id, middleware.UserFrom(r).UserID, content)
	if err != nil {
		httpx.Internal(w, "post announcement comment", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": comment})
}

// commentLink points a notification at the page the comment lives on. Returns
// nil when the comment is scoped to neither (shouldn't happen), leaving the
// notification navigable-less rather than broken.
func commentLink(c *model.Comment) *string {
	var link string
	switch {
	case c.AnimeID != nil:
		link = fmt.Sprintf("/anime/%d", *c.AnimeID)
	case c.MangaID != nil:
		link = fmt.Sprintf("/manga/%d", *c.MangaID)
	default:
		return nil
	}
	return &link
}

func validComment(w http.ResponseWriter, raw, label string) (string, bool) {
	content := strings.TrimSpace(raw)
	if content == "" {
		httpx.Error(w, http.StatusBadRequest, label+" content is required")
		return "", false
	}
	if len([]rune(content)) > 2000 {
		httpx.Error(w, http.StatusBadRequest, label+" too long (max 2000 chars)")
		return "", false
	}
	return content, true
}

// GET /api/comments/{id}/replies
func (h *Handler) commentReplies(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid comment ID")
		return
	}
	replies, err := h.repo.Replies(r.Context(), id, viewerID(r))
	if err != nil {
		httpx.Internal(w, "fetch replies", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": replies})
}

// POST /api/comments/{id}/reply
func (h *Handler) postReply(w http.ResponseWriter, r *http.Request) {
	if h.blockBanned(w, r) {
		return
	}
	parentID, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid comment ID")
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Reply content is required")
		return
	}
	content, ok := validComment(w, body.Content, "Reply")
	if !ok {
		return
	}
	actor := middleware.UserFrom(r)
	reply, err := h.repo.CreateReply(r.Context(), parentID, actor.UserID, content)
	if err != nil {
		notFoundOr(w, err, "Parent comment not found", "post reply")
		return
	}
	// ping the person you replied to (best-effort; skipped if it's yourself)
	if author, aerr := h.repo.CommentAuthorID(r.Context(), parentID); aerr == nil && author > 0 {
		link := commentLink(reply)
		h.notify(r.Context(), author, "reply",
			actor.Username+" ți-a răspuns la un comentariu.", &actor.UserID, link)
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": reply})
}

// PUT /api/comments/{id} — edit own comment
func (h *Handler) editComment(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid comment ID")
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Content is required")
		return
	}
	content := strings.TrimSpace(body.Content)
	if content == "" {
		httpx.Error(w, http.StatusBadRequest, "Content is required")
		return
	}
	if len([]rune(content)) > 2000 {
		httpx.Error(w, http.StatusBadRequest, "Comment too long")
		return
	}
	updated, err := h.repo.UpdateComment(r.Context(), id, middleware.UserFrom(r).UserID, content)
	if err != nil {
		notFoundOr(w, err, "Comment not found or not yours", "edit comment")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": updated})
}

// DELETE /api/comments/{id} — soft-delete own comment
func (h *Handler) deleteComment(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid comment ID")
		return
	}
	if err := h.repo.SoftDeleteComment(r.Context(), id, middleware.UserFrom(r).UserID); err != nil {
		notFoundOr(w, err, "Comment not found or not yours", "delete comment")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Comment deleted"})
}

// POST /api/comments/{id}/vote
func (h *Handler) voteComment(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid comment ID")
		return
	}
	var body struct {
		VoteType string `json:"voteType"`
	}
	if err := httpx.Decode(r, &body); err != nil ||
		(body.VoteType != "like" && body.VoteType != "dislike") {
		httpx.Error(w, http.StatusBadRequest, "voteType must be like or dislike")
		return
	}
	msg, vote, err := h.repo.Vote(r.Context(), id, middleware.UserFrom(r).UserID, body.VoteType)
	if err != nil {
		httpx.Internal(w, "vote", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"message": msg, "voteType": vote})
}

// POST /api/comments/{id}/report
func (h *Handler) reportComment(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid comment ID")
		return
	}
	if err := h.repo.ReportComment(r.Context(), id); err != nil {
		httpx.Internal(w, "report comment", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Comment reported"})
}
