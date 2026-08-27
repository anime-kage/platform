// Package jikan wraps the Jikan API v4 (MAL metadata): global rate limiting
// (~3 req/s), 429 retry with backoff, and transformation into our schema's
// vocabulary. Port of the old services/jikan.ts.
package jikan

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	baseURL        = "https://api.jikan.moe/v4"
	rateLimitDelay = 334 * time.Millisecond
	maxRetries     = 3
)

type Client struct {
	http    *http.Client
	baseURL string // overridable in tests

	mu       sync.Mutex
	lastCall time.Time
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}, baseURL: baseURL}
}

// AnimeData is a new/updated anime row, pre-transformation into the DB.
type AnimeData struct {
	MalID         int
	Title         string
	TitleEnglish  *string
	Synopsis      *string
	Genres        []string
	Studios       []string
	Status        string
	Type          string
	Episodes      *int
	Score         *float64
	Year          *int
	Season        *string
	ImageURL      *string
	TrailerURL    *string
	BroadcastDay  *string
	BroadcastTime *string
}

type MangaData struct {
	MalID        int
	Title        string
	TitleEnglish *string
	Synopsis     *string
	Genres       []string
	Authors      []string
	Status       string
	Type         string
	Chapters     *int
	Volumes      *int
	Score        *float64
	Year         *int
	ImageURL     *string
}

type Page struct {
	CurrentPage int
	TotalPages  int
	HasNextPage bool
	Total       int
}

// ── Raw Jikan shapes ──────────────────────────────────────────────────────────

type named struct {
	Name string `json:"name"`
}

type images struct {
	JPG struct {
		ImageURL      string `json:"image_url"`
		LargeImageURL string `json:"large_image_url"`
	} `json:"jpg"`
}

type jikanAnime struct {
	MalID        int      `json:"mal_id"`
	Title        string   `json:"title"`
	TitleEnglish *string  `json:"title_english"`
	Synopsis     *string  `json:"synopsis"`
	Genres       []named  `json:"genres"`
	Studios      []named  `json:"studios"`
	Status       string   `json:"status"`
	Type         string   `json:"type"`
	Episodes     *int     `json:"episodes"`
	Score        *float64 `json:"score"`
	Year         *int     `json:"year"`
	Season       *string  `json:"season"`
	Images       images   `json:"images"`
	Trailer      *struct {
		URL *string `json:"url"`
	} `json:"trailer"`
	Broadcast *struct {
		Day  *string `json:"day"`
		Time *string `json:"time"`
	} `json:"broadcast"`
}

type jikanManga struct {
	MalID        int      `json:"mal_id"`
	Title        string   `json:"title"`
	TitleEnglish *string  `json:"title_english"`
	Synopsis     *string  `json:"synopsis"`
	Genres       []named  `json:"genres"`
	Authors      []named  `json:"authors"`
	Status       string   `json:"status"`
	Type         string   `json:"type"`
	Chapters     *int     `json:"chapters"`
	Volumes      *int     `json:"volumes"`
	Score        *float64 `json:"score"`
	Published    *struct {
		From *string `json:"from"`
	} `json:"published"`
	Images images `json:"images"`
}

type jikanPagination struct {
	LastVisiblePage int  `json:"last_visible_page"`
	HasNextPage     bool `json:"has_next_page"`
	CurrentPage     int  `json:"current_page"`
	Items           struct {
		Total int `json:"total"`
	} `json:"items"`
}

