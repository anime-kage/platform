package games

// The comparison grid for the title puzzle.
//
// A guess is a real series, not a string, and what comes back is how it lines
// up with the answer attribute by attribute. That turns each attempt into
// information rather than a coin flip: wrong guesses narrow the field, which is
// the whole point of the format.
//
// Note what is compared. The catalogue carries year, season, type, episodes,
// studios, score and genres -- it has no "source" or "tags" columns, so those
// are not offered rather than faked from something adjacent.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Card is the slice of a series the grid needs.
type Card struct {
	ID       int
	Title    string
	Image    string
	Year     *int
	Season   *string
	Type     *string
	Episodes *int
	Score    *float64
	Studios  []string
	Genres   []string
}

// Cell states. Direction is carried separately so the client can draw an arrow
// without parsing the state.
const (
	StateHit  = "hit"  // exactly right
	StateNear = "near" // partial: some genres or studios shared
	StateMiss = "miss"
	DirUp     = "up"   // the answer is higher than this guess
	DirDown   = "down" // the answer is lower
)

// Cell is one column of one guess row.
type Cell struct {
	Key   string   `json:"key"`
	Value string   `json:"value"`
	State string   `json:"state"`
	Dir   string   `json:"dir,omitempty"`
	// Parts/PartStates carry multi-valued columns (genres, studios) so each
	// entry can be coloured on its own.
	Parts      []string `json:"parts,omitempty"`
	PartStates []string `json:"partStates,omitempty"`
}

// GuessRow is one attempt, ready to render.
type GuessRow struct {
	AnimeID int    `json:"animeId"`
	Title   string `json:"title"`
	Image   string `json:"image,omitempty"`
	Correct bool   `json:"correct"`
	Cells   []Cell `json:"cells"`
}

// Columns is the grid's shape, in display order. Exported so the client renders
// the same headers the server compares on and the two cannot drift.
var Columns = []struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}{
	{"year", "An"},
	{"season", "Sezon"},
	{"type", "Tip"},
	{"episodes", "Episoade"},
	{"studios", "Studio"},
	{"score", "Scor"},
	{"genres", "Genuri"},
}

func numCell(key string, guess, answer *int, fmtFn func(int) string) Cell {
	c := Cell{Key: key, State: StateMiss, Value: "?"}
	if guess == nil {
		return c
	}
	c.Value = fmtFn(*guess)
	if answer == nil {
		return c
	}
	switch {
	case *guess == *answer:
		c.State = StateHit
	case *answer > *guess:
		c.Dir = DirUp
	default:
		c.Dir = DirDown
	}
	return c
}

func textCell(key string, guess, answer *string, label func(string) string) Cell {
	c := Cell{Key: key, State: StateMiss, Value: "?"}
	if guess == nil || *guess == "" {
		return c
	}
	c.Value = label(*guess)
	if answer != nil && strings.EqualFold(*guess, *answer) {
		c.State = StateHit
	}
	return c
}

// setCell colours each entry of a list column individually, and gives the cell
// as a whole "hit" only when the two lists are identical -- a guess sharing one
// genre out of four has learned something, but it has not matched.
func setCell(key string, guess, answer []string, label func(string) string) Cell {
	c := Cell{Key: key, State: StateMiss}
	if len(guess) == 0 {
		c.Value = "?"
		return c
	}
	have := map[string]bool{}
	for _, a := range answer {
		have[strings.ToLower(a)] = true
	}
	shared := 0
	for _, g := range guess {
		c.Parts = append(c.Parts, label(g))
		if have[strings.ToLower(g)] {
			c.PartStates = append(c.PartStates, StateHit)
			shared++
		} else {
			c.PartStates = append(c.PartStates, StateMiss)
		}
	}
	switch {
	case shared == len(guess) && shared == len(answer):
		c.State = StateHit
	case shared > 0:
		c.State = StateNear
	}
	return c
}

// Compare builds one grid row.
func Compare(guess, answer Card, genre func(string) string, typ func(string) string) GuessRow {
	row := GuessRow{
		AnimeID: guess.ID,
		Title:   guess.Title,
		Image:   guess.Image,
		Correct: guess.ID == answer.ID,
	}
	same := func(s string) string { return s }
	row.Cells = []Cell{
		numCell("year", guess.Year, answer.Year, func(v int) string { return strconv.Itoa(v) }),
		textCell("season", guess.Season, answer.Season, same),
		textCell("type", guess.Type, answer.Type, typ),
		numCell("episodes", guess.Episodes, answer.Episodes, func(v int) string { return strconv.Itoa(v) }),
		setCell("studios", guess.Studios, answer.Studios, same),
		scoreCell(guess.Score, answer.Score),
		setCell("genres", guess.Genres, answer.Genres, genre),
	}
	// Keep Columns order.
	order := map[string]int{}
	for i, c := range Columns {
		order[c.Key] = i
	}
	sort.SliceStable(row.Cells, func(i, j int) bool { return order[row.Cells[i].Key] < order[row.Cells[j].Key] })
	return row
}

func scoreCell(guess, answer *float64) Cell {
	c := Cell{Key: "score", State: StateMiss, Value: "?"}
	if guess == nil {
		return c
	}
	c.Value = strconv.FormatFloat(*guess, 'f', 2, 64)
	if answer == nil {
		return c
	}
	switch {
	case fmt.Sprintf("%.2f", *guess) == fmt.Sprintf("%.2f", *answer):
		c.State = StateHit
	case *answer > *guess:
		c.Dir = DirUp
	default:
		c.Dir = DirDown
	}
	return c
}

// SummaryCell is one deduction the grid has established so far.
type SummaryCell struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	State string `json:"state"`
}

// Summarise folds every guess into what is now known, so a player does not have
// to re-read the whole grid to remember the bounds they have already proved.
func Summarise(rows []GuessRow) []SummaryCell {
	out := make([]SummaryCell, 0, len(Columns))
	for _, col := range Columns {
		out = append(out, summariseColumn(col.Key, rows))
	}
	return out
}

func summariseColumn(key string, rows []GuessRow) SummaryCell {
	s := SummaryCell{Key: key, Value: "—", State: StateMiss}
	var lo, hi *float64
	confirmed := map[string]bool{}

	for _, r := range rows {
		for _, c := range r.Cells {
			if c.Key != key {
				continue
			}
			if c.State == StateHit && len(c.Parts) == 0 {
				return SummaryCell{Key: key, Value: c.Value, State: StateHit}
			}
			for i, p := range c.Parts {
				if c.PartStates[i] == StateHit {
					confirmed[p] = true
				}
			}
			if v, err := strconv.ParseFloat(c.Value, 64); err == nil {
				switch c.Dir {
				case DirUp:
					if lo == nil || v > *lo {
						x := v
						lo = &x
					}
				case DirDown:
					if hi == nil || v < *hi {
						x := v
						hi = &x
					}
				}
			}
		}
	}

	if len(confirmed) > 0 {
		keys := make([]string, 0, len(confirmed))
		for k := range confirmed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return SummaryCell{Key: key, Value: strings.Join(keys, ", "), State: StateNear}
	}
	switch {
	case lo != nil && hi != nil:
		s.Value = fmt.Sprintf("%s – %s", trim(*lo), trim(*hi))
		s.State = StateNear
	case lo != nil:
		s.Value = "> " + trim(*lo)
		s.State = StateNear
	case hi != nil:
		s.Value = "< " + trim(*hi)
		s.State = StateNear
	}
	return s
}

func trim(f float64) string {
	if f == float64(int(f)) {
		return strconv.Itoa(int(f))
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}
