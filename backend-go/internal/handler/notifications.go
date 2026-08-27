package handler

// In-app notifications: the header bell + the /notificari inbox. Rows are
// written by the event handlers through h.notify (best-effort — a failed
// notification is logged, never surfaced, and never fails the action).

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
)

const notifListLimit = 40

// notify writes one inbox row for userID. It is deliberately forgiving:
// self-directed events are skipped (you don't get pinged for replying to
// yourself), and any DB error is logged rather than returned so the caller's
// main action still succeeds.
func (h *Handler) notify(ctx context.Context, userID int, typ, body string, actorID *int, link *string) {
	if userID <= 0 {
		return
	}
	if actorID != nil && *actorID == userID {
		return
	}
	if err := h.repo.CreateNotification(ctx, userID, typ, body, actorID, link); err != nil {
		slog.Warn("create notification", "userId", userID, "type", typ, "err", err)
	}
}

// GET /api/notifications — the inbox: recent rows plus the unread count.
func (h *Handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserFrom(r).UserID
	items, err := h.repo.Notifications(r.Context(), uid, notifListLimit)
	if err != nil {
		httpx.Internal(w, "list notifications", err)
		return
	}
	unread, err := h.repo.UnreadCount(r.Context(), uid)
	if err != nil {
		httpx.Internal(w, "count notifications", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": items, "unread": unread})
}

// GET /api/notifications/unread-count — the light poll for the badge.
func (h *Handler) notificationsUnread(w http.ResponseWriter, r *http.Request) {
	unread, err := h.repo.UnreadCount(r.Context(), middleware.UserFrom(r).UserID)
	if err != nil {
		httpx.Internal(w, "count notifications", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"unread": unread})
}

// POST /api/notifications/read-all — clears the badge.
func (h *Handler) markAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.MarkAllRead(r.Context(), middleware.UserFrom(r).UserID); err != nil {
		httpx.Internal(w, "mark notifications read", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// POST /api/notifications/{id}/read — clears a single row (e.g. on click).
func (h *Handler) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid notification ID")
		return
	}
	if err := h.repo.MarkNotificationRead(r.Context(), middleware.UserFrom(r).UserID, id); err != nil {
		httpx.Internal(w, "mark notification read", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// Largest fan-out a single publish may cause. A guard, not a policy: the
// watchlist cannot plausibly hold more than this for one series today, and if
// it ever does, that is worth seeing in the log before it becomes thousands of
// rows written inside one request.
const maxPublishFanout = 500

// notifyWatchers tells everyone tracking a series that a new episode is live.
//
// Best-effort, exactly like notify(): a failure here must never turn a
// successful publish into an error for the coordinator who performed it.
func (h *Handler) notifyWatchers(ctx context.Context, animeID, epNum int, link string, uploaderID, publisherID int) {
	// The uploader already got their own "your translation is live" message and
	// the publisher just pressed the button; neither needs telling twice.
	ids, err := h.repo.AnimeWatcherIDs(ctx, animeID, []int{uploaderID, publisherID})
	if err != nil {
		slog.Warn("notify watchers", "animeId", animeID, "err", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	if len(ids) > maxPublishFanout {
		slog.Warn("publish fan-out capped",
			"animeId", animeID, "watchers", len(ids), "cap", maxPublishFanout)
		ids = ids[:maxPublishFanout]
	}

	title := "Serie urmărită"
	if a, err := h.repo.AnimeByID(ctx, animeID); err == nil && a != nil && a.Title != "" {
		title = a.Title
	}
	body := fmt.Sprintf("%s — episodul %d a fost publicat cu subtitrare RO.", title, epNum)
	for _, uid := range ids {
		h.notify(ctx, uid, "release", body, nil, &link)
	}
}