func (c *Client) get(endpoint string, out any) error {
	for attempt := 0; ; attempt++ {
		c.mu.Lock()
		if wait := rateLimitDelay - time.Since(c.lastCall); wait > 0 {
			time.Sleep(wait)
		}
		c.lastCall = time.Now()
		c.mu.Unlock()

		req, err := http.NewRequest(http.MethodGet, c.baseURL+endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "Anime-Kage/1.0.0")

		resp, err := c.http.Do(req)
		if err != nil {
			if attempt < maxRetries {
				time.Sleep(time.Second)
				continue
			}
			return err
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries:
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			continue
		// Jikan's gateway hiccups with transient 5xx fairly often — retry those
		// like a rate limit instead of failing the caller's request outright
		case resp.StatusCode >= 500 && attempt < maxRetries:
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			continue
		case resp.StatusCode == http.StatusNotFound:
			resp.Body.Close()
			return fmt.Errorf("resource not found: %s", endpoint)
		case resp.StatusCode != http.StatusOK:
			resp.Body.Close()
			return fmt.Errorf("jikan API error: %d", resp.StatusCode)
		}

		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		return err
	}
}

// ── Transformations ───────────────────────────────────────────────────────────

var animeStatusMap = map[string]string{
	"Currently Airing": "airing",
	"Finished Airing":  "completed",
	"Not yet aired":    "upcoming",
}

var animeTypeMap = map[string]string{
	"TV": "tv", "Movie": "movie", "OVA": "ova", "Special": "special", "ONA": "ova",
}

func mapOr(m map[string]string, key, fallback string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return fallback
}

func names(xs []named) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.Name
	}
	return out
}

func transformAnime(j jikanAnime) AnimeData {
	a := AnimeData{
		MalID:        j.MalID,
		Title:        j.Title,
		TitleEnglish: j.TitleEnglish,
		Synopsis:     j.Synopsis,
		Genres:       names(j.Genres),
		Studios:      names(j.Studios),
		Status:       mapOr(animeStatusMap, j.Status, "completed"),
		Type:         mapOr(animeTypeMap, j.Type, "tv"),
		Episodes:     j.Episodes,
		Score:        j.Score,
		Year:         j.Year,
		Season:       j.Season,
	}
	if img := j.Images.JPG.LargeImageURL; img != "" {
		a.ImageURL = &img
	} else if img := j.Images.JPG.ImageURL; img != "" {
		a.ImageURL = &img
	}
	if j.Trailer != nil {
		a.TrailerURL = j.Trailer.URL
	}
	if j.Broadcast != nil {
		if j.Broadcast.Day != nil {
			// "Saturdays" → "saturday", matching the old normalization
			day := strings.ToLower(strings.TrimSuffix(*j.Broadcast.Day, "s"))
			a.BroadcastDay = &day
		}
		a.BroadcastTime = j.Broadcast.Time
	}
	return a
}

var mangaStatusMap = map[string]string{
	"Publishing": "publishing", "Finished": "completed", "On Hiatus": "hiatus",
	"Discontinued": "discontinued", "Not yet published": "upcoming",
}

var mangaTypeMap = map[string]string{
	"Manga": "manga", "Manhwa": "manhwa", "Manhua": "manhua",
	"Novel": "novel", "Light Novel": "novel", "One-shot": "manga", "Doujinshi": "manga",
}

func transformManga(j jikanManga) MangaData {
	m := MangaData{
		MalID:        j.MalID,
		Title:        j.Title,
		TitleEnglish: j.TitleEnglish,
		Synopsis:     j.Synopsis,
		Genres:       names(j.Genres),
		Authors:      names(j.Authors),
		Status:       mapOr(mangaStatusMap, j.Status, "completed"),
		Type:         mapOr(mangaTypeMap, j.Type, "manga"),
		Chapters:     j.Chapters,
		Volumes:      j.Volumes,
		Score:        j.Score,
	}
	if j.Published != nil && j.Published.From != nil {
		if t, err := time.Parse(time.RFC3339, *j.Published.From); err == nil {
			y := t.Year()
			m.Year = &y
		}
	}
	if img := j.Images.JPG.LargeImageURL; img != "" {
		m.ImageURL = &img
	} else if img := j.Images.JPG.ImageURL; img != "" {
		m.ImageURL = &img
	}
	return m
}

// ── Public API ────────────────────────────────────────────────────────────────

