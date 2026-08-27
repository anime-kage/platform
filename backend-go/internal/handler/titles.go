package handler

// Anime + manga catalog endpoints. Search and browse hit our database; the
// discovery endpoints (trending, airing, publishing, seasonal) proxy Jikan
// and lazily import what they find, exactly like the old backend.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/jikan"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
)

func titleFiltersFrom(r *http.Request, defaultSort string) repo.TitleFilters {
	q := r.URL.Query()
	f := repo.TitleFilters{
		Query:  q.Get("q"),
		Page:   httpx.QueryInt(r, "page", 1, 1, 1<<30),
		Limit:  httpx.QueryInt(r, "limit", 25, 1, 50),
		Year:   httpx.QueryInt(r, "year", 0, 0, 1<<30),
		Season: q.Get("season"),
		Status: q.Get("status"),
		Type:   q.Get("type"),
		Letter: q.Get("letter"),
		Sort:   defaultSort,
	}
	if f.Query == "" {
		f.Query = q.Get("query")
	}
	if s := q.Get("sort"); s != "" {
		f.Sort = s
	}
	// anything else falls through as "" and titleOrder picks the natural
	// direction for the chosen sort
	if d := q.Get("dir"); d == "asc" || d == "desc" {
		f.Dir = d
	}
	for _, g := range strings.Split(q.Get("genres"), ",") {
		if g = strings.TrimSpace(g); g != "" {
			f.Genres = append(f.Genres, g)
		}
	}
	return f
}

// ── Anime ─────────────────────────────────────────────────────────────────────

// GET /api/anime
func (h *Handler) listAnime(w http.ResponseWriter, r *http.Request) {
	f := titleFiltersFrom(r, "createdAt")
	f.Query = "" // the list endpoint never text-searches
	rows, total, err := h.repo.SearchAnime(r.Context(), f)
	if err != nil {
		httpx.Internal(w, "fetch anime", err)
		return
	}
	httpx.Paginated(w, rows, f.Page, f.Limit, total)
}

// GET /api/anime/search
func (h *Handler) searchAnime(w http.ResponseWriter, r *http.Request) {
	f := titleFiltersFrom(r, "score")
	if f.Query == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{
			"error":   "Search query is required",
			"message": `Please provide a search query using the "q" or "query" parameter`,
		})
		return
	}
	rows, total, err := h.repo.SearchAnime(r.Context(), f)
	if err != nil {
		httpx.Internal(w, "search anime", err)
		return
	}
	httpx.Paginated(w, rows, f.Page, f.Limit, total)
}

