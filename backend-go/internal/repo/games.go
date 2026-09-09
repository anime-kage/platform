package repo

// Arcade storage: the player's profile, the daily puzzles, and the attempts
// against them.
//
// Two rules shape everything here. The answer never leaves the server until a
// puzzle is finished for that player, and every change to xp or gold goes
// through AwardXPGold so the ledger stays a complete record.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
)

// CharSlots mirrors games.CharSlots; repeated here so the SQL that builds a
// round does not have to import the rules package.
const CharSlots = 5

// GameProfile is the player's standing. Faction is the ACTIVE one; experience
// for it lives in game_faction_xp. Gold is shared across every faction.
type GameProfile struct {
	UserID  int     `db:"user_id"  json:"-"`
	Faction *string `db:"faction"  json:"faction"`
	Gold    int64   `db:"gold"     json:"gold"`
	// Experience earned before picking a faction, waiting for one.
	PendingXp int64 `db:"pending_xp" json:"pendingXp"`
}

// FactionXP is one row of a player's per-faction progress, for the picker.
type FactionXP struct {
	Faction string `db:"faction" json:"faction"`
	Xp      int64  `db:"xp"      json:"xp"`
}

// EnsureProfile returns the player's row, creating it on first sight so no
// caller has to care whether they have played before.
func (r *Repo) EnsureProfile(ctx context.Context, userID int) (*GameProfile, error) {
	var p GameProfile
	err := pgxscan.Get(ctx, r.pool, &p, `
		INSERT INTO game_profiles (user_id) VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING user_id, faction, gold, pending_xp`, userID)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SetFaction switches the active faction. Always allowed: the per-faction
// experience rows mean nothing is lost by moving, and a player should be able
// to try a ladder before committing to it. Returns how much banked XP the move
// just claimed, so the UI can say so.
func (r *Repo) SetFaction(ctx context.Context, userID int, code string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// Read the balance before zeroing it -- RETURNING would hand back the new
	// value. FOR UPDATE holds the row for the rest of the transaction, so a
	// concurrent award cannot bank XP between the read and the clear and have
	// it silently dropped.
	var claimed int
	err = tx.QueryRow(ctx,
		`SELECT pending_xp FROM game_profiles WHERE user_id = $1 FOR UPDATE`, userID).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE game_profiles SET faction = $2, pending_xp = 0 WHERE user_id = $1`,
		userID, code); err != nil {
		return 0, err
	}

	// Materialise the row now so the picker can show 0 XP rather than nothing,
	// and fold in whatever was waiting.
	if _, err := tx.Exec(ctx, `
		INSERT INTO game_faction_xp (user_id, faction, xp) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, faction) DO UPDATE
		   SET xp = game_faction_xp.xp + $3`, userID, code, claimed); err != nil {
		return 0, err
	}
	return claimed, tx.Commit(ctx)
}

// FactionProgress lists every faction this player has earned anything in.
func (r *Repo) FactionProgress(ctx context.Context, userID int) ([]FactionXP, error) {
	out := []FactionXP{}
	err := pgxscan.Select(ctx, r.pool, &out,
		`SELECT faction, xp FROM game_faction_xp WHERE user_id = $1`, userID)
	return out, err
}

// ActiveXP is the experience on the player's current faction, 0 when none.
func (r *Repo) ActiveXP(ctx context.Context, userID int) (int64, error) {
	var xp int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(fx.xp, 0)
		  FROM game_profiles p
		  LEFT JOIN game_faction_xp fx
		         ON fx.user_id = p.user_id AND fx.faction = p.faction
		 WHERE p.user_id = $1`, userID).Scan(&xp)
	if err != nil {
		return 0, err
	}
	return xp, nil
}

