package handler

// Episode, chapter, and content-link endpoints. Writes are role-gated:
// admin/translator manage content, only admin deletes episodes/chapters.

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/anilist"
	"animekage/backend/internal/httpx"
	"animekage/backend/internal/jikan"
	"animekage/backend/internal/repo"
)

// validateHostingURL gates what may be stored as a content link — these URLs
// end up as iframe srcs on our pages. Absolute https, a real public hostname
// (no IPs, localhost, or single-label hosts), no embedded credentials, and
// when CONTENT_HOSTS is configured, only those domains (suffix match).
func validateHostingURL(raw string, allowed []string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return errors.New("hosting URL must be absolute")
	}
	if u.Scheme != "https" {
		return errors.New("hosting URL must use https")
	}
	if u.User != nil {
		return errors.New("hosting URL must not contain credentials")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || net.ParseIP(host) != nil || !strings.Contains(host, ".") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return errors.New("hosting URL must point at a public domain")
	}
	if len(allowed) == 0 {
		return nil
	}
	for _, a := range allowed {
		if host == a || strings.HasSuffix(host, "."+a) {
			return nil
		}
	}
	return errors.New("hosting URL domain is not on the allowed list")
}

// fileHostEmbed rewrites a file host's *page* URL into its bare-player embed
// URL. Pasting the page you land on after uploading (…/watch, …/file, …/d/…)
// is the natural mistake, and an iframe of it renders the host's whole site —
// header, ads and all — instead of just the player. Hosts we know keep the
// file code in the path and expose the player at /e/{code}.
//
// Unknown hosts are returned unchanged: guessing a path would break them, and
// each host's embed shape has to be confirmed against a real file code in a
// browser before it goes in the table — a fetch check is not enough. Filemoon
// serves a single-page-app shell with 200 on *every* path of its other domains,
// so "it returned 200" proves nothing there; and /e/{code}, which looks like
// the obvious form, 302s to their homepage.
var fileHostCode = regexp.MustCompile(`(?i)^(?:/[a-z]{2})?(?:/(?:e|d|f|v|file|embed|download))*/([a-z0-9]{8,24})(?:/[^/]*)*/?$`)

var fileHostLocale = regexp.MustCompile(`(?i)^/([a-z]{2})/`)

// embedHosts maps a host's first label to its player path, with {code} and
// {locale} filled in from what was pasted. Verified against a live upload:
// Filemoon July 2026, Playmogo (a DoodStream domain) July 2026.
var embedHosts = map[string]string{
	"filemoon":   "/{locale}/{code}/embed",
	"moonplayer": "/{locale}/{code}/embed",
	"playmogo":   "/e/{code}",
}

func fileHostEmbed(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	base := host
	if i := strings.IndexByte(base, '.'); i > 0 {
		base = base[:i]
	}
	tmpl, known := embedHosts[base]
	if !known {
		return raw
	}
	m := fileHostCode.FindStringSubmatch(u.Path)
	if m == nil {
		return raw
	}
	locale := "en"
	if l := fileHostLocale.FindStringSubmatch(u.Path); l != nil {
		locale = strings.ToLower(l[1])
	}
	path := strings.NewReplacer("{code}", m[1], "{locale}", locale).Replace(tmpl)
	return "https://" + host + path
}