// GET /api/anime/{id}
// The {id} segment accepts either a numeric id or a slug ("91-days"), so a
// pretty URL and an old numeric one resolve to the same row. Digits mean id:
// slugs can contain digits but never consist solely of them, because Make
// always keeps at least one letter run from the title... except for a title that
// is only digits, which is why the id branch is tried first and the slug branch
// is the fallback rather than the reverse.
func (h *Handler) animeByID(w http.ResponseWriter, r *http.Request) {
	a, err := h.resolveAnime(r, chi.URLParam(r, "id"))
	if err != nil {
		notFoundOr(w, err, "Anime not found", "fetch anime")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": a})
}

// resolveAnime looks a title up by numeric id, falling back to slug.
func (h *Handler) resolveAnime(r *http.Request, param string) (*model.Anime, error) {
	if id, ok := httpx.IntParam(param); ok {
		if a, err := h.repo.AnimeByID(r.Context(), id); err == nil {
			return a, nil
		}
	}
	return h.repo.AnimeBySlug(r.Context(), param)
}

// GET /api/anime/random
func (h *Handler) randomAnime(w http.ResponseWriter, r *http.Request) {
	a, err := h.repo.RandomAnime(r.Context())
	if err != nil {
		notFoundOr(w, err, "No anime found in database", "get random anime")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": a})
}

// GET /api/anime/schedule
func (h *Handler) schedule(w http.ResponseWriter, r *http.Request) {
	sched, err := h.repo.Schedule(r.Context())
	if err != nil {
		httpx.Internal(w, "fetch schedule", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": sched})
}

// GET /api/anime/most-watched?window=today|month|all
func (h *Handler) mostWatched(w http.ResponseWriter, r *http.Request) {
	limit := httpx.QueryInt(r, "limit", 8, 1, 25)
	window := r.URL.Query().Get("window")
	switch window {
	case "today", "month", "all":
	default:
		window = "all"
	}
	rows, err := h.repo.LeaderboardAnime(r.Context(), window, limit)
	if err != nil {
		httpx.Internal(w, "fetch most-watched anime", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GET /api/anime/recent-releases
func (h *Handler) recentReleases(w http.ResponseWriter, r *http.Request) {
	limit := httpx.QueryInt(r, "limit", 12, 1, 24)
	rows, err := h.repo.RecentReleases(r.Context(), limit)
	if err != nil {
		httpx.Internal(w, "fetch recent releases", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GET /api/anime/{id}/reviews
func (h *Handler) animeReviews(w http.ResponseWriter, r *http.Request) {
	h.titleReviews(w, r, "anime")
}

// GET /api/manga/{id}/reviews
func (h *Handler) mangaReviews(w http.ResponseWriter, r *http.Request) {
	h.titleReviews(w, r, "manga")
}

func (h *Handler) titleReviews(w http.ResponseWriter, r *http.Request, kind string) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid "+kind+" ID")
		return
	}
	rows, err := h.repo.TitleReviews(r.Context(), kind, id, 50)
	if err != nil {
		httpx.Internal(w, "fetch reviews", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GET /api/anime/trending | /api/anime/airing
func (h *Handler) trendingAnime(w http.ResponseWriter, r *http.Request) {
	h.jikanAnimeList(w, r, "bypopularity", "fetch trending anime")
}

func (h *Handler) airingAnime(w http.ResponseWriter, r *http.Request) {
	h.jikanAnimeList(w, r, "airing", "fetch currently airing anime")
}

func (h *Handler) jikanAnimeList(w http.ResponseWriter, r *http.Request, kind, action string) {
	page := httpx.QueryInt(r, "page", 1, 1, 1<<30)
	limit := httpx.QueryInt(r, "limit", 25, 1, 50)
	results, jp, err := h.jikan.TopAnime(kind, page, limit)
	if err != nil {
		httpx.Internal(w, action, err)
		return
	}
	h.respondImportedAnime(w, r, results, jp, limit)
}

// GET /api/anime/season/{year}/{season}
func (h *Handler) seasonalAnime(w http.ResponseWriter, r *http.Request) {
	year, ok := httpx.IntParam(chi.URLParam(r, "year"))
	if !ok || year < 1960 || year > time.Now().Year()+2 {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{
			"error":   "Invalid year",
			"message": "Year must be between 1960 and current year + 2",
		})
		return
	}
	season := chi.URLParam(r, "season")
	switch season {
	case "winter", "spring", "summer", "fall":
	default:
		httpx.JSON(w, http.StatusBadRequest, map[string]string{
			"error":   "Invalid season",
			"message": "Season must be one of: winter, spring, summer, fall",
		})
		return
	}

	page := httpx.QueryInt(r, "page", 1, 1, 1<<30)
	limit := httpx.QueryInt(r, "limit", 25, 1, 50)
	results, jp, err := h.jikan.SeasonalAnime(year, season, page)
	if err != nil {
		httpx.Internal(w, "fetch seasonal anime", err)
		return
	}
	h.respondImportedAnime(w, r, results, jp, limit)
}

// respondImportedAnime saves unknown Jikan results and returns the local rows
// with Jikan's pagination (its totals describe the remote list, not ours).
func (h *Handler) respondImportedAnime(w http.ResponseWriter, r *http.Request, results []jikan.AnimeData, jp jikan.Page, limit int) {
	saved := []model.Anime{}
	for _, d := range results {
		if d.MalID == 0 {
			continue
		}
		a, err := h.repo.InsertAnime(r.Context(), d) // conflict-safe: existing row wins
		if err != nil {
			continue // one bad row shouldn't sink the page
		}
		saved = append(saved, *a)
	}
	httpx.Paginated(w, saved, jp.CurrentPage, limit, jp.Total)
}

// GET /api/anime/mal-search?q= — MAL lookup for the publish flow: coordinators
// search by title instead of hunting for a MAL ID by hand. Nothing is saved;
// the import stays an explicit step by malId.
func (h *Handler) malSearchAnime(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		httpx.Error(w, http.StatusBadRequest, "Query must be at least 2 characters")
		return
	}
	var results []jikan.AnimeData
	err := errJikanSkipped
	if h.jikanUp() {
		results, _, err = h.jikan.SearchAnime(q, jikan.SearchOpts{Limit: 8})
		if err != nil {
			h.noteJikanDown()
		}
	}
	if err != nil {
		// Jikan outage (MAL blocks their servers for days at a time) —
		// AniList carries MAL ids, so the same flow works through it
		results, err = h.anilist.SearchAnime(q, 8)
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "MyAnimeList (Jikan) nu răspunde momentan — încearcă din nou în câteva minute.")
		return
	}
	type hit struct {
		MalID    int     `json:"malId"`
		Title    string  `json:"title"`
		Type     string  `json:"type"`
		Year     *int    `json:"year,omitempty"`
		Episodes *int    `json:"episodes,omitempty"`
		ImageURL *string `json:"imageUrl,omitempty"`
	}
	out := make([]hit, 0, len(results))
	for _, d := range results {
		out = append(out, hit{MalID: d.MalID, Title: d.Title, Type: d.Type, Year: d.Year, Episodes: d.Episodes, ImageURL: d.ImageURL})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

// GET /api/manga/mal-search?q= — the manga twin, for publishing manga
// releases whose series isn't in the catalog yet. Same Jikan → AniList
// fallback as the anime path.
func (h *Handler) malSearchManga(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		httpx.Error(w, http.StatusBadRequest, "Query must be at least 2 characters")
		return
	}
	var results []jikan.MangaData
	err := errJikanSkipped
	if h.jikanUp() {
		results, _, err = h.jikan.SearchManga(q, jikan.SearchOpts{Limit: 8})
		if err != nil {
			h.noteJikanDown()
		}
	}
	if err != nil {
		results, err = h.anilist.SearchManga(q, 8)
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "MyAnimeList (Jikan) nu răspunde momentan — încearcă din nou în câteva minute.")
		return
	}
	type hit struct {
		MalID    int     `json:"malId"`
		Title    string  `json:"title"`
		Type     string  `json:"type"`
		Year     *int    `json:"year,omitempty"`
		Chapters *int    `json:"chapters,omitempty"`
		ImageURL *string `json:"imageUrl,omitempty"`
	}
	out := make([]hit, 0, len(results))
	for _, d := range results {
		out = append(out, hit{MalID: d.MalID, Title: d.Title, Type: d.Type, Year: d.Year, Chapters: d.Chapters, ImageURL: d.ImageURL})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

// POST /api/anime/import/{malId}
func (h *Handler) importAnime(w http.ResponseWriter, r *http.Request) {
	malID, ok := httpx.IntParam(chi.URLParam(r, "malId"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid MAL ID")
		return
	}
	if existing, err := h.repo.AnimeByMalID(r.Context(), malID); err == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"data":    existing,
			"created": false,
			"message": "Anime already exists in database",
		})
		return
	}
	var d *jikan.AnimeData
	err := errJikanSkipped
	if h.jikanUp() {
		d, err = h.jikan.AnimeByID(malID)
		if err != nil {
			h.noteJikanDown()
		}
	}
	if err != nil {
		// same AniList fallback as mal-search — keeps imports working
		// through Jikan outages (broadcast day arrives later via autoupdate)
		d, err = h.anilist.AnimeByMal(malID)
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "MyAnimeList (Jikan) nu răspunde momentan — încearcă din nou în câteva minute.")
		return
	}
	// guard against an empty payload (an outage can hand back a zero-value
	// record with no error) — inserting it would pollute the catalog with junk
	if d == nil || d.MalID == 0 || strings.TrimSpace(d.Title) == "" {
		httpx.Error(w, http.StatusBadGateway, "Sursa a răspuns fără date pentru acest titlu — încearcă din nou în câteva minute.")
		return
	}
	a, err := h.repo.InsertAnime(r.Context(), *d)
	if err != nil {
		httpx.Internal(w, "import anime", err)
		return
	}
	h.translateSynopsisAsync("anime", a.ID, a.Title, a.Synopsis)
	h.fetchBannerAsync("anime", d.MalID)
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"data":    a,
		"created": true,
		"message": "Anime imported successfully",
	})
}

// translateSynopsisAsync fills synopsis_romanian in the background after an
// import — the response never waits on the model. No-op when the translator
// isn't configured (no ANTHROPIC_API_KEY) or there's nothing to translate.
// Season bulk imports skip this on purpose: one import = one call.
func (h *Handler) translateSynopsisAsync(kind string, id int, title string, synopsis *string) {
	if h.translator == nil || synopsis == nil || strings.TrimSpace(*synopsis) == "" {
		return
	}
	src := *synopsis
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		ro, err := h.translator.TranslateProse(ctx, title, src)
		if err != nil || ro == "" {
			slog.Warn("auto-translate synopsis", "kind", kind, "id", id, "err", err)
			return
		}
		if kind == "anime" {
			err = h.repo.SetAnimeSynopsisRo(ctx, id, ro)
		} else {
			err = h.repo.SetMangaSynopsisRo(ctx, id, ro)
		}
		if err != nil {
			slog.Warn("store synopsis translation", "kind", kind, "id", id, "err", err)
		}
	}()
}

// fetchBannerAsync resolves the AniList banner for a freshly imported title.
//
// Without this, banner_url stays NULL until `autoupdate banners` next runs from
// cron — which is nightly, so a series added during the day could not be chosen
// as a profile backdrop until the following morning.
// GET /api/users/me/banner/options only offers titles that have art, so the new
// series simply wasn't in the list and the delay looked like a sync bug.
//
// Background and best-effort, like translateSynopsisAsync: the import response
// must not wait on AniList, and a title with no banner is normal — SetBanners
// records ” for "asked, none exists", which is also what stops the cron from
// asking about it forever.
//
// The cron job is still the right place for bulk work: `cmd/populate` imports
// hundreds of titles without touching this handler, and AniList takes 50 ids per
// call, so a per-title request there would be wasteful. This covers the
// interactive case only.
func (h *Handler) fetchBannerAsync(kind string, malID int) {
	if h.anilist == nil || malID == 0 {
		return
	}
	gql, manga := "ANIME", false
	if kind == "manga" {
		gql, manga = "MANGA", true
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		banners, err := h.anilist.BannersByMal([]int{malID}, gql)
		if err != nil {
			slog.Warn("fetch banner after import", "kind", kind, "malId", malID, "err", err)
			return
		}
		// `asked` is passed even when the map is empty so the miss is recorded
		// as '' rather than left NULL for the cron to retry indefinitely.
		if _, err := h.repo.SetBanners(ctx, banners, manga, malID); err != nil {
			slog.Warn("store banner after import", "kind", kind, "malId", malID, "err", err)
		}
	}()
}

// POST /api/anime  (coordinator/admin) — manual catalog entry, the fallback
// for when neither Jikan nor AniList can serve. No MAL identity; a later
// import can be linked by hand.
func (h *Handler) createAnimeManual(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title         string   `json:"title"`
		TitleEnglish  *string  `json:"titleEnglish"`
		TitleRomanian *string  `json:"titleRomanian"`
		Synopsis      *string  `json:"synopsis"`
		SynopsisRo    *string  `json:"synopsisRomanian"`
		Type          string   `json:"type"`
		Status        string   `json:"status"`
		Year          *int     `json:"year"`
		Season        *string  `json:"season"`
		Episodes      *int     `json:"episodes"`
		Genres        []string `json:"genres"`
		Studios       []string `json:"studios"`
		ImageURL      *string  `json:"imageUrl"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		httpx.Error(w, http.StatusBadRequest, "Titlul este obligatoriu")
		return
	}
	if body.Type == "" {
		body.Type = "tv"
	}
	if body.Status == "" {
		body.Status = "airing"
	}
	a, err := h.repo.InsertAnimeManual(r.Context(), jikan.AnimeData{
		Title: body.Title, TitleEnglish: body.TitleEnglish,
		Synopsis: body.Synopsis, Genres: body.Genres, Studios: body.Studios,
		Status: body.Status, Type: body.Type, Episodes: body.Episodes,
		Year: body.Year, Season: body.Season, ImageURL: body.ImageURL,
	})
	if err != nil {
		httpx.Internal(w, "create anime", err)
		return
	}
	// RO fields live outside the Jikan shape — patch them on after
	if body.TitleRomanian != nil || body.SynopsisRo != nil {
		a, err = h.repo.PatchAnime(r.Context(), a.ID, repo.TitlePatch{
			TitleRomanian: body.TitleRomanian, SynopsisRo: body.SynopsisRo,
		})
		if err != nil {
			httpx.Internal(w, "create anime", err)
			return
		}
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": a, "message": "Serie creată manual"})
}

// POST /api/manga  (coordinator/admin) — manual manga entry, same idea.
func (h *Handler) createMangaManual(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title         string   `json:"title"`
		TitleEnglish  *string  `json:"titleEnglish"`
		TitleRomanian *string  `json:"titleRomanian"`
		Synopsis      *string  `json:"synopsis"`
		SynopsisRo    *string  `json:"synopsisRomanian"`
		Type          string   `json:"type"`
		Status        string   `json:"status"`
		Year          *int     `json:"year"`
		Chapters      *int     `json:"chapters"`
		Volumes       *int     `json:"volumes"`
		Genres        []string `json:"genres"`
		Authors       []string `json:"authors"`
		ImageURL      *string  `json:"imageUrl"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		httpx.Error(w, http.StatusBadRequest, "Titlul este obligatoriu")
		return
	}
	if body.Type == "" {
		body.Type = "manga"
	}
	if body.Status == "" {
		body.Status = "publishing"
	}
	m, err := h.repo.InsertMangaManual(r.Context(), jikan.MangaData{
		Title: body.Title, TitleEnglish: body.TitleEnglish,
		Synopsis: body.Synopsis, Genres: body.Genres, Authors: body.Authors,
		Status: body.Status, Type: body.Type, Chapters: body.Chapters,
		Volumes: body.Volumes, Year: body.Year, ImageURL: body.ImageURL,
	})
	if err != nil {
		httpx.Internal(w, "create manga", err)
		return
	}
	if body.TitleRomanian != nil || body.SynopsisRo != nil {
		m, err = h.repo.PatchManga(r.Context(), m.ID, repo.TitlePatch{
			TitleRomanian: body.TitleRomanian, SynopsisRo: body.SynopsisRo,
		})
		if err != nil {
			httpx.Internal(w, "create manga", err)
			return
		}
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": m, "message": "Serie creată manual"})
}

// uploadPoster is the shared multipart handler behind the anime/manga poster
// endpoints; subdir is the folder under UPLOADS_DIR the image lands in, and
// patch persists the stored URL on the right table (nil when the caller
// stores the URL itself, as the curated placements do).
// Per-caller upload ceilings. A poster is a 2:3 cover that renders at ~240px
// wide, so 4 MB is already generous; a news post's images are full-width
// screenshots and photos straight off a phone, which routinely exceed that.
const (
	posterMaxUpload = 4 << 20  // 4 MB — anime/manga covers, curated artwork
	postMaxUpload   = 24 << 20 // 24 MB — images inside a news post
)

// newUploadName is a random filename with the right extension for a sniffed
// image format. Shared by the poster and emote paths.
func newUploadName(format string) string {
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	return uuid.NewString() + "." + ext
}

func (h *Handler) uploadPoster(w http.ResponseWriter, r *http.Request, subdir string, patch func(ctx context.Context, url string) error) {
	h.uploadImage(w, r, subdir, posterMaxUpload, patch)
}

// uploadImage is the shared multipart handler. `maxSize` is the caller's
// ceiling; the field is accepted as either "poster" or "image" so a caller can
// name it after what it actually is.
func (h *Handler) uploadImage(w http.ResponseWriter, r *http.Request, subdir string, maxSize int64, patch func(ctx context.Context, url string) error) {
	tooBig := fmt.Sprintf("Imaginea trebuie să aibă sub %d MB", maxSize>>20)
	r.Body = http.MaxBytesReader(w, r.Body, maxSize+4096)
	if err := r.ParseMultipartForm(maxSize); err != nil {
		// Distinguish the two reasons this fails. They used to share a message,
		// and that cost real debugging time: a client that sent the form with
		// Content-Type: application/json (so the multipart boundary was lost)
		// got told its 300 KB image was too big.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, tooBig)
			return
		}
		httpx.Error(w, http.StatusBadRequest,
			"Formularul de încărcare nu a putut fi citit (trebuie trimis ca multipart/form-data)")
		return
	}
	file, header, err := r.FormFile("poster")
	if err != nil {
		if file, header, err = r.FormFile("image"); err != nil {
			httpx.Error(w, http.StatusBadRequest, "Fișierul imagine lipsește")
			return
		}
	}
	defer file.Close()
	if header.Size > maxSize {
		httpx.Error(w, http.StatusBadRequest, tooBig)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		httpx.Internal(w, "upload poster", err)
		return
	}
	ext, ok := avatarExt[http.DetectContentType(data)]
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Doar imagini JPEG, PNG, WebP sau GIF")
		return
	}
	dir := filepath.Join(h.cfg.UploadsDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		httpx.Internal(w, "upload poster", err)
		return
	}
	filename := uuid.NewString() + "." + ext
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o644); err != nil {
		httpx.Internal(w, "upload poster", err)
		return
	}
	url := "/uploads/" + subdir + "/" + filename
	if patch != nil {
		if err := patch(r.Context(), url); err != nil {
			notFoundOr(w, err, "Title not found", "upload poster")
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"imageUrl": url, "message": "Poster încărcat"})
}

// POST /api/anime/{id}/poster  (coordinator/admin)
func (h *Handler) uploadAnimePoster(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}
	h.uploadPoster(w, r, "posters", func(ctx context.Context, url string) error {
		_, err := h.repo.PatchAnime(ctx, id, repo.TitlePatch{ImageURL: &url})
		return err
	})
}

