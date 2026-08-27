package handler

// Translation requests ("Cereri"). New requests are resolved against
// Jikan/AniList so we can dedup by canonical MAL id: the same series can't be
// requested twice, and one already in the catalog is refused. Coordinators and
// admins move requests through the queue.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/jikan"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/repo"
)

var requestStatuses = map[string]bool{
	"pending": true, "in_progress": true, "approved": true, "rejected": true,
}

// noteMaxLen caps a request's "why" note (matches the frontend textarea limit).
const noteMaxLen = 300

// GET /api/requests?status=&sort=votes|recent&page=
func (h *Handler) listRequests(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && !requestStatuses[status] {
		status = ""
	}
	sort := r.URL.Query().Get("sort")
	if sort != "recent" {
		sort = "votes"
	}
	const perPage = 20
	page := httpx.QueryInt(r, "page", 1, 1, 100000)
	rows, total, err := h.repo.ListRequests(r.Context(), viewerIDOr0(r), status, sort, perPage, (page-1)*perPage)
	if err != nil {
		httpx.Internal(w, "list requests", err)
		return
	}
	pages := (total + perPage - 1) / perPage
	if pages < 1 {
		pages = 1
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data":       rows,
		"pagination": map[string]int{"page": page, "pages": pages, "total": total, "perPage": perPage},
	})
}

// searchAnimeExternal / searchMangaExternal wrap the Jikan → AniList fallback
// (MAL blocks Jikan's servers for days at a time; AniList carries MAL ids).
func (h *Handler) searchAnimeExternal(q string, limit int) ([]jikan.AnimeData, error) {
	var hits []jikan.AnimeData
	err := errJikanSkipped
	if h.jikanUp() {
		hits, _, err = h.jikan.SearchAnime(q, jikan.SearchOpts{Limit: limit})
		if err != nil {
			h.noteJikanDown()
		}
	}
	if err != nil {
		hits, err = h.anilist.SearchAnime(q, limit)
	}
	return hits, err
}

func (h *Handler) searchMangaExternal(q string, limit int) ([]jikan.MangaData, error) {
	var hits []jikan.MangaData
	err := errJikanSkipped
	if h.jikanUp() {
		hits, _, err = h.jikan.SearchManga(q, jikan.SearchOpts{Limit: limit})
		if err != nil {
			h.noteJikanDown()
		}
	}
	if err != nil {
		hits, err = h.anilist.SearchManga(q, limit)
	}
	return hits, err
}

