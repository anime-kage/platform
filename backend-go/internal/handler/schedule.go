package handler

// The team-decided weekly programme. Every member reads it; admins and
// coordinators decide what goes in it.

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
)

const (
	slotNoteMax  = 120
	slotListMax  = 200
	scheduleDays = 7
)

// GET /api/schedule?days=7 — the window the home page draws.
// GET /api/schedule?upcoming=1 — everything ahead, for the admin editor.
func (h *Handler) listSchedule(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("upcoming") == "1" {
		// From the start of today, not from now: an episode scheduled for
		// 09:00 must not vanish out of the editor at 09:01, which is exactly
		// when someone wants to move it.
		rows, err := h.repo.UpcomingSchedule(r.Context(), startOfToday(), slotListMax)
		if err != nil {
			httpx.Internal(w, "upcoming schedule", err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
		return
	}

	days := httpx.QueryInt(r, "days", scheduleDays, 1, 60)
	from := startOfToday()
	rows, err := h.repo.ScheduleWindow(r.Context(), from, from.AddDate(0, 0, days))
	if err != nil {
		httpx.Internal(w, "schedule window", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// POST /api/schedule — place an episode. Rescheduling the same episode moves
// the existing slot rather than adding a second one.
func (h *Handler) createScheduleSlot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AnimeID       int    `json:"animeId"`
		EpisodeNumber int    `json:"episodeNumber"`
		ScheduledAt   string `json:"scheduledAt"`
		Note          string `json:"note"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	at, note, ok := validateSlot(w, body.ScheduledAt, body.Note, body.EpisodeNumber)
	if !ok {
		return
	}
	// The series has to exist: a slot pointing nowhere would render as a blank
	// row on the front page.
	if _, err := h.repo.AnimeByID(r.Context(), body.AnimeID); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Anime-ul nu există în catalog")
		return
	}

	slot, err := h.repo.UpsertSlot(r.Context(), body.AnimeID, body.EpisodeNumber, at, note,
		middleware.UserFrom(r).UserID)
	if err != nil {
		httpx.Internal(w, "create slot", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": slot})
}

// PUT /api/schedule/{id}
func (h *Handler) updateScheduleSlot(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid slot ID")
		return
	}
	var body struct {
		EpisodeNumber int    `json:"episodeNumber"`
		ScheduledAt   string `json:"scheduledAt"`
		Note          string `json:"note"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	at, note, ok := validateSlot(w, body.ScheduledAt, body.Note, body.EpisodeNumber)
	if !ok {
		return
	}
	slot, err := h.repo.UpdateSlot(r.Context(), id, body.EpisodeNumber, at, note)
	if err != nil {
		notFoundOr(w, err, "Programarea nu există", "update slot")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": slot})
}

// DELETE /api/schedule/{id}
func (h *Handler) deleteScheduleSlot(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid slot ID")
		return
	}
	if err := h.repo.DeleteSlot(r.Context(), id); err != nil {
		notFoundOr(w, err, "Programarea nu există", "delete slot")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"message": "Programare ștearsă"})
}

// validateSlot parses and bounds the fields shared by create and update.
func validateSlot(w http.ResponseWriter, rawAt, rawNote string, episode int) (time.Time, *string, bool) {
	if episode < 1 || episode > 10000 {
		httpx.Error(w, http.StatusBadRequest, "Numărul episodului e invalid")
		return time.Time{}, nil, false
	}
	// RFC3339 only, so the offset is explicit and the instant is unambiguous —
	// a bare "2026-08-20T18:00" would be interpreted in the server's timezone,
	// which is not the one the coordinator was thinking in.
	at, err := time.Parse(time.RFC3339, rawAt)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "Data programării e invalidă")
		return time.Time{}, nil, false
	}
	var note *string
	if t := strings.TrimSpace(rawNote); t != "" {
		if len([]rune(t)) > slotNoteMax {
			httpx.Error(w, http.StatusBadRequest, "Nota e prea lungă (max 120 de caractere)")
			return time.Time{}, nil, false
		}
		note = &t
	}
	return at, note, true
}

// startOfToday is local midnight on the server, the window both reads use.
func startOfToday() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
}
