package anilist

// Episode titles, for when Jikan cannot supply them.
//
// This exists because MAL fails per-entry, not just globally: at the time of
// writing `/anime/32998/episodes` (91 Days) and `/anime/877/episodes` (NANA)
// return 504 on every attempt while `/anime/5114/episodes` (FMA) answers 200
// every time. A retry loop cannot fix that, and those series' episodes were
// sitting in the catalog with no titles at all.
//
// AniList has no `filler`/`recap` flags and no per-episode air dates, so this is
// strictly a titles fallback — Jikan stays the primary source because it carries
// all three.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// streamingEpisodes carries titles as one string, in the form the streaming site
// uses: "Episode 1 - Night of the Murder". The number has to be parsed back out
// because the list is not indexed and is not reliably in order.
//
// Tolerates "Episode 1", "Ep. 1", "E1" and a bare "1", and any of - – — : as the
// separator. Anything that does not match is skipped rather than guessed at: a
// wrong episode number is worse than a missing title.
var episodeTitleRe = regexp.MustCompile(`^(?:episode|episodio|ep\.?|e)?\s*(\d{1,4})\s*[-–—:]\s*(.+)$`)

// EpisodeTitlesByMal maps episode number → title for one series.
//
// Returns an empty map (not an error) when AniList knows the series but has no
// streaming episode list, which is common for older titles.
func (c *Client) EpisodeTitlesByMal(malID int) (map[int]string, error) {
	var res struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Media *struct {
				StreamingEpisodes []struct {
					Title string `json:"title"`
				} `json:"streamingEpisodes"`
			} `json:"Media"`
		} `json:"data"`
	}

	gql := `query ($mal: Int) {
		Media(idMal: $mal, type: ANIME) { streamingEpisodes { title } }
	}`
	if err := c.query(gql, map[string]any{"mal": malID}, &res); err != nil {
		return nil, err
	}
	if len(res.Errors) > 0 {
		return nil, fmt.Errorf("anilist: %s", res.Errors[0].Message)
	}

	out := map[int]string{}
	if res.Data.Media == nil {
		return out, nil
	}
	for _, e := range res.Data.Media.StreamingEpisodes {
		num, title, ok := parseEpisodeTitle(e.Title)
		if !ok {
			continue
		}
		// First match wins: several streaming sites can list the same episode,
		// and they mostly agree on the title.
		if _, seen := out[num]; !seen {
			out[num] = title
		}
	}
	return out, nil
}

// parseEpisodeTitle splits "Episode 3 - Where the Footfalls Lead" into 3 and the
// title. Exported behaviour is covered by tests.
func parseEpisodeTitle(raw string) (int, string, bool) {
	m := episodeTitleRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(raw)))
	if m == nil {
		return 0, "", false
	}
	num, err := strconv.Atoi(m[1])
	if err != nil || num <= 0 {
		return 0, "", false
	}
	// Re-cut the ORIGINAL string at the same offset rather than returning the
	// lowercased match: titles are proper nouns and "night of the murder" is not
	// what anyone wants to read.
	idx := strings.LastIndex(strings.ToLower(raw), m[2])
	if idx < 0 {
		return 0, "", false
	}
	title := strings.TrimSpace(raw[idx:])
	if title == "" {
		return 0, "", false
	}
	return num, title, true
}