// AwardXPGold writes one ledger row and moves the cached totals with it, in a
// single transaction. Nothing else in the codebase should touch game_profiles.xp
// or .gold -- if it does, the ledger stops being able to explain a balance.
func (r *Repo) AwardXPGold(ctx context.Context, userID, xp, gold int, reason, ref string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO game_ledger (user_id, xp_delta, gold_delta, reason, ref)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))`, userID, xp, gold, reason, ref); err != nil {
		return err
	}
	// Only the parameters this statement actually uses. Passing xp to a query
	// that never references it left Postgres unable to infer $2's type, and the
	// whole award failed with 42P18 -- which surfaced as "Failed to award".
	if _, err := tx.Exec(ctx, `
		INSERT INTO game_profiles (user_id, gold) VALUES ($1, GREATEST($2,0))
		ON CONFLICT (user_id) DO UPDATE
		   SET gold = GREATEST(game_profiles.gold + $2, 0)`, userID, gold); err != nil {
		return err
	}
	// Experience attaches to whichever faction is active.
	if _, err := tx.Exec(ctx, `
		INSERT INTO game_faction_xp (user_id, faction, xp)
		SELECT p.user_id, p.faction, GREATEST($2,0)
		  FROM game_profiles p
		 WHERE p.user_id = $1 AND p.faction IS NOT NULL
		ON CONFLICT (user_id, faction) DO UPDATE
		   SET xp = GREATEST(game_faction_xp.xp + $2, 0)`, userID, xp); err != nil {
		return err
	}
	// With no faction chosen the statement above matches nothing, so the xp is
	// held on the profile until there is somewhere to put it. SetFaction moves
	// it across. Dropping it here is what made a new player's first wins count
	// for nothing.
	if _, err := tx.Exec(ctx, `
		UPDATE game_profiles SET pending_xp = GREATEST(pending_xp + $2, 0)
		 WHERE user_id = $1 AND faction IS NULL`, userID, xp); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// PuzzleRow is one day's puzzle joined to the answer's display data. Answer
// fields are only ever copied into a response once that player is finished.
type PuzzleRow struct {
	ID        int64     `db:"id"`
	Mode      string    `db:"mode"`
	PlayDate  time.Time `db:"play_date"`
	AnimeID   int       `db:"anime_id"`
	Title     string    `db:"title"`
	TitleEn   *string   `db:"title_english"`
	TitleRo   *string   `db:"title_romanian"`
	ImageURL  *string   `db:"image_url"`
	Year      *int      `db:"year"`
	Type      *string   `db:"type"`
	Episodes  *int      `db:"episodes"`
	// genres/studios are text[] in Postgres, not text.
	Genres    []string  `db:"genres"`
	Studios   []string  `db:"studios"`
	Score     *float64  `db:"score"`
	Slug      *string   `db:"slug"`
	// theme mode only
	ThemeSlug  *string `db:"theme_slug"`
	ThemeSong  *string `db:"theme_song"`
	ThemeAudio *string `db:"theme_audio"`
	ThemeVideo *string `db:"theme_video"`
	Synopsis  *string   `db:"synopsis"`
	// The catalogue keeps a translated synopsis where one exists; the hint
	// should read like the rest of the site, not fall back to English.
	SynopsisRo *string  `db:"synopsis_romanian"`
	// attempt state, null when the player has never opened this one
	Guesses   *string    `db:"guesses"`
	SolvedAt  *time.Time `db:"solved_at"`
	GaveUp    *bool      `db:"gave_up"`
	AwardedXp *int       `db:"awarded_xp"`
	UsedHint  *bool      `db:"used_hint"`
	CharIDs   []int      `db:"char_ids"`
	CharAnswers *string  `db:"char_answers"`
	// Grupe: the four hidden groups, as written by the generator. Never sent
	// to the browser wholesale -- the handler reveals a group only once found.
	Groups    *string    `db:"groups"`
}

const puzzleCols = `p.id, p.mode, p.play_date,
	-- character rounds have no single series, so this is NULL for them. 0 is
	-- never a valid anime id, so the comparisons that use it fail safely.
	coalesce(p.anime_id, 0) AS anime_id,
	coalesce(a.title, '') AS title, a.title_english, a.title_romanian,
	a.image_url, a.year, a.type,
	a.episodes, a.genres, a.studios, a.score, a.slug, a.synopsis, a.synopsis_romanian,
	th.slug AS theme_slug, th.song_title AS theme_song,
	th.audio_url AS theme_audio, th.video_url AS theme_video,
	att.guesses::text AS guesses, att.solved_at, att.gave_up, att.awarded_xp,
	att.used_hint, p.char_ids, att.char_answers::text AS char_answers,
	p.groups::text AS groups`

// Board returns every puzzle still inside the playable window, newest first,
// with this player's progress attached.
func (r *Repo) Board(ctx context.Context, userID, days int) ([]PuzzleRow, error) {
	rows := []PuzzleRow{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+puzzleCols+`
		  FROM game_puzzles p
		  LEFT JOIN anime a ON a.id = p.anime_id
		  LEFT JOIN game_themes th ON th.id = p.theme_id
		  LEFT JOIN game_attempts att
		         ON att.puzzle_id = p.id AND att.user_id = $1
		 WHERE p.play_date > (CURRENT_DATE - $2::int)
		   AND p.play_date <= CURRENT_DATE
		 ORDER BY p.play_date DESC, p.mode`, userID, days)
	return rows, err
}

// PuzzleForUser fetches one puzzle plus this player's progress on it.
func (r *Repo) PuzzleForUser(ctx context.Context, puzzleID int64, userID int) (*PuzzleRow, error) {
	var row PuzzleRow
	err := pgxscan.Get(ctx, r.pool, &row, `
		SELECT `+puzzleCols+`
		  FROM game_puzzles p
		  LEFT JOIN anime a ON a.id = p.anime_id
		  LEFT JOIN game_themes th ON th.id = p.theme_id
		  LEFT JOIN game_attempts att
		         ON att.puzzle_id = p.id AND att.user_id = $2
		 WHERE p.id = $1`, puzzleID, userID)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// SaveAttempt writes the player's progress for one puzzle.
func (r *Repo) SaveAttempt(ctx context.Context, puzzleID int64, userID int,
	guesses []int, solved, gaveUp bool, xp, gold int) error {

	blob, err := json.Marshal(guesses)
	if err != nil {
		return err
	}
	var solvedAt any
	if solved {
		solvedAt = time.Now()
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO game_attempts
		       (puzzle_id, user_id, guesses, solved_at, gave_up, awarded_xp, awarded_gold, updated_at, used_hint)
		VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, now(), false)
		ON CONFLICT (puzzle_id, user_id) DO UPDATE
		   SET guesses      = EXCLUDED.guesses,
		       solved_at    = COALESCE(game_attempts.solved_at, EXCLUDED.solved_at),
		       gave_up      = game_attempts.gave_up OR EXCLUDED.gave_up,
		       awarded_xp   = GREATEST(game_attempts.awarded_xp,   EXCLUDED.awarded_xp),
		       awarded_gold = GREATEST(game_attempts.awarded_gold, EXCLUDED.awarded_gold),
		       used_hint    = game_attempts.used_hint OR EXCLUDED.used_hint,
		       updated_at   = now()`,
		puzzleID, userID, string(blob), solvedAt, gaveUp, xp, gold)
	return err
}

