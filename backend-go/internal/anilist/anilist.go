// Package anilist is the fallback metadata source for when Jikan is down
// (MAL periodically blocks Jikan's servers for days at a time). AniList's
// public GraphQL API needs no auth and carries MAL ids (idMal), so results
// keep the same identity as the Jikan path and land in the same tables.
package anilist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"animekage/backend/internal/jikan"
)

const defaultURL = "https://graphql.anilist.co"

type Client struct {
	http    *http.Client
	baseURL string // overridable in tests
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}, baseURL: defaultURL}
}

// media mirrors the fields we select in both queries.
type media struct {
	IDMal *int `json:"idMal"`
	Title struct {
		Romaji  string  `json:"romaji"`
		English *string `json:"english"`
	} `json:"title"`
	Description *string  `json:"description"`
	Format      string   `json:"format"`
	Status      string   `json:"status"`
	Episodes    *int     `json:"episodes"`
	SeasonYear  *int     `json:"seasonYear"`
	Season      *string  `json:"season"`
	AvgScore    *float64 `json:"averageScore"` // 0–100
	Genres      []string `json:"genres"`
	Studios     struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"studios"`
	CoverImage struct {
		ExtraLarge string `json:"extraLarge"`
		Large      string `json:"large"`
	} `json:"coverImage"`
	Trailer *struct {
		ID   string `json:"id"`
		Site string `json:"site"`
	} `json:"trailer"`
}

const mediaFields = `idMal title { romaji english } description(asHtml: false)
	format status episodes seasonYear season averageScore genres
	studios(isMain: true) { nodes { name } }
	coverImage { extraLarge large } trailer { id site }`

func (c *Client) query(q string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": q, "variables": vars})
	if err != nil {
		return err
	}
	// AniList rate-limits by minute and says how long to wait in Retry-After.
	// Without this a bulk job (relations asks in pages of 50, so the whole
	// catalog is dozens of calls back to back) turns one 429 into a hole in the
	// data that looks like "AniList has no relations for these titles".
	for attempt := 0; ; attempt++ {
		resp, err := c.http.Post(c.baseURL, "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("anilist request: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 4 {
			wait := 20 * time.Second
			if s := resp.Header.Get("Retry-After"); s != "" {
				if n, convErr := strconv.Atoi(s); convErr == nil && n > 0 && n <= 120 {
					wait = time.Duration(n) * time.Second
				}
			}
			resp.Body.Close()
			slog.Warn("anilist rate limited, waiting", "seconds", wait.Seconds(), "attempt", attempt+1)
			time.Sleep(wait)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("anilist responded %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
}

// SearchAnime searches by title; entries without a MAL id are dropped so the
// import-by-malId flow stays intact.
func (c *Client) SearchAnime(q string, limit int) ([]jikan.AnimeData, error) {
	var res struct {
		Data struct {
			Page struct {
				Media []media `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}
	gql := fmt.Sprintf(`query ($q: String, $n: Int) {
		Page(perPage: $n) { media(search: $q, type: ANIME) { %s } } }`, mediaFields)
	if err := c.query(gql, map[string]any{"q": q, "n": limit}, &res); err != nil {
		return nil, err
	}
	out := make([]jikan.AnimeData, 0, len(res.Data.Page.Media))
	for _, m := range res.Data.Page.Media {
		if m.IDMal == nil {
			continue
		}
		out = append(out, transform(m))
	}
	return out, nil
}

// AnimeByMal fetches one title by its MAL id.
func (c *Client) AnimeByMal(malID int) (*jikan.AnimeData, error) {
	var res struct {
		Data struct {
			Media *media `json:"Media"`
		} `json:"data"`
	}
	gql := fmt.Sprintf(`query ($id: Int) { Media(idMal: $id, type: ANIME) { %s } }`, mediaFields)
	if err := c.query(gql, map[string]any{"id": malID}, &res); err != nil {
		return nil, err
	}
	if res.Data.Media == nil || res.Data.Media.IDMal == nil {
		return nil, fmt.Errorf("anilist: no anime with MAL id %d", malID)
	}
	d := transform(*res.Data.Media)
	return &d, nil
}

