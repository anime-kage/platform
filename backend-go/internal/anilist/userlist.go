package anilist

// Importing a member's existing list.
//
// AniList is the only one of the two that makes this easy: MediaListCollection
// is public, unauthenticated, and returns a whole list — status, progress,
// score, dates — in a single request. Jikan's /users/* endpoints scrape MAL
// and answer 504 far too often to build on, and MAL's own v2 API needs a
// registered client id, so MAL is imported from its XML export instead
// (see internal/malxml).

import (
	"fmt"
	"strings"
	"time"
)

// ListEntry is one row of somebody's list, already flattened out of AniList's
// per-list nesting. MalID is the join key: our catalog's identity is the MAL
// id, and AniList carries it for ~99% of entries.
type ListEntry struct {
	MalID       int
	Title       string
	Status      string // AniList vocabulary: CURRENT, PLANNING, …
	Progress    int
	Score       float64 // 0–10, 0 = unscored
	Notes       string
	StartedAt   *time.Time
	CompletedAt *time.Time
}

type fuzzyDate struct {
	Year  *int `json:"year"`
	Month *int `json:"month"`
	Day   *int `json:"day"`
}

// toTime returns a date only when it is complete. AniList lets people record
// "2019" with no month or day, and inventing January 1st would show a
// start date the member never entered.
func (d fuzzyDate) toTime() *time.Time {
	if d.Year == nil || d.Month == nil || d.Day == nil {
		return nil
	}
	t := time.Date(*d.Year, time.Month(*d.Month), *d.Day, 0, 0, 0, 0, time.UTC)
	return &t
}

// AniList answers HTTP 200 even for "Private User" / "User not found",
// putting the reason in `errors` — so both levels have to be decoded or a
// refused list looks exactly like an empty one.
type listResponse struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Data struct {
		MediaListCollection *struct {
			Lists []struct {
				Entries []struct {
					Status      string    `json:"status"`
					Progress    int       `json:"progress"`
					Score       float64   `json:"score"`
					Notes       *string   `json:"notes"`
					StartedAt   fuzzyDate `json:"startedAt"`
					CompletedAt fuzzyDate `json:"completedAt"`
					Media       struct {
						IDMal *int `json:"idMal"`
						Title struct {
							Romaji  string  `json:"romaji"`
							English *string `json:"english"`
						} `json:"title"`
					} `json:"media"`
				} `json:"entries"`
			} `json:"lists"`
		} `json:"MediaListCollection"`
	} `json:"data"`
}

// score(format: POINT_10) normalises every user's scale — AniList members can
// set 100-point, 5-star or smiley scoring, and importing those raw would put
// a "3" next to a favourite series.
const userListQuery = `query ($name: String, $type: MediaType) {
  MediaListCollection(userName: $name, type: $type) {
    lists {
      entries {
        status progress score(format: POINT_10) notes
        startedAt { year month day }
        completedAt { year month day }
        media { idMal title { romaji english } }
      }
    }
  }
}`

// UserList fetches a public AniList list. kind is "ANIME" or "MANGA".
//
// Custom lists are flattened away deliberately: an entry can appear in
// several of them ("PTW 2024", "Dubbed"), but its `status` is a property of
// the entry itself, so de-duplicating by MAL id keeps exactly one row per
// title with the status AniList itself considers current.
func (c *Client) UserList(username, kind string) ([]ListEntry, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}

	var resp listResponse
	if err := c.query(userListQuery, map[string]any{"name": username, "type": kind}, &resp); err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("anilist: %s", resp.Errors[0].Message)
	}
	if resp.Data.MediaListCollection == nil {
		// no error and no collection: the account exists but has no list of
		// this type, which is normal for anime-only or manga-only users
		return nil, nil
	}

	seen := make(map[int]bool)
	var out []ListEntry
	for _, list := range resp.Data.MediaListCollection.Lists {
		for _, e := range list.Entries {
			if e.Media.IDMal == nil || seen[*e.Media.IDMal] {
				continue
			}
			seen[*e.Media.IDMal] = true

			title := e.Media.Title.Romaji
			if e.Media.Title.English != nil && *e.Media.Title.English != "" {
				title = *e.Media.Title.English
			}
			var notes string
			if e.Notes != nil {
				notes = *e.Notes
			}
			out = append(out, ListEntry{
				MalID:       *e.Media.IDMal,
				Title:       title,
				Status:      e.Status,
				Progress:    e.Progress,
				Score:       e.Score,
				Notes:       notes,
				StartedAt:   e.StartedAt.toTime(),
				CompletedAt: e.CompletedAt.toTime(),
			})
		}
	}
	return out, nil
}