// GET /api/requests/search?q= (auth) — combined anime+manga MAL search so the
// requester picks the exact series (season 1, season 2, the manga…).
func (h *Handler) searchRequests(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		httpx.Error(w, http.StatusBadRequest, "Query must be at least 2 characters")
		return
	}
	type hit struct {
		Medium   string  `json:"medium"`
		MalID    int     `json:"malId"`
		Title    string  `json:"title"`
		Type     string  `json:"type"`
		Year     *int    `json:"year,omitempty"`
		Episodes *int    `json:"episodes,omitempty"`
		Chapters *int    `json:"chapters,omitempty"`
		ImageURL *string `json:"imageUrl,omitempty"`
	}
	out := []hit{}
	if a, err := h.searchAnimeExternal(q, 6); err == nil {
		for _, d := range a {
			out = append(out, hit{Medium: "anime", MalID: d.MalID, Title: d.Title, Type: d.Type, Year: d.Year, Episodes: d.Episodes, ImageURL: d.ImageURL})
		}
	}
	if m, err := h.searchMangaExternal(q, 6); err == nil {
		for _, d := range m {
			out = append(out, hit{Medium: "manga", MalID: d.MalID, Title: d.Title, Type: d.Type, Year: d.Year, Chapters: d.Chapters, ImageURL: d.ImageURL})
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

// resolveRequestTitle is the freeform fallback (no picked hit): it takes the
// best MAL match, or the raw title with a nil id when nothing resolves.
func (h *Handler) resolveRequestTitle(medium, q string) (malID *int, title string, imageURL *string) {
	if medium == "anime" {
		if hits, err := h.searchAnimeExternal(q, 1); err == nil && len(hits) > 0 {
			id := hits[0].MalID
			return &id, hits[0].Title, hits[0].ImageURL
		}
		return nil, q, nil
	}
	if hits, err := h.searchMangaExternal(q, 1); err == nil && len(hits) > 0 {
		id := hits[0].MalID
		return &id, hits[0].Title, hits[0].ImageURL
	}
	return nil, q, nil
}

// alreadyInCatalog reports whether a resolved series is already published.
func (h *Handler) alreadyInCatalog(r *http.Request, medium string, malID int) bool {
	if medium == "anime" {
		_, err := h.repo.AnimeByMalID(r.Context(), malID)
		return err == nil
	}
	_, err := h.repo.MangaByMalID(r.Context(), malID)
	return err == nil
}

// POST /api/requests  (auth) — {medium, title, note?}
func (h *Handler) createRequest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Medium   string  `json:"medium"`
		Title    string  `json:"title"`
		MalID    *int    `json:"malId"`    // set when the requester picked a MAL hit
		ImageURL *string `json:"imageUrl"` // poster from the picked hit
		Note     *string `json:"note"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Medium != "anime" && body.Medium != "manga" {
		httpx.Error(w, http.StatusBadRequest, "medium must be 'anime' or 'manga'")
		return
	}
	if len(body.Title) < 2 || len(body.Title) > 200 {
		httpx.Error(w, http.StatusBadRequest, "Titlul e obligatoriu (2–200 caractere)")
		return
	}
	if body.Note != nil {
		n := strings.TrimSpace(*body.Note)
		if n == "" {
			body.Note = nil
		} else {
			// rune-safe cap (Romanian diacritics are multibyte)
			if r := []rune(n); len(r) > noteMaxLen {
				n = string(r[:noteMaxLen])
			}
			body.Note = &n
		}
	}
	uid := middleware.UserFrom(r).UserID

	// A picked MAL hit carries the canonical id + poster; otherwise resolve the
	// freeform title against MAL (fallback for MAL outages / manual entry).
	var malID *int
	var title string
	var imageURL *string
	if body.MalID != nil && *body.MalID > 0 {
		malID, title, imageURL = body.MalID, body.Title, body.ImageURL
	} else {
		malID, title, imageURL = h.resolveRequestTitle(body.Medium, body.Title)
	}

	// already available → no point requesting it
	if malID != nil && h.alreadyInCatalog(r, body.Medium, *malID) {
		httpx.Error(w, http.StatusConflict, "„"+title+"” e deja disponibil în catalog.")
		return
	}

	// dedup: merge into the existing request (as a vote) instead of duplicating
	if existingID, err := h.repo.FindRequest(r.Context(), body.Medium, malID, title); err == nil {
		if _, err := h.repo.VoteRequest(r.Context(), existingID, uid); err != nil {
			httpx.Internal(w, "vote existing request", err)
			return
		}
		tr, err := h.repo.RequestByID(r.Context(), existingID, uid)
		if err != nil {
			httpx.Internal(w, "load merged request", err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"data": tr, "merged": true,
			"message": "„" + title + "” era deja cerut — ți-am adăugat votul.",
		})
		return
	}

	id, err := h.repo.CreateRequest(r.Context(), uid, body.Medium, malID, title, imageURL, body.Note)
	if errors.Is(err, repo.ErrExists) {
		// lost a create race — treat as a merge
		if existingID, e := h.repo.FindRequest(r.Context(), body.Medium, malID, title); e == nil {
			id = existingID
		} else {
			httpx.Error(w, http.StatusConflict, "Cererea există deja.")
			return
		}
	} else if err != nil {
		httpx.Internal(w, "create request", err)
		return
	}
	if _, err := h.repo.VoteRequest(r.Context(), id, uid); err != nil {
		httpx.Internal(w, "self-vote request", err)
		return
	}
	tr, err := h.repo.RequestByID(r.Context(), id, uid)
	if err != nil {
		httpx.Internal(w, "load new request", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": tr, "merged": false})
}

// POST /api/requests/{id}/vote  (auth)
func (h *Handler) voteRequest(w http.ResponseWriter, r *http.Request) {
	h.setRequestVote(w, r, true)
}

// DELETE /api/requests/{id}/vote  (auth)
func (h *Handler) unvoteRequest(w http.ResponseWriter, r *http.Request) {
	h.setRequestVote(w, r, false)
}

func (h *Handler) setRequestVote(w http.ResponseWriter, r *http.Request, vote bool) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid request ID")
		return
	}
	uid := middleware.UserFrom(r).UserID
	var count int
	var err error
	if vote {
		count, err = h.repo.VoteRequest(r.Context(), id, uid)
	} else {
		count, err = h.repo.UnvoteRequest(r.Context(), id, uid)
	}
	if err != nil {
		httpx.Internal(w, "set request vote", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"voteCount": count, "voted": vote})
}

// PATCH /api/requests/{id}/status  (coordinator/admin) — {status}
func (h *Handler) setRequestStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid request ID")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := httpx.Decode(r, &body); err != nil || !requestStatuses[body.Status] {
		httpx.Error(w, http.StatusBadRequest, "status invalid")
		return
	}
	if err := h.repo.SetRequestStatus(r.Context(), id, body.Status); err != nil {
		notFoundOr(w, err, "Cererea nu există", "set request status")
		return
	}
	tr, err := h.repo.RequestByID(r.Context(), id, viewerIDOr0(r))
	if err != nil {
		httpx.Internal(w, "load request", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": tr})
}