// POST /api/manga/{id}/poster  (coordinator/admin)
func (h *Handler) uploadMangaPoster(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid manga ID")
		return
	}
	h.uploadPoster(w, r, "posters", func(ctx context.Context, url string) error {
		_, err := h.repo.PatchManga(ctx, id, repo.TitlePatch{ImageURL: &url})
		return err
	})
}

// PUT /api/anime/{id}/update
func (h *Handler) updateAnime(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}
	existing, err := h.repo.AnimeByID(r.Context(), id)
	if err != nil {
		notFoundOr(w, err, "Anime not found", "update anime")
		return
	}
	if existing.MalID == nil {
		httpx.Error(w, http.StatusBadRequest, fmt.Sprintf("Anime %d has no MAL ID to update from", id))
		return
	}
	d, err := h.jikan.AnimeByID(*existing.MalID)
	if err != nil {
		httpx.Internal(w, "update anime", err)
		return
	}
	a, err := h.repo.UpdateAnimeFromJikan(r.Context(), id, *d)
	if err != nil {
		httpx.Internal(w, "update anime", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data":    a,
		"message": "Anime updated successfully",
	})
}

// ── Manga ─────────────────────────────────────────────────────────────────────

// GET /api/manga
func (h *Handler) listManga(w http.ResponseWriter, r *http.Request) {
	f := repo.TitleFilters{
		Page:  httpx.QueryInt(r, "page", 1, 1, 1<<30),
		Limit: httpx.QueryInt(r, "limit", 25, 1, 50),
		Sort:  "createdAt",
	}
	rows, total, err := h.repo.SearchManga(r.Context(), f)
	if err != nil {
		httpx.Internal(w, "fetch manga", err)
		return
	}
	httpx.Paginated(w, rows, f.Page, f.Limit, total)
}

