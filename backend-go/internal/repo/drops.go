package repo

import (
	"context"
	"errors"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
)

// Drop is a character waiting to be named.
type Drop struct {
	ID        int     `db:"id"`
	MalCharID int     `db:"mal_char_id"`
	ChannelID string  `db:"channel_id"`
	MessageID *string `db:"message_id"`
	Name      string  `db:"name"`
	ImageURL  string  `db:"image_url"`
	Favorites int     `db:"favorites"`
	Series    *string `db:"series"`
	DropDate  time.Time `db:"drop_date"`
	Guesses   int     `db:"guesses"`
}

const dropCols = `d.id, d.mal_char_id, d.channel_id, d.message_id, d.drop_date, d.guesses,
	c.name, c.image_url, c.favorites, coalesce(a.title_english, a.title) AS series`

const dropFrom = ` FROM game_drops d
	JOIN game_characters c ON c.mal_char_id = d.mal_char_id
	LEFT JOIN anime a ON a.id = c.anime_id`

// LiveDrop is the channel's unclaimed drop, if there is one.
func (r *Repo) LiveDrop(ctx context.Context, channelID string) (*Drop, error) {
	var d Drop
	err := pgxscan.Get(ctx, r.pool, &d, `SELECT `+dropCols+dropFrom+`
		 WHERE d.channel_id = $1 AND d.claimed_by IS NULL AND d.expired_at IS NULL`, channelID)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	return &d, err
}

// CreateDrop picks a character nobody has been asked to name recently and puts
// it up. ErrExists when one is already live.
//
// Weighted towards the well known: a drop whose answer is a background extra
// with 40 favourites is not a guessing game, it is a lookup.
func (r *Repo) CreateDrop(ctx context.Context, channelID string, recentDays int) (*Drop, error) {
	var d Drop
	err := pgxscan.Get(ctx, r.pool, &d, `
		WITH pick AS (
		  SELECT c.mal_char_id FROM game_characters c
		   WHERE c.favorites >= 500
		     AND NOT EXISTS (
		           SELECT 1 FROM game_drops gd
		            WHERE gd.mal_char_id = c.mal_char_id
		              AND gd.dropped_at > now() - make_interval(days => $2))
		   ORDER BY random() LIMIT 1
		), made AS (
		  INSERT INTO game_drops (mal_char_id, channel_id)
		  SELECT mal_char_id, $1 FROM pick
		  RETURNING id, mal_char_id, channel_id, message_id, drop_date, guesses
		)
		SELECT made.id, made.mal_char_id, made.channel_id, made.message_id,
		       made.drop_date, made.guesses,
		       c.name, c.image_url, c.favorites, coalesce(a.title_english, a.title) AS series
		  FROM made
		  JOIN game_characters c ON c.mal_char_id = made.mal_char_id
		  LEFT JOIN anime a ON a.id = c.anime_id`, channelID, recentDays)
	if IsUniqueViolation(err) {
		return nil, ErrExists
	}
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound // pool exhausted
	}
	return &d, err
}

// SetDropMessage remembers which message shows the drop.
func (r *Repo) SetDropMessage(ctx context.Context, id int, messageID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE game_drops SET message_id = $2 WHERE id = $1`, id, messageID)
	return err
}

// ClaimDrop awards the card to the first correct guess.
//
// The WHERE clause is the lock: two players naming it in the same instant, only
// one row updates, and the loser is told it has just gone.
func (r *Repo) ClaimDrop(ctx context.Context, dropID, userID int) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var charID int
	err = tx.QueryRow(ctx, `
		UPDATE game_drops SET claimed_by = $2, claimed_at = now()
		 WHERE id = $1 AND claimed_by IS NULL AND expired_at IS NULL
		RETURNING mal_char_id`, dropID, userID).Scan(&charID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrExists // somebody was faster
	}
	if err != nil {
		return 0, err
	}
	var copies int
	if err = tx.QueryRow(ctx, `
		INSERT INTO game_cards (user_id, mal_char_id) VALUES ($1, $2)
		ON CONFLICT (user_id, mal_char_id) DO UPDATE
		   SET copies = game_cards.copies + 1
		RETURNING copies`, userID, charID).Scan(&copies); err != nil {
		return 0, err
	}
	return copies, tx.Commit(ctx)
}

// CountGuess records a wrong attempt, for the drop's own counter.
func (r *Repo) CountGuess(ctx context.Context, dropID int) {
	_, _ = r.pool.Exec(ctx, `UPDATE game_drops SET guesses = guesses + 1 WHERE id = $1`, dropID)
}

// ExpireStaleDrops retires anything left unclaimed from a previous day and
// returns what it closed, so the caller can go and say so in the channel.
//
// Returning the rows rather than a count is the point: expiring a database row
// leaves the Discord message standing, still inviting people to guess something
// that ended hours ago.
func (r *Repo) ExpireStaleDrops(ctx context.Context) ([]Drop, error) {
	rows := []Drop{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		WITH gone AS (
		  UPDATE game_drops SET expired_at = now()
		   WHERE claimed_by IS NULL AND expired_at IS NULL AND drop_date < CURRENT_DATE
		  RETURNING id, mal_char_id, channel_id, message_id, drop_date, guesses
		)
		SELECT g.id, g.mal_char_id, g.channel_id, g.message_id, g.drop_date, g.guesses,
		       c.name, c.image_url, c.favorites,
		       coalesce(a.title_english, a.title) AS series
		  FROM gone g
		  JOIN game_characters c ON c.mal_char_id = g.mal_char_id
		  LEFT JOIN anime a ON a.id = c.anime_id`)
	return rows, err
}

// DropsToday is how many have been put up since midnight, claimed or not.
func (r *Repo) DropsToday(ctx context.Context, channelID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM game_drops WHERE channel_id = $1 AND drop_date = CURRENT_DATE`,
		channelID).Scan(&n)
	return n, err
}

// TryGuess claims the player's guess slot, or reports when they may try again.
func (r *Repo) TryGuess(ctx context.Context, userID int, rest time.Duration) (time.Time, error) {
	var next time.Time
	err := r.pool.QueryRow(ctx, `
		INSERT INTO game_guessers (user_id, next_guess_at) VALUES ($1, now() + $2::interval)
		ON CONFLICT (user_id) DO UPDATE SET next_guess_at = now() + $2::interval
		 WHERE game_guessers.next_guess_at <= now()
		RETURNING next_guess_at`, userID, rest.String()).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		var when time.Time
		if e := r.pool.QueryRow(ctx,
			`SELECT next_guess_at FROM game_guessers WHERE user_id = $1`, userID).Scan(&when); e != nil {
			return time.Time{}, e
		}
		return when, ErrExists
	}
	return next, err
}
