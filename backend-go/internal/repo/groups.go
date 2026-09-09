package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/games"
)

// GroupSet is one hidden group: four titles that share something, and the label
// that says what. The label is only ever sent to the browser once the group has
// been found -- before that it is the answer.
type GroupSet struct {
	Label    string `json:"label"`
	AnimeIDs []int  `json:"anime_ids"`
	// Kind and Value carry the raw fact so the browser can write the label with
	// the catalogue's own helpers. Go has no business translating a genre: the
	// site already decides which ones become Romanian and which stay English
	// (Mecha, Isekai, Slice of Life), and a second mapping here would drift.
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// groupCandidate is a series the generator may use, with the traits it can be
// grouped by.
type groupCandidate struct {
	ID      int      `db:"id"`
	Title   string   `db:"title"`
	Studios []string `db:"studios"`
	Genres  []string `db:"genres"`
	Year    *int     `db:"year"`
}

// buildGroupSets picks GroupCount groups whose members do not overlap.
//
// Overlap is the whole difficulty: a series belongs to a studio AND a genre AND
// a year, so the obvious approach -- pick four traits, take four series each --
// routinely puts the same title in two groups and makes the board unsolvable.
// So members are claimed as groups are built, and a trait that can no longer
// field GroupSize unclaimed series is skipped.
//
// Traits are tried in a deliberate order. Studio and year are specific enough
// to feel like a discovery; genre is broad and many series carry several, so it
// fills in rather than leads.
func buildGroupSets(pool []groupCandidate, rnd *rand.Rand) ([]GroupSet, bool) {
	type trait struct {
		label   string
		kind    string
		value   string
		members []int
	}
	byTrait := map[string]*trait{}
	add := func(key, label, kind, value string, id int) {
		t, ok := byTrait[key]
		if !ok {
			t = &trait{label: label, kind: kind, value: value}
			byTrait[key] = t
		}
		t.members = append(t.members, id)
	}
	for _, c := range pool {
		for _, s := range c.Studios {
			if s != "" {
				add("s:"+s, "Studio: "+s, "studio", s, c.ID)
			}
		}
		if c.Year != nil && *c.Year > 0 {
			y := strconv.Itoa(*c.Year)
			add("y:"+y, "Anul "+y, "year", y, c.ID)
		}
		for _, g := range c.Genres {
			if g != "" {
				add("g:"+g, "Gen: "+g, "genre", g, c.ID)
			}
		}
	}

	keys := make([]string, 0, len(byTrait))
	for k, t := range byTrait {
		if len(t.members) >= games.GroupSize {
			keys = append(keys, k)
		}
	}
	// Shuffle so the same studios do not lead every day, then sort the narrow
	// traits first: a trait with exactly enough members has no slack, and
	// placing it after a broad one usually means it can no longer be filled.
	rnd.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	// Which kind of trait leads is decided per board, not fixed. A hardcoded
	// studio-then-year-then-genre order meant the two studio slots and the two
	// year slots filled the board every time and genre was never once reached,
	// so a third of the vocabulary was dead and every board looked alike.
	kinds := []byte{'s', 'y', 'g'}
	rnd.Shuffle(len(kinds), func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })
	rank := map[byte]int{}
	for i, k := range kinds {
		rank[k] = i
	}
	prio := func(k string) int { return rank[k[0]] }
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && prio(keys[j]) < prio(keys[j-1]); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}

	// No more than two groups from any one trait type. Without this the board
	// came out four studios every single day: studios sort first and the pool
	// always has enough of them, so year and genre were never reached. A board
	// where every group is the same kind of fact is one puzzle repeated four
	// times, and it gives away the grouping rule for free.
	const maxPerType = 2
	claimed := map[int]bool{}
	perType := map[byte]int{}
	out := []GroupSet{}
	for _, k := range keys {
		if len(out) == games.GroupCount {
			break
		}
		if perType[k[0]] >= maxPerType {
			continue
		}
		t := byTrait[k]
		free := make([]int, 0, len(t.members))
		for _, id := range t.members {
			if !claimed[id] {
				free = append(free, id)
			}
		}
		if len(free) < games.GroupSize {
			continue
		}
		rnd.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
		pick := free[:games.GroupSize]
		for _, id := range pick {
			claimed[id] = true
		}
		perType[k[0]]++
		out = append(out, GroupSet{Label: t.label, AnimeIDs: pick, Kind: t.kind, Value: t.value})
	}
	return out, len(out) == games.GroupCount
}