// GET /api/manga/search
func (h *Handler) searchManga(w http.ResponseWriter, r *http.Request) {
	f := titleFiltersFrom(r, "score")
	if f.Query == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{
			"error":   "Search query is required",
			"message": `Please provide a search query using the "q" or "query" parameter`,
		})
		return
	}
	rows, total, err := h.repo.SearchManga(r.Context(), f)
	if err != nil {
		httpx.Internal(w, "search manga", err)
		return
	}
	httpx.Paginated(w, rows, f.Page, f.Limit, total)
}

// GET /api/manga/{id}
// Accepts a numeric id or a slug, same as animeByID.
func (h *Handler) mangaByID(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "id")
	var m *model.Manga
	var err error = repo.ErrNotFound
	if id, ok := httpx.IntParam(param); ok {
		m, err = h.repo.MangaByID(r.Context(), id)
	}
	if err != nil {
		m, err = h.repo.MangaBySlug(r.Context(), param)
	}
	if err != nil {
		notFoundOr(w, err, "Manga not found", "fetch manga")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": m})
}

// GET /api/manga/trending | /api/manga/publishing
func (h *Handler) trendingManga(w http.ResponseWriter, r *http.Request) {
	h.jikanMangaList(w, r, "bypopularity", "fetch trending manga")
}

