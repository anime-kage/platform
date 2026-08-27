package handler

// Live chat endpoints: a short backlog on open, an SSE stream to
// follow, an ordinary POST to send, and a moderator delete.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
)

const (
	chatMaxLen      = 500
	chatBacklog     = 60 // what a newly-opened panel shows
	chatMaxBacklog  = 200
	chatExcerptMax  = 80
	chatMinInterval = 750 * time.Millisecond // per-user floor between messages
	chatBurst       = 5                      // …and a short burst allowance
	chatBurstWindow = 10 * time.Second
)

// chatLimiter is a per-user send throttle. The per-IP middleware limiter is too
// coarse here: a shared connection would punish everyone behind it, and chat
// wants a much tighter budget than the rest of the API.
type chatLimiter struct {
	mu   sync.Mutex
	seen map[int][]time.Time
}

func newChatLimiter() *chatLimiter { return &chatLimiter{seen: make(map[int][]time.Time)} }

// allow reports whether this user may send now, and how long to wait if not.
func (l *chatLimiter) allow(userID int) (time.Duration, bool) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	hits := l.seen[userID]
	kept := hits[:0]
	for _, t := range hits {
		if now.Sub(t) < chatBurstWindow {
			kept = append(kept, t)
		}
	}
	if n := len(kept); n > 0 {
		if wait := chatMinInterval - now.Sub(kept[n-1]); wait > 0 {
			l.seen[userID] = kept
			return wait, false
		}
	}
	if len(kept) >= chatBurst {
		l.seen[userID] = kept
		return chatBurstWindow - now.Sub(kept[0]), false
	}
	l.seen[userID] = append(kept, now)

	// the map would otherwise grow with every user who ever chatted
	if len(l.seen) > 5000 {
		for id, ts := range l.seen {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > chatBurstWindow {
				delete(l.seen, id)
			}
		}
	}
	return 0, true
}

// GET /api/chat/messages — the backlog a panel shows on open.
func (h *Handler) chatMessages(w http.ResponseWriter, r *http.Request) {
	limit := httpx.QueryInt(r, "limit", chatBacklog, 1, chatMaxBacklog)
	msgs, err := h.repo.RecentChatMessages(r.Context(), limit)
	if err != nil {
		httpx.Internal(w, "fetch chat messages", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data":    msgs,
		"viewers": h.chat.viewerCount(),
	})
}

type chatSendBody struct {
	Body           string  `json:"body"`
	ReplyToUser    *string `json:"replyToUser"`
	ReplyToExcerpt *string `json:"replyToExcerpt"`
	ReplyToID      *int64  `json:"replyToId"`
}

// POST /api/chat/messages
func (h *Handler) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFrom(r)
	if h.blockBanned(w, r) {
		return
	}
	var body chatSendBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Mesaj invalid")
		return
	}
	if h.blockChatRestricted(w, r, u.UserID) {
		return
	}
	text := strings.TrimSpace(body.Body)
	if text == "" {
		httpx.Error(w, http.StatusBadRequest, "Mesajul e gol")
		return
	}
	// count runes, not bytes: Romanian diacritics are two bytes each and a
	// byte limit would silently cut a message short of the advertised 500
	if utf8.RuneCountInString(text) > chatMaxLen {
		httpx.Error(w, http.StatusBadRequest,
			fmt.Sprintf("Mesajul e prea lung (max %d caractere)", chatMaxLen))
		return
	}
	if wait, ok := h.chatLimit.allow(u.UserID); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		httpx.Error(w, http.StatusTooManyRequests, "Prea multe mesaje — încetinește puțin.")
		return
	}

	replyUser, replyExcerpt := trimReply(body.ReplyToUser, body.ReplyToExcerpt)
	msg, err := h.repo.InsertChatMessage(r.Context(), u.UserID, text, replyUser, replyExcerpt, body.ReplyToID)
	if err != nil {
		httpx.Internal(w, "insert chat message", err)
		return
	}
	h.publishChat("message", msg)
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": msg})
}