// GET /api/anime/{id}/episodes
func (h *Handler) listEpisodes(w http.ResponseWriter, r *http.Request) {
	animeID, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}
	eps, err := h.repo.EpisodesByAnime(r.Context(), animeID)
	if err != nil {
		httpx.Internal(w, "fetch episodes", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": eps})
}

// GET /api/anime/{id}/episodes/{num}
func (h *Handler) episodeByNumber(w http.ResponseWriter, r *http.Request) {
	animeID, ok1 := httpx.IntParam(chi.URLParam(r, "id"))
	num, ok2 := httpx.IntParam(chi.URLParam(r, "num"))
	if !ok1 || !ok2 {
		httpx.Error(w, http.StatusBadRequest, "Invalid parameters")
		return
	}
	ep, err := h.repo.EpisodeByNumber(r.Context(), animeID, num)
	if err != nil {
		notFoundOr(w, err, "Episode not found", "fetch episode")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": ep})
}

type episodeBody struct {
	EpisodeNumber *int    `json:"episodeNumber"`
	Title         *string `json:"title"`
	AirDate       *string `json:"airDate"`
	Duration      *int    `json:"duration"`
	Synopsis      *string `json:"synopsis"`
	IsFiller      *bool   `json:"isFiller"`
	IsRecap       *bool   `json:"isRecap"`
}

// episodeSynopsisMax caps the per-episode description. Generous enough for a
// real paragraph, small enough that a long list of episodes stays a sane
// payload — /anime/{id} ships up to 100 of these at a time.
const episodeSynopsisMax = 2000

// POST /api/anime/{id}/episodes  (admin/translator)
func (h *Handler) createEpisode(w http.ResponseWriter, r *http.Request) {
	animeID, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}
	var body episodeBody
	if err := httpx.Decode(r, &body); err != nil || body.EpisodeNumber == nil {
		httpx.Error(w, http.StatusBadRequest, "Episode number is required")
		return
	}
	ep, err := h.repo.CreateEpisode(r.Context(), animeID, *body.EpisodeNumber, repo.EpisodeInput{
		Title: body.Title, AirDate: body.AirDate, Duration: body.Duration,
		Synopsis: body.Synopsis, IsFiller: body.IsFiller, IsRecap: body.IsRecap,
	})
	if errors.Is(err, repo.ErrExists) {
		httpx.Error(w, http.StatusConflict, "Episode already exists")
		return
	}
	if err != nil {
		notFoundOr(w, err, err.Error(), "create episode")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": ep})
}

// POST /api/anime/{id}/episodes/sync  (admin/translator)
//
// Pulls episode titles, air dates and MAL's filler/recap marks for one series,
// filling gaps on rows that already exist and adding any episode we are missing.
//
// This is a button, not a cron step, and the reason is the nightly job's scope:
// it only polls anime whose status is 'airing' or 'upcoming', so a completed
// series added by hand — 91 Days, say, whose 12 episodes were created with no
// titles — was never going to be filled in by anything. Making it explicit also
// suits Jikan's reliability: it 504s for days at a time, and an editor pressing
// a button gets told that, where a silent cron step does not.
func (h *Handler) syncEpisodesFromMAL(w http.ResponseWriter, r *http.Request) {
	animeID, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}
	a, err := h.repo.AnimeByID(r.Context(), animeID)
	if err != nil {
		notFoundOr(w, err, "Anime not found", "sync episodes")
		return
	}
	if a.MalID == nil || *a.MalID <= 0 {
		httpx.Error(w, http.StatusBadRequest, "Seria nu are un MAL id, deci nu are de unde sincroniza")
		return
	}

	// Jikan first: it is the only source with filler/recap marks and air dates.
	eps, jikanErr := h.jikan.AnimeEpisodes(*a.MalID)

	// AniList fallback, titles only. MAL fails per-entry rather than globally —
	// 91 Days and NANA 504 on every attempt while other series answer fine — so
	// "retry later" would have been a permanent answer for them.
	fromAniList := false
	if jikanErr != nil || len(eps) == 0 {
		titles, alErr := anilist.NewClient().EpisodeTitlesByMal(*a.MalID)
		if alErr != nil || len(titles) == 0 {
			httpx.Error(w, http.StatusBadGateway,
				"Nici MyAnimeList, nici AniList nu au episoade pentru seria asta acum. Încearcă mai târziu.")
			return
		}
		eps = eps[:0]
		for num, title := range titles {
			t := title
			eps = append(eps, jikan.EpisodeData{Number: num, Title: &t})
		}
		fromAniList = true
	}

	existing, err := h.repo.EpisodeNumbers(r.Context(), animeID)
	if err != nil {
		httpx.Internal(w, "episode numbers", err)
		return
	}

	var added, updated int
	for _, ep := range eps {
		if ep.Number <= 0 {
			continue
		}
		// nil when the data came from AniList: it has no filler/recap, and
		// writing its defaults would clear marks MAL had already given us.
		var filler, recap *bool
		if !fromAniList {
			filler, recap = &ep.Filler, &ep.Recap
		}
		if existing[ep.Number] {
			changed, err := h.repo.FillEpisodeMeta(r.Context(), animeID, ep.Number,
				ep.Title, ep.Aired, filler, recap)
			if err != nil {
				httpx.Internal(w, "fill episode meta", err)
				return
			}
			if changed {
				updated++
			}
			continue
		}
		_, err := h.repo.CreateEpisode(r.Context(), animeID, ep.Number, repo.EpisodeInput{
			Title: ep.Title, AirDate: ep.Aired,
			IsFiller: filler, IsRecap: recap,
		})
		if err != nil && !errors.Is(err, repo.ErrExists) {
			httpx.Internal(w, "create episode from sync", err)
			return
		}
		if err == nil {
			added++
		}
	}

	// `source` lets the editor know whether filler marks came along: AniList has
	// none, so a series filled from there still needs them set by hand.
	source := "mal"
	if fromAniList {
		source = "anilist"
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"added": added, "updated": updated, "found": len(eps), "source": source,
	})
}