func (h *Handler) publishingManga(w http.ResponseWriter, r *http.Request) {
	h.jikanMangaList(w, r, "publishing", "fetch currently publishing manga")
}

func (h *Handler) jikanMangaList(w http.ResponseWriter, r *http.Request, kind, action string) {
	page := httpx.QueryInt(r, "page", 1, 1, 1<<30)
	limit := httpx.QueryInt(r, "limit", 25, 1, 50)
	results, jp, err := h.jikan.TopManga(kind, page, limit)
	if err != nil {
		httpx.Internal(w, action, err)
		return
	}
	saved := []model.Manga{}
	for _, d := range results {
		if d.MalID == 0 {
			continue
		}
		m, err := h.repo.InsertManga(r.Context(), d)
		if err != nil {
			continue
		}
		saved = append(saved, *m)
	}
	httpx.Paginated(w, saved, jp.CurrentPage, limit, jp.Total)
}

// POST /api/manga/import/{malId}
func (h *Handler) importManga(w http.ResponseWriter, r *http.Request) {
	malID, ok := httpx.IntParam(chi.URLParam(r, "malId"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid MAL ID")
		return
	}
	if existing, err := h.repo.MangaByMalID(r.Context(), malID); err == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"data":    existing,
			"created": false,
			"message": "Manga already exists in database",
		})
		return
	}
	var d *jikan.MangaData
	err := errJikanSkipped
	if h.jikanUp() {
		d, err = h.jikan.MangaByID(malID)
		if err != nil {
			h.noteJikanDown()
		}
	}
	if err != nil {
		// same AniList fallback as the anime import
		d, err = h.anilist.MangaByMal(malID)
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "MyAnimeList (Jikan) nu răspunde momentan — încearcă din nou în câteva minute.")
		return
	}
	if d == nil || d.MalID == 0 || strings.TrimSpace(d.Title) == "" {
		httpx.Error(w, http.StatusBadGateway, "Sursa a răspuns fără date pentru acest titlu — încearcă din nou în câteva minute.")
		return
	}
	m, err := h.repo.InsertManga(r.Context(), *d)
	if err != nil {
		httpx.Internal(w, "import manga", err)
		return
	}
	h.translateSynopsisAsync("manga", m.ID, m.Title, m.Synopsis)
	h.fetchBannerAsync("manga", d.MalID)
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"data":    m,
		"created": true,
		"message": "Manga imported successfully",
	})
}