// mangaMedia is the manga-flavoured selection: chapters/volumes instead of
// episodes, staff (story/art) instead of studios.
type mangaMedia struct {
	IDMal *int `json:"idMal"`
	Title struct {
		Romaji  string  `json:"romaji"`
		English *string `json:"english"`
	} `json:"title"`
	Description *string  `json:"description"`
	Format      string   `json:"format"`
	Status      string   `json:"status"`
	Chapters    *int     `json:"chapters"`
	Volumes     *int     `json:"volumes"`
	AvgScore    *float64 `json:"averageScore"` // 0–100
	Genres      []string `json:"genres"`
	StartDate   struct {
		Year *int `json:"year"`
	} `json:"startDate"`
	Staff struct {
		Edges []struct {
			Role string `json:"role"`
			Node struct {
				Name struct {
					Full string `json:"full"`
				} `json:"name"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"staff"`
	CoverImage struct {
		ExtraLarge string `json:"extraLarge"`
		Large      string `json:"large"`
	} `json:"coverImage"`
}

const mangaMediaFields = `idMal title { romaji english } description(asHtml: false)
	format status chapters volumes averageScore genres startDate { year }
	staff(perPage: 4) { edges { role node { name { full } } } }
	coverImage { extraLarge large }`

// SearchManga is SearchAnime's manga twin; entries without a MAL id are
// dropped so the import-by-malId flow stays intact.
func (c *Client) SearchManga(q string, limit int) ([]jikan.MangaData, error) {
	var res struct {
		Data struct {
			Page struct {
				Media []mangaMedia `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}
	gql := fmt.Sprintf(`query ($q: String, $n: Int) {
		Page(perPage: $n) { media(search: $q, type: MANGA) { %s } } }`, mangaMediaFields)
	if err := c.query(gql, map[string]any{"q": q, "n": limit}, &res); err != nil {
		return nil, err
	}
	out := make([]jikan.MangaData, 0, len(res.Data.Page.Media))
	for _, m := range res.Data.Page.Media {
		if m.IDMal == nil {
			continue
		}
		out = append(out, transformManga(m))
	}
	return out, nil
}

// MangaByMal fetches one manga by its MAL id.
func (c *Client) MangaByMal(malID int) (*jikan.MangaData, error) {
	var res struct {
		Data struct {
			Media *mangaMedia `json:"Media"`
		} `json:"data"`
	}
	gql := fmt.Sprintf(`query ($id: Int) { Media(idMal: $id, type: MANGA) { %s } }`, mangaMediaFields)
	if err := c.query(gql, map[string]any{"id": malID}, &res); err != nil {
		return nil, err
	}
	if res.Data.Media == nil || res.Data.Media.IDMal == nil {
		return nil, fmt.Errorf("anilist: no manga with MAL id %d", malID)
	}
	d := transformManga(*res.Data.Media)
	return &d, nil
}

var (
	formatMap = map[string]string{
		"TV": "tv", "TV_SHORT": "tv", "MOVIE": "movie",
		"OVA": "ova", "ONA": "ova", "SPECIAL": "special", "MUSIC": "special",
	}
	statusMap = map[string]string{
		"FINISHED": "completed", "RELEASING": "airing",
		"NOT_YET_RELEASED": "upcoming", "CANCELLED": "completed", "HIATUS": "airing",
	}
	// targets the same value sets as jikan's mangaStatusMap/mangaTypeMap
	mangaFormatMap = map[string]string{
		"MANGA": "manga", "ONE_SHOT": "manga", "NOVEL": "novel",
	}
	mangaStatusMap = map[string]string{
		"FINISHED": "completed", "RELEASING": "publishing",
		"NOT_YET_RELEASED": "upcoming", "CANCELLED": "discontinued", "HIATUS": "hiatus",
	}
	brTag  = regexp.MustCompile(`(?i)<br\s*/?>`)
	anyTag = regexp.MustCompile(`<[^>]+>`)
)

func mapOr(m map[string]string, key, fallback string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return fallback
}

// transform maps an AniList media into the same shape the Jikan client
// produces, so the repo import path is identical for both sources.
func transform(m media) jikan.AnimeData {
	a := jikan.AnimeData{
		MalID:        *m.IDMal,
		Title:        m.Title.Romaji,
		TitleEnglish: m.Title.English,
		Genres:       m.Genres,
		Status:       mapOr(statusMap, m.Status, "completed"),
		Type:         mapOr(formatMap, m.Format, "tv"),
		Episodes:     m.Episodes,
		Year:         m.SeasonYear,
	}
	if m.Description != nil {
		syn := brTag.ReplaceAllString(*m.Description, "\n")
		syn = strings.TrimSpace(anyTag.ReplaceAllString(syn, ""))
		if syn != "" {
			a.Synopsis = &syn
		}
	}
	if m.AvgScore != nil {
		score := *m.AvgScore / 10
		a.Score = &score
	}
	if m.Season != nil {
		season := strings.ToLower(*m.Season)
		a.Season = &season
	}
	for _, s := range m.Studios.Nodes {
		a.Studios = append(a.Studios, s.Name)
	}
	if img := m.CoverImage.ExtraLarge; img != "" {
		a.ImageURL = &img
	} else if img := m.CoverImage.Large; img != "" {
		a.ImageURL = &img
	}
	if m.Trailer != nil && strings.EqualFold(m.Trailer.Site, "youtube") && m.Trailer.ID != "" {
		url := "https://www.youtube.com/watch?v=" + m.Trailer.ID
		a.TrailerURL = &url
	}
	return a
}

// transformManga maps an AniList manga into jikan.MangaData, same contract
// as transform above.
func transformManga(m mangaMedia) jikan.MangaData {
	d := jikan.MangaData{
		MalID:        *m.IDMal,
		Title:        m.Title.Romaji,
		TitleEnglish: m.Title.English,
		Genres:       m.Genres,
		Status:       mapOr(mangaStatusMap, m.Status, "completed"),
		Type:         mapOr(mangaFormatMap, m.Format, "manga"),
		Chapters:     m.Chapters,
		Volumes:      m.Volumes,
		Year:         m.StartDate.Year,
	}
	if m.Description != nil {
		syn := brTag.ReplaceAllString(*m.Description, "\n")
		syn = strings.TrimSpace(anyTag.ReplaceAllString(syn, ""))
		if syn != "" {
			d.Synopsis = &syn
		}
	}
	if m.AvgScore != nil {
		score := *m.AvgScore / 10
		d.Score = &score
	}
	for _, e := range m.Staff.Edges {
		// AniList roles read "Story & Art", "Story", "Art" — those are the authors
		if strings.Contains(e.Role, "Story") || strings.Contains(e.Role, "Art") {
			d.Authors = append(d.Authors, e.Node.Name.Full)
		}
	}
	if img := m.CoverImage.ExtraLarge; img != "" {
		d.ImageURL = &img
	} else if img := m.CoverImage.Large; img != "" {
		d.ImageURL = &img
	}
	return d
}