// ErrEmptyPayload is a 200 response that carried no record — Jikan answering
// with a null/empty `data` object, which it does while MAL is refusing it.
//
// This has to be an error rather than a zero-valued result, because callers
// write what they get straight back to the catalog. `transformAnime` maps an
// empty payload to a *plausible-looking* row — MalID 0, empty Title, and
// Status "completed" via mapOr's default — so an unchecked write silently
// blanks a real series instead of failing. That is not hypothetical: it is what
// destroyed the identity of one anime row in production (the row survived only
// as an episode titled "My Cousin Girlfriend"), and the resulting mal_id=0 row
// then collided with every later occurrence, which is the
// `anime_mal_id_unique` violation the nightly autoupdate logged for weeks.
var ErrEmptyPayload = errors.New("jikan returned an empty record")

func (c *Client) AnimeByID(malID int) (*AnimeData, error) {
	var resp struct {
		Data jikanAnime `json:"data"`
	}
	if err := c.get(fmt.Sprintf("/anime/%d", malID), &resp); err != nil {
		return nil, err
	}
	if resp.Data.MalID == 0 || strings.TrimSpace(resp.Data.Title) == "" {
		return nil, fmt.Errorf("anime %d: %w", malID, ErrEmptyPayload)
	}
	a := transformAnime(resp.Data)
	return &a, nil
}

// EpisodeData is one entry from /anime/{id}/episodes. On that endpoint the
// mal_id IS the episode number.
type EpisodeData struct {
	Number int
	Title  *string
	Aired  *string // "YYYY-MM-DD"
	// MAL's own filler/recap marks — the same ones its episode list shows.
	// Not pointers: absent means false, which is also the correct default.
	Filler bool
	Recap  bool
}

// AnimeEpisodes fetches every episode page for a MAL id.
func (c *Client) AnimeEpisodes(malID int) ([]EpisodeData, error) {
	var all []EpisodeData
	for page := 1; ; page++ {
		var resp struct {
			Data []struct {
				MalID        int     `json:"mal_id"`
				Title        *string `json:"title"`
				TitleRomanji *string `json:"title_romanji"`
				Aired        *string `json:"aired"`
				Filler       bool    `json:"filler"`
				Recap        bool    `json:"recap"`
			} `json:"data"`
			Pagination jikanPagination `json:"pagination"`
		}
		if err := c.get(fmt.Sprintf("/anime/%d/episodes?page=%d", malID, page), &resp); err != nil {
			return nil, err
		}
		for _, e := range resp.Data {
			ep := EpisodeData{Number: e.MalID, Filler: e.Filler, Recap: e.Recap}
			// `title` is the English/official one and is what MAL's episode list
			// shows; `title_romanji` is the fallback for the entries that only
			// carry a transliteration.
			if e.Title != nil && strings.TrimSpace(*e.Title) != "" {
				ep.Title = e.Title
			} else if e.TitleRomanji != nil && strings.TrimSpace(*e.TitleRomanji) != "" {
				ep.Title = e.TitleRomanji
			}
			if e.Aired != nil {
				if t, err := time.Parse(time.RFC3339, *e.Aired); err == nil {
					d := t.Format("2006-01-02")
					ep.Aired = &d
				}
			}
			all = append(all, ep)
		}
		if !resp.Pagination.HasNextPage {
			return all, nil
		}
	}
}

func (c *Client) MangaByID(malID int) (*MangaData, error) {
	var resp struct {
		Data jikanManga `json:"data"`
	}
	if err := c.get(fmt.Sprintf("/manga/%d", malID), &resp); err != nil {
		return nil, err
	}
	// Same guard as AnimeByID, for the same reason — the manga sync writes
	// this straight back too.
	if resp.Data.MalID == 0 || strings.TrimSpace(resp.Data.Title) == "" {
		return nil, fmt.Errorf("manga %d: %w", malID, ErrEmptyPayload)
	}
	m := transformManga(resp.Data)
	return &m, nil
}

type SearchOpts struct {
	Page   int
	Limit  int
	Type   string
	Status string
	Year   int
}

