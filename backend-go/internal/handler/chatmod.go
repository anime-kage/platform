package handler

// Chat moderation: timeouts and bans that apply to the live chat
// only. Nothing here touches users.banned_at — a chat ban must never read as
// an account suspension, and vice versa.

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
)

// chatModRoles is who may hand out a timeout or a ban. Staff moderate the room
// they work in: the people running releases are the ones actually reading chat
// at 2am, so waiting for a moderator to show up would mean nobody moderates.
var chatModRoles = []string{"translator", "verifier", "coordinator", "moderator", "admin"}

// chatRank orders staff so nobody can mute sideways or upwards — a translator
// silencing a coordinator (or two admins muting each other in a disagreement)
// is the failure mode that makes handing out this power expensive. Restricting
// someone requires a *strictly* higher rank than theirs.
var chatRank = map[string]int{
	"user":        0,
	"translator":  1,
	"verifier":    1,
	"coordinator": 2,
	"moderator":   2,
	"admin":       3,
}

// chatMaxTimeout is the ceiling on a timeout. Past this the honest action is a
// ban, which is visible as one and can be lifted as one — a 3-month "timeout"
// is a ban nobody remembers issuing.
const chatMaxTimeout = 14 * 24 * time.Hour

func isChatMod(role string) bool {
	for _, r := range chatModRoles {
		if r == role {
			return true
		}
	}
	return false
}

// blockChatRestricted rejects a send from a muted user, telling them how long
// is left. Reported as 403 rather than 429: this is not a rate limit, and the
// panel must not offer to retry in a second.
func (h *Handler) blockChatRestricted(w http.ResponseWriter, r *http.Request, userID int) bool {
	c, err := h.repo.ChatRestrictionFor(r.Context(), userID)
	if err != nil {
		httpx.Internal(w, "check chat restriction", err)
		return true
	}
	if c == nil {
		return false
	}
	msg := "Ai fost blocat permanent din chat."
	if c.ExpiresAt != nil {
		msg = "Ai primit timeout în chat. Mai ai " + humanRemaining(time.Until(*c.ExpiresAt)) + "."
	}
	if c.Reason != nil && *c.Reason != "" {
		msg += " Motiv: " + *c.Reason
	}
	httpx.Error(w, http.StatusForbidden, msg)
	return true
}

// humanRemaining renders a duration the way a muted user needs to read it —
// rounded up, so "0 secunde" never appears on a mute that is still in force.
func humanRemaining(d time.Duration) string {
	if d < time.Minute {
		s := int(d.Seconds()) + 1
		return fmt.Sprintf("%d secunde", s)
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minute", int(d.Minutes())+1)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d ore", int(d.Hours())+1)
	}
	return fmt.Sprintf("%d zile", int(d.Hours()/24)+1)
}

// chatModTarget resolves {username} and enforces the rank rule. It returns the
// target's id, or false having already written the response.
func (h *Handler) chatModTarget(w http.ResponseWriter, r *http.Request) (int, bool) {
	actor := middleware.UserFrom(r)
	target, err := h.repo.UserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		notFoundOr(w, err, "Utilizatorul nu există", "chat moderation target")
		return 0, false
	}
	if target.ID == actor.UserID {
		httpx.Error(w, http.StatusBadRequest, "Nu te poți modera pe tine.")
		return 0, false
	}
	if chatRank[actor.Role] <= chatRank[target.Role] {
		httpx.Error(w, http.StatusForbidden,
			"Nu poți modera un membru cu rol egal sau superior.")
		return 0, false
	}
	return target.ID, true
}

// GET /api/chat/restrictions/{username}  (chat staff) — what the moderation
// card shows before offering any button.
func (h *Handler) chatRestriction(w http.ResponseWriter, r *http.Request) {
	u, err := h.repo.UserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		notFoundOr(w, err, "Utilizatorul nu există", "chat restriction")
		return
	}
	c, err := h.repo.ChatRestrictionFor(r.Context(), u.ID)
	if err != nil {
		httpx.Internal(w, "fetch chat restriction", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": c})
}

type chatRestrictBody struct {
	// 0 (or absent) means a permanent ban; anything else is a timeout
	Seconds int     `json:"seconds"`
	Reason  *string `json:"reason"`
}

// PUT /api/chat/restrictions/{username}  (chat staff) — timeout or ban.
func (h *Handler) setChatRestriction(w http.ResponseWriter, r *http.Request) {
	var body chatRestrictBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Date invalide")
		return
	}
	if body.Seconds < 0 || time.Duration(body.Seconds)*time.Second > chatMaxTimeout {
		httpx.Error(w, http.StatusBadRequest,
			"Timeout-ul poate fi de cel mult 14 zile — peste atât, folosește ban.")
		return
	}
	if body.Reason != nil {
		reason := strings.TrimSpace(*body.Reason)
		if len([]rune(reason)) > 200 {
			reason = string([]rune(reason)[:200])
		}
		if reason == "" {
			body.Reason = nil
		} else {
			body.Reason = &reason
		}
	}

	targetID, ok := h.chatModTarget(w, r)
	if !ok {
		return
	}
	var until *time.Time
	if body.Seconds > 0 {
		t := time.Now().Add(time.Duration(body.Seconds) * time.Second)
		until = &t
	}
	actor := middleware.UserFrom(r)
	if err := h.repo.SetChatRestriction(r.Context(), targetID, until, actor.UserID, body.Reason); err != nil {
		httpx.Internal(w, "set chat restriction", err)
		return
	}

	msg := "Membru blocat din chat."
	if until != nil {
		msg = "Timeout aplicat: " + humanRemaining(time.Until(*until)) + "."
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": msg})
}

// DELETE /api/chat/restrictions/{username}  (chat staff) — lift it early.
func (h *Handler) clearChatRestriction(w http.ResponseWriter, r *http.Request) {
	targetID, ok := h.chatModTarget(w, r)
	if !ok {
		return
	}
	if err := h.repo.ClearChatRestriction(r.Context(), targetID); err != nil {
		notFoundOr(w, err, "Membrul nu are nicio restricție activă", "clear chat restriction")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Restricție ridicată."})
}