// EnsurePuzzles fills in any missing day for the given modes, back across the
// window, and returns how many it created.
//
// Candidates are drawn from scored, poster-bearing titles rather than the whole
// catalogue: a uniform pick out of 2,149 rows lands on something nobody has
// heard of most days, which is not a puzzle, it is a coin flip. Recently used
// answers are excluded so the same series does not come round twice in a season.
func (r *Repo) EnsurePuzzles(ctx context.Context, modes []string, days int) (int, error) {
	// One cheap count before the loop. Every board load ran modes*days -- 28 --
	// insert statements, and after the first visit of a day all 28 are no-ops
	// that still cost a round trip each: measured at ~5ms apiece, most of the
	// board's server time was spent proving there was nothing to do. The loop
	// below is still the only thing that creates a puzzle, so a partially
	// filled window (a new mode, a gap after a purge) falls straight through
	// to it and is completed exactly as before.
	var have int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM game_puzzles
		 WHERE play_date > CURRENT_DATE - $1::int
		   AND play_date <= CURRENT_DATE
		   AND mode = ANY($2)`, days, modes).Scan(&have); err == nil {
		if have >= len(modes)*days {
			return 0, nil
		}
	}

	made := 0
	for _, mode := range modes {
		for back := 0; back < days; back++ {
			if mode == "character" {
				// DISTINCT ON keeps one person per series where it can, so the
				// "which show" half of the round is not five guesses at the same
				// two titles. Ordered by favorites so they are nameable.
				// One per series first, then seconds, then thirds -- so the
				// "which show" half is not five guesses at the same two titles
				// while the pool is small, and spreads naturally as it grows.
				t, terr := r.pool.Exec(ctx, `
					WITH ranked AS (
					  SELECT mal_char_id,
					         row_number() OVER (PARTITION BY anime_id ORDER BY random()) AS rn
					    FROM game_characters
					)
					INSERT INTO game_puzzles (mode, play_date, char_ids)
					SELECT $1, (CURRENT_DATE - $2::int),
					       (SELECT array_agg(mal_char_id)
					          FROM (SELECT mal_char_id FROM ranked
					                 ORDER BY rn, random() LIMIT $3) pick)
					 WHERE (SELECT count(*) FROM game_characters) >= $3
					ON CONFLICT (mode, play_date) DO NOTHING`, mode, back, CharSlots)
				if terr != nil {
					return made, fmt.Errorf("ensure character -%d: %w", back, terr)
				}
				made += int(t.RowsAffected())
				continue
			}
			if mode == "theme" {
				// Candidates are whatever the harvest has reached. Picking a
				// specific theme here rather than at read time keeps the day
				// stable if the harvest later adds more for that series.
				t, terr := r.pool.Exec(ctx, `
					INSERT INTO game_puzzles (mode, play_date, anime_id, theme_id)
					SELECT $1, (CURRENT_DATE - $2::int), th.anime_id, th.id
					  FROM game_themes th
					 WHERE NOT EXISTS (
					         SELECT 1 FROM game_puzzles gp
					          WHERE gp.theme_id = th.id
					            AND gp.play_date > CURRENT_DATE - 120)
					 ORDER BY random()
					 LIMIT 1
					ON CONFLICT (mode, play_date) DO NOTHING`, mode, back)
				if terr != nil {
					return made, fmt.Errorf("ensure theme -%d: %w", back, terr)
				}
				made += int(t.RowsAffected())
				continue
			}
			tag, err := r.pool.Exec(ctx, `
				INSERT INTO game_puzzles (mode, play_date, anime_id)
				SELECT $1, (CURRENT_DATE - $2::int), a.id
				  FROM anime a
				 WHERE a.image_url IS NOT NULL
				   AND a.score IS NOT NULL
				   AND a.id IN (SELECT id FROM anime
				                 WHERE score IS NOT NULL AND image_url IS NOT NULL
				                 ORDER BY score DESC NULLS LAST
				                 LIMIT 700)
				   AND NOT EXISTS (
				         SELECT 1 FROM game_puzzles gp
				          WHERE gp.anime_id = a.id
				            AND gp.play_date > CURRENT_DATE - 120)
				 ORDER BY random()
				 LIMIT 1
				ON CONFLICT (mode, play_date) DO NOTHING`, mode, back)
			if err != nil {
				return made, fmt.Errorf("ensure %s -%d: %w", mode, back, err)
			}
			made += int(tag.RowsAffected())
		}
	}
	return made, nil
}

// GrantBadge records a badge once. Returns true when it was newly earned.
func (r *Repo) GrantBadge(ctx context.Context, userID int, code string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO game_badges (user_id, code) VALUES ($1, $2)
		ON CONFLICT (user_id, code) DO NOTHING`, userID, code)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// Badge is one earned badge.