// trimReply normalises the quoted context. Both halves must be present or the
// quote is dropped — a reply header with no author renders as noise.
func trimReply(user, excerpt *string) (*string, *string) {
	if user == nil || excerpt == nil {
		return nil, nil
	}
	u := strings.TrimSpace(*user)
	e := strings.TrimSpace(*excerpt)
	if u == "" || e == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(e) > chatExcerptMax {
		e = string([]rune(e)[:chatExcerptMax]) + "…"
	}
	return &u, &e
}

// DELETE /api/chat/messages/{id} — moderators and admins, or your own message.
func (h *Handler) deleteChatMessage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		httpx.Error(w, http.StatusBadRequest, "Invalid message ID")
		return
	}
	u := middleware.UserFrom(r)
	author, err := h.repo.ChatMessageAuthor(r.Context(), id)
	if err != nil {
		notFoundOr(w, err, "Mesajul nu există", "chat message author")
		return
	}
	// Anyone who can time a member out can also clear the message that earned
	// it — the two are one action in practice. The rank rule guards the
	// person-level tools (chatmod.go); a single deleted line does not need it.
	if !isChatMod(u.Role) && author != u.UserID {
		httpx.Error(w, http.StatusForbidden, "Poți șterge doar mesajele tale")
		return
	}
	if err := h.repo.DeleteChatMessage(r.Context(), id); err != nil {
		notFoundOr(w, err, "Mesajul nu există", "delete chat message")
		return
	}
	h.publishChat("delete", map[string]any{"id": id})
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Mesaj șters"})
}

// GET /api/chat/stream — SSE. Frames are pre-encoded once and shared by every
// subscriber; the hub never re-marshals per connection.
func (h *Handler) chatStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, http.StatusInternalServerError, "Streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// nginx/caddy must not sit on the stream waiting for a full buffer
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch, unsubscribe := h.subscribeChat()
	// Defers run LIFO, so this pair must be registered in this order: the
	// viewer has to be removed from the hub *before* we broadcast the new
	// count, or every disconnect reports a number one too high.
	defer h.publishViewers()
	defer unsubscribe()

	// tell everyone the viewer count moved, this connection included
	h.publishViewers()

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Comment pings keep intermediaries from reaping an idle stream and let the
	// writer notice a client that vanished without a FIN.
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case frame, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// publishChat encodes one SSE frame and hands it to the hub.
func (h *Handler) publishChat(event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal chat frame", "event", event, "err", err)
		return
	}
	h.chat.publish([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)))
}

func (h *Handler) publishViewers() {
	h.publishChat("viewers", map[string]any{"viewers": h.chat.viewerCount()})
}

func (h *Handler) subscribeChat() (<-chan []byte, func()) { return h.chat.subscribe() }

// chatRetentionDays is how long a message survives. The backlog exists so an
// opening panel is not blank, not to be an archive — but the rows are tiny
// (a 500-char cap, ~250 bytes with overhead), so a month costs single-digit
// megabytes even at thousands of messages a day. Short retention would buy
// nothing and would routinely show newcomers an empty room.
const chatRetentionDays = 30

// PurgeOldChat is the retention sweep, run by cmd/api alongside the staging
// janitor.
func (h *Handler) PurgeOldChat(ctx context.Context) {
	n, err := h.repo.PurgeChatBefore(ctx, time.Now().AddDate(0, 0, -chatRetentionDays))
	if err != nil {
		slog.Error("purge old chat", "err", err)
		return
	}
	if n > 0 {
		slog.Info("purged old chat messages", "deleted", n, "olderThanDays", chatRetentionDays)
	}

	// Lapsed timeouts are already inert (the send path filters on expires_at);
	// this only stops the table growing by one row per timeout ever issued.
	if n, err := h.repo.PurgeExpiredChatRestrictions(ctx); err != nil {
		slog.Error("purge expired chat restrictions", "err", err)
	} else if n > 0 {
		slog.Info("purged lapsed chat timeouts", "deleted", n)
	}
}