// PUT /api/manga/{id}/update
func (h *Handler) updateManga(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid manga ID")
		return
	}
	existing, err := h.repo.MangaByID(r.Context(), id)
	if err != nil {
		notFoundOr(w, err, "Manga not found", "update manga")
		return
	}
	if existing.MalID == nil {
		httpx.Error(w, http.StatusBadRequest, fmt.Sprintf("Manga %d has no MAL ID to update from", id))
		return
	}
	d, err := h.jikan.MangaByID(*existing.MalID)
	if err != nil {
		httpx.Internal(w, "update manga", err)
		return
	}
	m, err := h.repo.UpdateMangaFromJikan(r.Context(), id, *d)
	if err != nil {
		httpx.Internal(w, "update manga", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data":    m,
		"message": "Manga updated successfully",
	})
}

// ── Manual catalog edits, deletes, season import ───────────────────

type titlePatchBody struct {
	Title         *string   `json:"title"`
	TitleEnglish  *string   `json:"titleEnglish"`
	TitleRomanian *string   `json:"titleRomanian"`
	Synopsis      *string   `json:"synopsis"`
	SynopsisRo    *string   `json:"synopsisRomanian"`
	ImageURL      *string   `json:"imageUrl"`
	Status        *string   `json:"status"`
	Type          *string   `json:"type"`
	Year          *int      `json:"year"`
	Genres        *[]string `json:"genres"`
	Episodes      *int      `json:"episodes"`
	Studios       *[]string `json:"studios"`
	Chapters      *int      `json:"chapters"`
	Volumes       *int      `json:"volumes"`
	Authors       *[]string `json:"authors"`
}