func (c *Client) buildQuery(q string, o SearchOpts) url.Values {
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	page := o.Page
	if page < 1 {
		page = 1
	}
	limit := o.Limit
	if limit < 1 || limit > 25 {
		limit = 25 // Jikan max
	}
	v.Set("page", fmt.Sprint(page))
	v.Set("limit", fmt.Sprint(limit))
	if o.Type != "" {
		v.Set("type", o.Type)
	}
	if o.Status != "" {
		v.Set("status", o.Status)
	}
	if o.Year != 0 {
		v.Set("start_date", fmt.Sprintf("%d-01-01", o.Year))
	}
	return v
}

func (c *Client) SearchAnime(q string, o SearchOpts) ([]AnimeData, Page, error) {
	var resp struct {
		Data       []jikanAnime    `json:"data"`
		Pagination jikanPagination `json:"pagination"`
	}
	if err := c.get("/anime?"+c.buildQuery(q, o).Encode(), &resp); err != nil {
		return nil, Page{}, err
	}
	out := make([]AnimeData, len(resp.Data))
	for i, j := range resp.Data {
		out[i] = transformAnime(j)
	}
	return out, pageOf(resp.Pagination), nil
}

// TopAnime: kind is airing | upcoming | bypopularity | favorite. The old
// backend sent it as ?type= (invalid per Jikan v4 docs, silently ignored);
// ?filter= is the correct parameter, so trending/airing actually filter now.
func (c *Client) TopAnime(kind string, page, limit int) ([]AnimeData, Page, error) {
	v := c.buildQuery("", SearchOpts{Page: page, Limit: limit})
	v.Set("filter", kind)
	var resp struct {
		Data       []jikanAnime    `json:"data"`
		Pagination jikanPagination `json:"pagination"`
	}
	if err := c.get("/top/anime?"+v.Encode(), &resp); err != nil {
		return nil, Page{}, err
	}
	out := make([]AnimeData, len(resp.Data))
	for i, j := range resp.Data {
		out[i] = transformAnime(j)
	}
	return out, pageOf(resp.Pagination), nil
}

func (c *Client) SeasonalAnime(year int, season string, page int) ([]AnimeData, Page, error) {
	var resp struct {
		Data       []jikanAnime    `json:"data"`
		Pagination jikanPagination `json:"pagination"`
	}
	if err := c.get(fmt.Sprintf("/seasons/%d/%s?page=%d", year, season, page), &resp); err != nil {
		return nil, Page{}, err
	}
	out := make([]AnimeData, len(resp.Data))
	for i, j := range resp.Data {
		out[i] = transformAnime(j)
	}
	return out, pageOf(resp.Pagination), nil
}

func (c *Client) SearchManga(q string, o SearchOpts) ([]MangaData, Page, error) {
	var resp struct {
		Data       []jikanManga    `json:"data"`
		Pagination jikanPagination `json:"pagination"`
	}
	if err := c.get("/manga?"+c.buildQuery(q, o).Encode(), &resp); err != nil {
		return nil, Page{}, err
	}
	out := make([]MangaData, len(resp.Data))
	for i, j := range resp.Data {
		out[i] = transformManga(j)
	}
	return out, pageOf(resp.Pagination), nil
}

func (c *Client) TopManga(kind string, page, limit int) ([]MangaData, Page, error) {
	v := c.buildQuery("", SearchOpts{Page: page, Limit: limit})
	v.Set("filter", kind)
	var resp struct {
		Data       []jikanManga    `json:"data"`
		Pagination jikanPagination `json:"pagination"`
	}
	if err := c.get("/top/manga?"+v.Encode(), &resp); err != nil {
		return nil, Page{}, err
	}
	out := make([]MangaData, len(resp.Data))
	for i, j := range resp.Data {
		out[i] = transformManga(j)
	}
	return out, pageOf(resp.Pagination), nil
}

func pageOf(p jikanPagination) Page {
	return Page{
		CurrentPage: p.CurrentPage,
		TotalPages:  p.LastVisiblePage,
		HasNextPage: p.HasNextPage,
		Total:       p.Items.Total,
	}
}