type Badge struct {
	Code     string    `db:"code"      json:"code"`
	EarnedAt time.Time `db:"earned_at" json:"earnedAt"`
}

func (r *Repo) Badges(ctx context.Context, userID int) ([]Badge, error) {
	out := []Badge{}
	err := pgxscan.Select(ctx, r.pool, &out, `
		SELECT code, earned_at FROM game_badges
		 WHERE user_id = $1 ORDER BY earned_at DESC`, userID)
	return out, err
}

// SolvedInWindow counts how many puzzles of a mode the player solved inside the
// window, and how many existed. Used for the weekly badge.
func (r *Repo) SolvedInWindow(ctx context.Context, userID int, mode string, days int) (solved, total int, err error) {
	row := r.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE att.solved_at IS NOT NULL), count(*)
		  FROM game_puzzles p
		  LEFT JOIN game_attempts att ON att.puzzle_id = p.id AND att.user_id = $1
		 WHERE p.mode = $2
		   AND p.play_date > (CURRENT_DATE - $3::int)
		   AND p.play_date <= CURRENT_DATE`, userID, mode, days)
	err = row.Scan(&solved, &total)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	return solved, total, err
}

// SetHintUsed marks the synopsis as taken for this attempt, creating the row if
// the player asks before their first guess is recorded.
func (r *Repo) SetHintUsed(ctx context.Context, puzzleID int64, userID int) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO game_attempts (puzzle_id, user_id, used_hint)
		VALUES ($1, $2, true)
		ON CONFLICT (puzzle_id, user_id) DO UPDATE SET used_hint = true`, puzzleID, userID)
	return err
}