// EnsureGroupsPuzzle writes one Grupe round per missing day in the window.
func (r *Repo) EnsureGroupsPuzzle(ctx context.Context, days int) (int, error) {
	var pool []groupCandidate
	// The same popular slice the title and poster rounds draw from: a board of
	// sixteen titles nobody recognises is not a puzzle, it is a list.
	if err := pgxscan.Select(ctx, r.pool, &pool, `
		SELECT id, title, COALESCE(studios, '{}') AS studios,
		       COALESCE(genres, '{}') AS genres, year
		  FROM anime
		 WHERE image_url IS NOT NULL AND score IS NOT NULL
		 ORDER BY score DESC NULLS LAST
		 LIMIT 700`); err != nil {
		return 0, fmt.Errorf("load group pool: %w", err)
	}
	if len(pool) < games.GroupTiles {
		return 0, nil
	}

	made := 0
	rnd := rand.New(rand.NewSource(rand.Int63()))
	for back := 0; back < days; back++ {
		sets, ok := buildGroupSets(pool, rnd)
		if !ok {
			continue // not enough disjoint traits today; the mode just sits out
		}
		payload, err := json.Marshal(sets)
		if err != nil {
			return made, err
		}
		tag, err := r.pool.Exec(ctx, `
			INSERT INTO game_puzzles (mode, play_date, groups)
			VALUES ($1, CURRENT_DATE - $2::int, $3)
			ON CONFLICT (mode, play_date) DO NOTHING`,
			games.ModeGroups, back, payload)
		if err != nil {
			return made, fmt.Errorf("ensure groups -%d: %w", back, err)
		}
		made += int(tag.RowsAffected())
	}
	return made, nil
}

// GroupTile is one card as the browser needs it.
type GroupTile struct {
	AnimeID int     `db:"id"    json:"animeId"`
	Title   string  `db:"title" json:"title"`
	Image   *string `db:"image_url" json:"image,omitempty"`
}

// GroupTiles loads the sixteen cards of a board in one query.
func (r *Repo) GroupTiles(ctx context.Context, ids []int) ([]GroupTile, error) {
	rows := []GroupTile{}
	if len(ids) == 0 {
		return rows, nil
	}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT id, title, image_url FROM anime WHERE id = ANY($1)`, ids)
	return rows, err
}

// ParseGroups reads the stored payload. A board that will not parse is treated
// as no board rather than a panic: the mode sits out that day.
func ParseGroups(raw *string) []GroupSet {
	if raw == nil || *raw == "" {
		return nil
	}
	var sets []GroupSet
	if err := json.Unmarshal([]byte(*raw), &sets); err != nil {
		return nil
	}
	return sets
}

// RecordGroupRound saves the submission history and, once the round is over,
// the payout.
//
// Upsert rather than insert: a Grupe round is played across several requests,
// so the row is created by the first submission and updated by every one after.
// awarded_xp/gold are written only at the end, which is what keeps a player
// from banking a payout per group.
func (r *Repo) RecordGroupRound(ctx context.Context, puzzleID int64, userID int,
	submissions [][]int, over, solved bool, xp, gold int) error {

	payload, err := json.Marshal(submissions)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO game_attempts (puzzle_id, user_id, guesses, solved_at, awarded_xp, awarded_gold, updated_at)
		VALUES ($1, $2, $3, CASE WHEN $4 THEN now() END, $5, $6, now())
		ON CONFLICT (puzzle_id, user_id) DO UPDATE
		SET guesses      = EXCLUDED.guesses,
		    solved_at    = COALESCE(game_attempts.solved_at, EXCLUDED.solved_at),
		    awarded_xp   = EXCLUDED.awarded_xp,
		    awarded_gold = EXCLUDED.awarded_gold,
		    updated_at   = now()`,
		puzzleID, userID, payload, solved, xp, gold)
	if err != nil {
		return err
	}
	if !over || (xp == 0 && gold == 0) {
		return nil
	}
	// AwardXPGold is the only thing that may move a balance: it writes the
	// ledger row and the cached totals in one transaction.
	return r.AwardXPGold(ctx, userID, xp, gold, "groups_round", fmt.Sprint(puzzleID))
}