func (b titlePatchBody) toPatch() repo.TitlePatch {
	return repo.TitlePatch{
		Title: b.Title, TitleEnglish: b.TitleEnglish, TitleRomanian: b.TitleRomanian,
		Synopsis: b.Synopsis, SynopsisRo: b.SynopsisRo,
		ImageURL: b.ImageURL, Status: b.Status, Type: b.Type, Year: b.Year,
		Genres: b.Genres, Episodes: b.Episodes, Studios: b.Studios,
		Chapters: b.Chapters, Volumes: b.Volumes, Authors: b.Authors,
	}
}

// PUT /api/anime/{id}  (admin/translator) — manual field edits; the separate
// /{id}/update route stays the "re-sync from Jikan" button.
func (h *Handler) patchAnime(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}
	var body titlePatchBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	a, err := h.repo.PatchAnime(r.Context(), id, body.toPatch())
	if err != nil {
		notFoundOr(w, err, "Anime not found", "patch anime")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": a})
}

// PUT /api/manga/{id}  (admin/translator)
func (h *Handler) patchManga(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid manga ID")
		return
	}
	var body titlePatchBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	m, err := h.repo.PatchManga(r.Context(), id, body.toPatch())
	if err != nil {
		notFoundOr(w, err, "Manga not found", "patch manga")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": m})
}