// TotalSolved counts every puzzle this player has ever solved.
func (r *Repo) TotalSolved(ctx context.Context, userID int) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM game_attempts WHERE user_id = $1 AND solved_at IS NOT NULL`,
		userID).Scan(&n)
	return n, err
}

// SolvedToday reports progress across every mode playable today.
func (r *Repo) SolvedToday(ctx context.Context, userID int) (solved, total int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE att.solved_at IS NOT NULL), count(*)
		  FROM game_puzzles p
		  LEFT JOIN game_attempts att ON att.puzzle_id = p.id AND att.user_id = $1
		 WHERE p.play_date = CURRENT_DATE`, userID).Scan(&solved, &total)
	return
}

// CardRow is the slice of an anime the comparison grid needs.
type CardRow struct {
	ID       int      `db:"id"`
	Title    string   `db:"title"`
	TitleEn  *string  `db:"title_english"`
	TitleRo  *string  `db:"title_romanian"`
	Image    *string  `db:"image_url"`
	Year     *int     `db:"year"`
	Season   *string  `db:"season"`
	Type     *string  `db:"type"`
	Episodes *int     `db:"episodes"`
	Score    *float64 `db:"score"`
	Studios  []string `db:"studios"`
	Genres   []string `db:"genres"`
}

const cardCols = `id, title, title_english, title_romanian, image_url, year, season, type, episodes, score, studios, genres`

// normExpr strips everything but letters and digits so a typed guess matches a
// stored title regardless of punctuation, case or spacing. The catalogue is
// ~2k rows, so the sequential scan this forces is cheaper than maintaining an
// index for a query that runs a handful of times per player per day.
const normExpr = `lower(regexp_replace(%s, '[^a-zA-Z0-9]', '', 'g'))`