// PUT /api/anime/{id}/episodes/{num}  (admin/translator)
func (h *Handler) updateEpisode(w http.ResponseWriter, r *http.Request) {
	animeID, ok1 := httpx.IntParam(chi.URLParam(r, "id"))
	num, ok2 := httpx.IntParam(chi.URLParam(r, "num"))
	if !ok1 || !ok2 {
		httpx.Error(w, http.StatusBadRequest, "Invalid parameters")
		return
	}
	var body episodeBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	if body.Synopsis != nil && len([]rune(*body.Synopsis)) > episodeSynopsisMax {
		httpx.Error(w, http.StatusBadRequest, "Descrierea e prea lungă (max 2000 de caractere)")
		return
	}
	ep, err := h.repo.UpdateEpisode(r.Context(), animeID, num, repo.EpisodeInput{
		Title: body.Title, AirDate: body.AirDate, Duration: body.Duration,
		Synopsis: body.Synopsis, IsFiller: body.IsFiller, IsRecap: body.IsRecap,
	})
	if err != nil {
		notFoundOr(w, err, "Episode not found", "update episode")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": ep})
}

// DELETE /api/anime/{id}/episodes/{num}  (admin)
func (h *Handler) deleteEpisode(w http.ResponseWriter, r *http.Request) {
	animeID, ok1 := httpx.IntParam(chi.URLParam(r, "id"))
	num, ok2 := httpx.IntParam(chi.URLParam(r, "num"))
	if !ok1 || !ok2 {
		httpx.Error(w, http.StatusBadRequest, "Invalid parameters")
		return
	}
	if err := h.repo.DeleteEpisode(r.Context(), animeID, num); err != nil {
		notFoundOr(w, err, "Episode not found", "delete episode")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Episode deleted successfully"})
}

// ── Chapters ──────────────────────────────────────────────────────────────────

// chapterNum parses the {num} path segment, which may be fractional ("10.5").
func chapterNum(r *http.Request) (float64, bool) {
	n, err := strconv.ParseFloat(chi.URLParam(r, "num"), 64)
	return n, err == nil && n >= 0
}

// GET /api/manga/{id}/chapters
func (h *Handler) listChapters(w http.ResponseWriter, r *http.Request) {
	mangaID, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid manga ID")
		return
	}
	chs, err := h.repo.ChaptersByManga(r.Context(), mangaID)
	if err != nil {
		httpx.Internal(w, "fetch chapters", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": chs})
}

// GET /api/manga/{id}/chapters/{num}
func (h *Handler) chapterByNumber(w http.ResponseWriter, r *http.Request) {
	mangaID, ok1 := httpx.IntParam(chi.URLParam(r, "id"))
	num, ok2 := chapterNum(r)
	if !ok1 || !ok2 {
		httpx.Error(w, http.StatusBadRequest, "Invalid parameters")
		return
	}
	ch, err := h.repo.ChapterByNumber(r.Context(), mangaID, num)
	if err != nil {
		notFoundOr(w, err, "Chapter not found", "fetch chapter")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": ch})
}

// GET /api/anime/{id}/episodes/{num}/credits — the translator + verifier
// behind the published RO track (public; empty when nothing's published).
func (h *Handler) episodeCredits(w http.ResponseWriter, r *http.Request) {
	animeID, ok1 := httpx.IntParam(chi.URLParam(r, "id"))
	num, ok2 := httpx.IntParam(chi.URLParam(r, "num"))
	if !ok1 || !ok2 {
		httpx.Error(w, http.StatusBadRequest, "Invalid parameters")
		return
	}
	c, err := h.repo.EpisodeCredits(r.Context(), animeID, num)
	if err != nil {
		httpx.Internal(w, "episode credits", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": c})
}

// GET /api/manga/{id}/chapters/{num}/credits — the manga twin.
func (h *Handler) chapterCredits(w http.ResponseWriter, r *http.Request) {
	mangaID, ok1 := httpx.IntParam(chi.URLParam(r, "id"))
	num, ok2 := chapterNum(r)
	if !ok1 || !ok2 {
		httpx.Error(w, http.StatusBadRequest, "Invalid parameters")
		return
	}
	c, err := h.repo.ChapterCredits(r.Context(), mangaID, num)
	if err != nil {
		httpx.Internal(w, "chapter credits", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": c})
}

type chapterBody struct {
	ChapterNumber *float64 `json:"chapterNumber"`
	Title         *string  `json:"title"`
	ReleaseDate   *string  `json:"releaseDate"`
	Pages         *int     `json:"pages"`
}

// POST /api/manga/{id}/chapters  (admin/translator)
func (h *Handler) createChapter(w http.ResponseWriter, r *http.Request) {
	mangaID, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid manga ID")
		return
	}
	var body chapterBody
	if err := httpx.Decode(r, &body); err != nil || body.ChapterNumber == nil {
		httpx.Error(w, http.StatusBadRequest, "Chapter number is required")
		return
	}
	ch, err := h.repo.CreateChapter(r.Context(), mangaID, *body.ChapterNumber, repo.ChapterInput{
		Title: body.Title, ReleaseDate: body.ReleaseDate, Pages: body.Pages,
	})
	if errors.Is(err, repo.ErrExists) {
		httpx.Error(w, http.StatusConflict, "Chapter already exists")
		return
	}
	if err != nil {
		notFoundOr(w, err, err.Error(), "create chapter")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": ch})
}

// PUT /api/manga/{id}/chapters/{num}  (admin/translator)
func (h *Handler) updateChapter(w http.ResponseWriter, r *http.Request) {
	mangaID, ok1 := httpx.IntParam(chi.URLParam(r, "id"))
	num, ok2 := chapterNum(r)
	if !ok1 || !ok2 {
		httpx.Error(w, http.StatusBadRequest, "Invalid parameters")
		return
	}
	var body chapterBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	ch, err := h.repo.UpdateChapter(r.Context(), mangaID, num, repo.ChapterInput{
		Title: body.Title, ReleaseDate: body.ReleaseDate, Pages: body.Pages,
	})
	if err != nil {
		notFoundOr(w, err, "Chapter not found", "update chapter")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": ch})
}

// DELETE /api/manga/{id}/chapters/{num}  (admin)
func (h *Handler) deleteChapter(w http.ResponseWriter, r *http.Request) {
	mangaID, ok1 := httpx.IntParam(chi.URLParam(r, "id"))
	num, ok2 := chapterNum(r)
	if !ok1 || !ok2 {
		httpx.Error(w, http.StatusBadRequest, "Invalid parameters")
		return
	}
	if err := h.repo.DeleteChapter(r.Context(), mangaID, num); err != nil {
		notFoundOr(w, err, "Chapter not found", "delete chapter")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Chapter deleted successfully"})
}

// ── Content links ─────────────────────────────────────────────────────────────

type linkBody struct {
	HostingURL  string  `json:"hostingUrl"`
	Quality     *string `json:"quality"`
	Language    string  `json:"language"`
	Kind        string  `json:"kind"` // 'embed' (default) | 'extract'
	Provider    *string `json:"provider"`
	ProviderRef *string `json:"providerRef"`
	Priority    int     `json:"priority"`
}

// POST /api/episodes/{id}/links | /api/chapters/{id}/links  (admin/translator)
func (h *Handler) addEpisodeLink(w http.ResponseWriter, r *http.Request) {
	h.addLink(w, r, "episode")
}

func (h *Handler) addChapterLink(w http.ResponseWriter, r *http.Request) {
	h.addLink(w, r, "chapter")
}

func (h *Handler) addLink(w http.ResponseWriter, r *http.Request, kind string) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid "+kind+" ID")
		return
	}
	var body linkBody
	if err := httpx.Decode(r, &body); err != nil || body.HostingURL == "" {
		httpx.Error(w, http.StatusBadRequest, "Hosting URL is required")
		return
	}
	if err := validateHostingURL(body.HostingURL, h.cfg.ContentHosts); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Kind == "" {
		body.Kind = "embed"
	}
	if body.Kind != "embed" && body.Kind != "extract" {
		httpx.Error(w, http.StatusBadRequest, "kind must be 'embed' or 'extract'")
		return
	}
	if body.Kind == "embed" {
		body.HostingURL = fileHostEmbed(body.HostingURL)
	}
	if body.Kind == "extract" && (body.Provider == nil || *body.Provider == "" ||
		body.ProviderRef == nil || *body.ProviderRef == "") {
		httpx.Error(w, http.StatusBadRequest, "extract sources need provider and providerRef")
		return
	}
	if body.Language == "" {
		body.Language = "ro"
	}
	in := repo.LinkInput{
		HostingURL: body.HostingURL, Quality: body.Quality, Language: body.Language,
		Kind: body.Kind, Provider: body.Provider, ProviderRef: body.ProviderRef,
		Priority: body.Priority,
	}
	if kind == "episode" {
		in.EpisodeID = &id
	} else {
		in.ChapterID = &id
	}
	link, err := h.repo.AddContentLink(r.Context(), in)
	if err != nil {
		httpx.Internal(w, "add content link", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": link})
}

type linkPatchBody struct {
	Quality  *string `json:"quality"`
	IsActive *bool   `json:"isActive"`
	Priority *int    `json:"priority"`
}

// PUT /api/links/{id}  (admin/translator) — patch priority/active/quality
func (h *Handler) updateLink(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid link ID")
		return
	}
	var body linkPatchBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	link, err := h.repo.UpdateContentLink(r.Context(), id, repo.LinkPatch{
		Quality: body.Quality, IsActive: body.IsActive, Priority: body.Priority,
	})
	if err != nil {
		notFoundOr(w, err, "Link not found", "update link")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": link})
}

// DELETE /api/links/{id}  (admin/translator)
func (h *Handler) deleteLink(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid link ID")
		return
	}
	if err := h.repo.DeleteContentLink(r.Context(), id); err != nil {
		notFoundOr(w, err, "Link not found", "delete link")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Link deleted successfully"})
}
