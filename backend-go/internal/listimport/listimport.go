// Package listimport turns somebody else's list — AniList or a MyAnimeList
// XML export — into rows we can write to watchlist/readlist.
//
// It owns the status vocabulary translation and nothing else: fetching lives
// in internal/anilist and internal/malxml, writing lives in internal/repo.
// Keeping the mapping here means the two sources cannot drift into disagreeing
// about what "on-hold" is called.
package listimport

import "time"

// Entry is the source-neutral shape both importers produce.
type Entry struct {
	MalID       int
	Title       string // only for reporting what could not be matched
	Status      string // already in OUR vocabulary
	Progress    int
	Score       int // 0–10, 0 = unscored
	Notes       string
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// Our five statuses, per media kind (see handler/users.go).
const (
	StatusWatching  = "watching"
	StatusReading   = "reading"
	StatusCompleted = "completed"
	StatusOnHold    = "on-hold"
	StatusDropped   = "dropped"
	StatusPlanWatch = "plan-to-watch"
	StatusPlanRead  = "plan-to-read"
)

// FromAniList maps AniList's vocabulary. REPEATING (a rewatch) collapses to
// watching/reading: we have no separate rewatch state, and "currently going
// through it again" is much closer to watching than to completed.
func FromAniList(status string, manga bool) (string, bool) {
	switch status {
	case "CURRENT", "REPEATING":
		return current(manga), true
	case "PLANNING":
		return planned(manga), true
	case "COMPLETED":
		return StatusCompleted, true
	case "PAUSED":
		return StatusOnHold, true
	case "DROPPED":
		return StatusDropped, true
	default:
		return "", false
	}
}

// FromMAL maps the strings MyAnimeList writes into its XML export. It uses
// prose ("Plan to Watch", "On-Hold") rather than an enum, and the manga file
// says "Reading" where the anime file says "Watching".
func FromMAL(status string, manga bool) (string, bool) {
	switch status {
	case "Watching", "Reading":
		return current(manga), true
	case "Plan to Watch", "Plan to Read":
		return planned(manga), true
	case "Completed":
		return StatusCompleted, true
	case "On-Hold":
		return StatusOnHold, true
	case "Dropped":
		return StatusDropped, true
	default:
		return "", false
	}
}

func current(manga bool) string {
	if manga {
		return StatusReading
	}
	return StatusWatching
}

func planned(manga bool) string {
	if manga {
		return StatusPlanRead
	}
	return StatusPlanWatch
}

// Result reports what an import did. Skipped titles are the ones we have no
// catalog entry for — the number that actually explains a disappointing
// import, so it is counted and sampled rather than swallowed.
type Result struct {
	Imported  int      `json:"imported"`
	Updated   int      `json:"updated"`
	Skipped   int      `json:"skipped"`
	Unmatched []string `json:"unmatched"` // a sample of titles we don't carry
}

// UnmatchedSample caps how many missing titles we name back to the user —
// enough to recognise the pattern, not a wall of text.
const UnmatchedSample = 12