// DELETE /api/anime/{id}  (admin) — episodes/links/lists/comments cascade
func (h *Handler) deleteAnime(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}
	if err := h.repo.DeleteAnime(r.Context(), id); err != nil {
		notFoundOr(w, err, "Anime not found", "delete anime")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Anime deleted successfully"})
}

// DELETE /api/manga/{id}  (admin)
func (h *Handler) deleteManga(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid manga ID")
		return
	}
	if err := h.repo.DeleteManga(r.Context(), id); err != nil {
		notFoundOr(w, err, "Manga not found", "delete manga")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Manga deleted successfully"})
}

// POST /api/admin/import-season/{year}/{season}  (admin/translator)
//
// One Jikan page (25 titles) per call; the response's hasNext tells the panel
// to call again with ?page=N+1. Import itself is DB-only upserts.
func (h *Handler) importSeason(w http.ResponseWriter, r *http.Request) {
	year, ok := httpx.IntParam(chi.URLParam(r, "year"))
	season := chi.URLParam(r, "season")
	if !ok || year < 1950 || year > 2100 {
		httpx.Error(w, http.StatusBadRequest, "Invalid year")
		return
	}
	valid := map[string]bool{"winter": true, "spring": true, "summer": true, "fall": true}
	if !valid[season] {
		httpx.Error(w, http.StatusBadRequest, "season must be winter, spring, summer or fall")
		return
	}
	page := httpx.QueryInt(r, "page", 1, 1, 50)

	list, pg, err := h.jikan.SeasonalAnime(year, season, page)
	if err != nil {
		httpx.Internal(w, "fetch season from Jikan", err)
		return
	}
	imported, skipped := 0, 0
	for _, d := range list {
		if _, err := h.repo.AnimeByMalID(r.Context(), d.MalID); err == nil {
			skipped++
			continue
		}
		if _, err := h.repo.InsertAnime(r.Context(), d); err != nil {
			httpx.Internal(w, "import seasonal anime", err)
			return
		}
		imported++
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"imported": imported, "skipped": skipped, "page": page, "hasNext": pg.HasNextPage,
	}})
}
