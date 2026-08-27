package handler

// Auto-translate endpoint. POST /api/releases/{id}/translate kicks
// off a background run that fills empty ro_text rows window by window with
// Claude; the editor polls the events endpoint and watches rows appear.
// Guarantees: only rows still empty at write time are filled (human edits
// win the race), machine rows keep edited=false so the grid can mark them,
// and one release never has two concurrent runs.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/translate"
)

// translationStatus tracks one release's auto-translate run so the editor can
// show progress and, crucially, learn WHY a run stopped (it used to fail
// silently in the goroutine). Kept in h.translating after the run ends so the
// editor's next poll sees the final done/error state; overwritten on the next run.
type translationStatus struct {
	mu       sync.Mutex
	running  bool
	total    int
	filled   int
	err      string // user-facing failure reason, empty on success
	finished time.Time
}

// StatusView is the JSON snapshot the status endpoint returns.
type StatusView struct {
	Running bool   `json:"running"`
	Total   int    `json:"total"`
	Filled  int    `json:"filled"`
	Done    bool   `json:"done"`
	Error   string `json:"error,omitempty"`
}

func (s *translationStatus) snapshot() StatusView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StatusView{
		Running: s.running,
		Total:   s.total,
		Filled:  s.filled,
		Done:    !s.running && s.err == "" && !s.finished.IsZero(),
		Error:   s.err,
	}
}

func (s *translationStatus) addFilled(n int) {
	s.mu.Lock()
	s.filled += n
	s.mu.Unlock()
}

func (s *translationStatus) finish(errMsg string) {
	s.mu.Lock()
	s.running = false
	s.err = errMsg
	s.finished = time.Now()
	s.mu.Unlock()
}

// translateErrorReason turns a raw Anthropic/API error into a short Romanian
// message the translator can act on (the full error still goes to the log).
func translateErrorReason(err error) string {
	e := strings.ToLower(err.Error())
	switch {
	case strings.Contains(e, "credit balance is too low"):
		return "Fonduri Anthropic insuficiente — adaugă credit în contul Anthropic."
	case strings.Contains(e, "401"), strings.Contains(e, "authentication"), strings.Contains(e, "invalid x-api-key"):
		return "Cheia ANTHROPIC_API_KEY e invalidă."
	case strings.Contains(e, "rate limit"), strings.Contains(e, "429"):
		return "Prea multe cereri către Anthropic — încearcă din nou peste un minut."
	case strings.Contains(e, "not_found_error"), strings.Contains(e, "model:"):
		return "Modelul de traducere nu e disponibil (verifică TRANSLATE_MODEL)."
	default:
		return "Traducerea automată a eșuat — vezi log-ul serverului."
	}
}

// POST /api/releases/{id}/translate  (translator+, own release)
func (h *Handler) translateRelease(w http.ResponseWriter, r *http.Request) {
	if h.translator == nil {
		httpx.Error(w, http.StatusServiceUnavailable,
			"Traducerea automată nu e configurată (lipsește ANTHROPIC_API_KEY)")
		return
	}
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	editable := rel.State == "draft" || rel.State == "changes_requested" ||
		(rel.State == "in_review" && canReview(r))
	if !editable {
		httpx.Error(w, http.StatusConflict, "Release is not editable in state '"+rel.State+"'")
		return
	}

	events, err := h.repo.EventsByRelease(r.Context(), rel.ID)
	if err != nil {
		httpx.Internal(w, "fetch events", err)
		return
	}
	var pending []translate.Line
	for _, ev := range events {
		if ev.RoText == "" && ev.EnText != "" {
			pending = append(pending, translate.Line{Index: ev.Idx, Text: ev.EnText})
		}
	}
	if len(pending) == 0 {
		httpx.Error(w, http.StatusBadRequest, "Nimic de tradus — toate replicile au deja text în română")
		return
	}

	// claim the run: a fresh status replaces a stale finished one, but a live
	// run blocks a second launch
	st := &translationStatus{running: true, total: len(pending)}
	if prev, loaded := h.translating.LoadOrStore(rel.ID, st); loaded {
		if prev.(*translationStatus).snapshot().Running {
			httpx.Error(w, http.StatusConflict, "O traducere automată rulează deja pentru acest release")
			return
		}
		h.translating.Store(rel.ID, st) // overwrite the old result
	}

	// series context: catalog title + glossary when linked, else the
	// translator's proposed title (no glossary exists yet for those)
	var seriesContext string
	if rel.AnimeID != nil {
		title, glossary, err := h.repo.TranslationContext(r.Context(), *rel.AnimeID)
		if err != nil {
			h.translating.Delete(rel.ID)
			httpx.Internal(w, "fetch glossary", err)
			return
		}
		seriesContext = "Series: " + title
		if glossary != nil && *glossary != "" {
			seriesContext += "\n" + *glossary
		}
	} else if rel.ProposedTitle != nil {
		seriesContext = "Series: " + *rel.ProposedTitle
	}

	go h.runTranslation(rel.ID, rel.UploaderID, releaseLabel(rel), seriesContext, pending, st)

	httpx.JSON(w, http.StatusAccepted, map[string]any{
		"message": "Traducerea automată a pornit",
		"pending": len(pending),
	})
}

// GET /api/releases/{id}/translate/status — progress + result of the current
// (or last) auto-translate run, so the editor can show a bar and surface errors.
func (h *Handler) translationStatusHandler(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	view := StatusView{}
	if v, ok := h.translating.Load(rel.ID); ok {
		view = v.(*translationStatus).snapshot()
	}
	httpx.JSON(w, http.StatusOK, view)
}

// runTranslation is the background worker: one window at a time, writing rows
// as each window lands so the editor's progress bar moves. It records progress
// and any failure reason on st (the editor polls it) and drops the uploader an
// in-app notification when the run ends — done or failed — so a silent
// background death (e.g. no Anthropic credit) can no longer go unnoticed.
func (h *Handler) runTranslation(releaseID, uploaderID int, label, seriesContext string, pending []translate.Line, st *translationStatus) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	start := time.Now()
	link := fmt.Sprintf("/translate/%d", releaseID)

	for _, window := range translate.Windows(pending) {
		lines, err := h.translator.TranslateWindow(ctx, seriesContext, window)
		if err != nil {
			reason := translateErrorReason(err)
			slog.Error("auto-translate window failed", "release", releaseID,
				"filled", st.snapshot().Filled, "err", err)
			st.finish(reason)
			h.notify(context.Background(), uploaderID, "release",
				"Traducerea automată pentru "+label+" a eșuat: "+reason, nil, &link)
			return
		}
		for _, l := range lines {
			if err := h.repo.FillEventRO(ctx, releaseID, l.Index, l.Text); err != nil {
				slog.Error("auto-translate write failed", "release", releaseID, "idx", l.Index, "err", err)
				st.finish("Scrierea traducerii a eșuat — încearcă din nou.")
				h.notify(context.Background(), uploaderID, "release",
					"Traducerea automată pentru "+label+" a eșuat.", nil, &link)
				return
			}
			st.addFilled(1)
		}
	}
	st.finish("") // success
	filled := st.snapshot().Filled
	slog.Info("auto-translate done", "release", releaseID, "filled", filled,
		"of", len(pending), "took", time.Since(start).Round(time.Second))
	h.notify(context.Background(), uploaderID, "release",
		fmt.Sprintf("Traducerea automată pentru %s e gata — %d replici completate.", label, filled), nil, &link)
}