// CardByTitle resolves a typed guess to a series, matching any of the three
// spellings the catalogue stores.
func (r *Repo) CardByTitle(ctx context.Context, guess string) (*CardRow, error) {
	var c CardRow
	err := pgxscan.Get(ctx, r.pool, &c, `
		SELECT `+cardCols+` FROM anime
		 WHERE lower(regexp_replace(title,           '[^a-zA-Z0-9]', '', 'g')) = $1
		    OR lower(regexp_replace(coalesce(title_english,''),  '[^a-zA-Z0-9]', '', 'g')) = $1
		    OR lower(regexp_replace(coalesce(title_romanian,''), '[^a-zA-Z0-9]', '', 'g')) = $1
		 LIMIT 1`, guess)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CardByID fetches one series for the grid.
func (r *Repo) CardByID(ctx context.Context, id int) (*CardRow, error) {
	var c CardRow
	err := pgxscan.Get(ctx, r.pool, &c, `SELECT `+cardCols+` FROM anime WHERE id = $1`, id)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Cards fetches many, for replaying a player's guesses into the grid.
func (r *Repo) Cards(ctx context.Context, ids []int) (map[int]CardRow, error) {
	out := map[int]CardRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows := []CardRow{}
	if err := pgxscan.Select(ctx, r.pool, &rows,
		`SELECT `+cardCols+` FROM anime WHERE id = ANY($1)`, ids); err != nil {
		return nil, err
	}
	for _, c := range rows {
		out[c.ID] = c
	}
	return out, nil
}


// SolvedWithGuesses counts solves that used at most n guesses -- the skill
// badges, as opposed to the persistence ones.
func (r *Repo) SolvedWithGuesses(ctx context.Context, userID, maxGuesses int) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM game_attempts
		 WHERE user_id = $1 AND solved_at IS NOT NULL
		   AND jsonb_array_length(guesses) <= $2`, userID, maxGuesses).Scan(&n)
	return n, err
}

// HintFreeWindow reports solves and hint-free solves for a mode in the window,
// for the "no help at all" badge.
func (r *Repo) HintFreeWindow(ctx context.Context, userID int, mode string, days int) (clean, total int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE att.solved_at IS NOT NULL AND NOT att.used_hint),
		       count(*)
		  FROM game_puzzles p
		  LEFT JOIN game_attempts att ON att.puzzle_id = p.id AND att.user_id = $1
		 WHERE p.mode = $2
		   AND p.play_date > (CURRENT_DATE - $3::int)
		   AND p.play_date <= CURRENT_DATE`, userID, mode, days).Scan(&clean, &total)
	return
}

// SolvedWeekAllModes is the whole board for the window: every puzzle, every mode.
func (r *Repo) SolvedWeekAllModes(ctx context.Context, userID, days int) (solved, total int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE att.solved_at IS NOT NULL), count(*)
		  FROM game_puzzles p
		  LEFT JOIN game_attempts att ON att.puzzle_id = p.id AND att.user_id = $1
		 WHERE p.play_date > (CURRENT_DATE - $2::int)
		   AND p.play_date <= CURRENT_DATE`, userID, days).Scan(&solved, &total)
	return
}

// ChestState is what the arcade needs to draw the chest button.
type ChestState struct {
	Streak   int       `db:"streak"    json:"streak"`
	CanClaim bool      `db:"can_claim" json:"canClaim"`
	NextAt   time.Time `db:"next_at"   json:"nextAt"`
	NextGold int       `json:"nextGold"`
}

// Chest reports whether today's chest is still unopened, and the streak that
// would apply. A player who has never claimed gets streak 0 and canClaim true.
func (r *Repo) Chest(ctx context.Context, userID int) (*ChestState, error) {
	var st ChestState
	err := pgxscan.Get(ctx, r.pool, &st, `
		SELECT COALESCE(c.streak, 0) AS streak,
		       (c.last_claim IS NULL OR c.last_claim < CURRENT_DATE) AS can_claim,
		       (CURRENT_DATE + 1)::timestamptz AS next_at
		  FROM (SELECT 1) x
		  LEFT JOIN game_chests c ON c.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// ClaimChest opens today's chest and returns the new streak, or ErrExists when
// it has already been opened today.
//
// The streak continues only when the previous claim was yesterday; any longer
// gap starts again at 1. Done in one statement so two rapid clicks cannot both
// win -- the WHERE clause is the lock.
func (r *Repo) ClaimChest(ctx context.Context, userID int) (int, error) {
	var streak int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO game_chests (user_id, last_claim, streak)
		VALUES ($1, CURRENT_DATE, 1)
		ON CONFLICT (user_id) DO UPDATE
		   SET last_claim = CURRENT_DATE,
		       streak = CASE
		                  WHEN game_chests.last_claim = CURRENT_DATE - 1
		                    THEN game_chests.streak + 1
		                  ELSE 1
		                END
		 WHERE game_chests.last_claim < CURRENT_DATE
		RETURNING streak`, userID).Scan(&streak)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrExists
	}
	if err != nil {
		return 0, err
	}
	return streak, nil
}

// CharRow is one portrait in a character round.
type CharRow struct {
	MalCharID int    `db:"mal_char_id" json:"id"`
	AnimeID   int    `db:"anime_id"    json:"-"`
	Name      string `db:"name"        json:"name"`
	ImageURL  string `db:"image_url"   json:"image"`
	Favorites int    `db:"favorites"   json:"-"`
}

// CharsByIDs fetches a round's portraits, preserving no particular order --
// the caller re-orders by the stored id list so the layout is stable per day.
func (r *Repo) CharsByIDs(ctx context.Context, ids []int) (map[int]CharRow, error) {
	out := map[int]CharRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows := []CharRow{}
	if err := pgxscan.Select(ctx, r.pool, &rows,
		`SELECT mal_char_id, anime_id, name, image_url, favorites
		   FROM game_characters WHERE mal_char_id = ANY($1)`, ids); err != nil {
		return nil, err
	}
	for _, c := range rows {
		out[c.MalCharID] = c
	}
	return out, nil
}

// SearchChars powers the name autocomplete. Ordered by favorites so the
// recognisable people surface first.
func (r *Repo) SearchChars(ctx context.Context, q string, limit int) ([]CharRow, error) {
	out := []CharRow{}
	err := pgxscan.Select(ctx, r.pool, &out, `
		SELECT mal_char_id, anime_id, name, image_url, favorites
		  FROM game_characters
		 WHERE name ILIKE '%' || $1 || '%'
		 ORDER BY favorites DESC
		 LIMIT $2`, q, limit)
	return out, err
}

// SaveCharAnswers records the single submission for a character round.
func (r *Repo) SaveCharAnswers(ctx context.Context, puzzleID int64, userID int,
	answers string, xp, gold int) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO game_attempts
		       (puzzle_id, user_id, char_answers, solved_at, awarded_xp, awarded_gold, updated_at)
		VALUES ($1, $2, $3::jsonb, now(), $4, $5, now())
		ON CONFLICT (puzzle_id, user_id) DO UPDATE
		   SET char_answers = EXCLUDED.char_answers,
		       solved_at    = COALESCE(game_attempts.solved_at, EXCLUDED.solved_at),
		       awarded_xp   = GREATEST(game_attempts.awarded_xp,   EXCLUDED.awarded_xp),
		       awarded_gold = GREATEST(game_attempts.awarded_gold, EXCLUDED.awarded_gold),
		       updated_at   = now()`, puzzleID, userID, answers, xp, gold)
	return err
}

// FamilyOf returns every anime in the same franchise as the given one, walking
// sequels, prequels, side stories, spin-offs and alternatives transitively.
//
// This is what lets "Re:Zero" count for a character who technically belongs to
// the 4th Season row. A player naming the franchise has demonstrably recognised
// the character; making them pick the exact season tests catalogue trivia
// rather than knowledge of the show. Depth is capped because a long-running
// franchise can otherwise chain a very long way.
func (r *Repo) FamilyOf(ctx context.Context, animeID int) (map[int]bool, error) {
	ids := []int{}
	// Walked in MAL-id space, not local-id space, and in both directions.
	//
	// The edges name a related MAL id whether or not we hold that entry, and
	// stepping only to rows we hold breaks the chain wherever one is missing.
	// The Slime franchise fragmented exactly that way: S1 and S2 both point at
	// 54565, S2 and S3 both at 41487, and we hold neither -- so Season 4 could
	// not reach Season 1, and naming Season 1 for a Season 4 character scored
	// as a different series. Two entries pointing at the same absent id are
	// provably related, so absent ids are kept as nodes and simply have no
	// edges of their own.
	//
	// Both directions because a relation is only guaranteed to be recorded on
	// one side of the pair.
	//
	// Depth 8 rather than 5: the walk now passes through the gaps, which costs
	// hops. Measured over the top 60 series this averages 4.1 entries and peaks
	// at 16 (Bleach, which really does have that many arcs and films), so it
	// does not run away into unrelated shows.
	err := pgxscan.Select(ctx, r.pool, &ids, `
		WITH RECURSIVE fam(mal_id, depth) AS (
		  SELECT a.mal_id, 0 FROM anime a WHERE a.id = $1::int
		  UNION
		  SELECT nxt.mal_id, f.depth + 1
		    FROM fam f
		    JOIN LATERAL (
		      SELECT r.related_mal_id AS mal_id
		        FROM anime a
		        JOIN anime_relations r ON r.anime_id = a.id
		       WHERE a.mal_id = f.mal_id
		         AND r.relation IN ('SEQUEL','PREQUEL','SIDE_STORY','PARENT',
		                            'ALTERNATIVE','SUMMARY','SPIN_OFF')
		      UNION
		      SELECT a.mal_id
		        FROM anime_relations r
		        JOIN anime a ON a.id = r.anime_id
		       WHERE r.related_mal_id = f.mal_id
		         AND r.relation IN ('SEQUEL','PREQUEL','SIDE_STORY','PARENT',
		                            'ALTERNATIVE','SUMMARY','SPIN_OFF')
		    ) nxt ON true
		   WHERE f.depth < 8
		)
		SELECT DISTINCT a.id FROM fam JOIN anime a ON a.mal_id = fam.mal_id`, animeID)
	if err != nil {
		return nil, err
	}
	out := make(map[int]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// LeaderRow is one line of the arcade leaderboard.
type LeaderRow struct {
	UserID    int     `db:"user_id"    json:"-"`
	Username  string  `db:"username"   json:"username"`
	AvatarURL *string `db:"avatar_url" json:"avatarUrl,omitempty"`
	Faction   *string `db:"faction"    json:"faction,omitempty"`
	Xp        int64   `db:"xp"         json:"xp"`
	Gold      int64   `db:"gold"       json:"gold"`
}

// Leaderboard ranks players by the experience on their ACTIVE faction, which is
// the same number their own profile card shows -- ranking on a lifetime total
// across factions would read as a different game to the one being played, and
// would disagree with the level printed next to their name.
//
// Gold breaks a tie because two players on the same XP have genuinely played the
// same amount; gold then says who kept at the daily chest. Username last so the
// order is stable between requests rather than shuffling on every load.
func (r *Repo) Leaderboard(ctx context.Context, limit int) ([]LeaderRow, error) {
	rows := []LeaderRow{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT u.id AS user_id, u.username, u.avatar_url,
		       p.faction, COALESCE(fx.xp, 0) AS xp, p.gold
		  FROM game_profiles p
		  JOIN users u ON u.id = p.user_id
		  LEFT JOIN game_faction_xp fx
		         ON fx.user_id = p.user_id AND fx.faction = p.faction
		 ORDER BY COALESCE(fx.xp, 0) DESC, p.gold DESC, u.username
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
