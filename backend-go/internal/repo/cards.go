package repo

import (
	"context"
	"errors"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
)

// UserByDiscordID resolves a Discord account to a site account.
//
// The link is a column on users, backfilled from the invite each account was
// created with. A Discord user with no site account gets ErrNotFound, which the
// bot turns into "leagă-ți contul" rather than a silent failure.
func (r *Repo) UserByDiscordID(ctx context.Context, discordID string) (int, string, error) {
	var id int
	var name string
	err := r.pool.QueryRow(ctx,
		`SELECT id, username FROM users WHERE discord_id = $1`, discordID).Scan(&id, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return id, name, err
}

// ClaimDailyDraw takes today's draw, or reports ErrExists if it is already gone.
//
// One statement, so two fast /daily commands cannot both win: the WHERE clause
// on the update is the lock, exactly as the site's chest does it. The calendar
// day is the database's, which is UTC -- the same boundary the arcade rolls on,
// so a player never sees Discord and the site disagree about what today is.
func (r *Repo) ClaimDailyDraw(ctx context.Context, userID int) (int, error) {
	var streak int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO game_draws (user_id, last_draw, streak)
		VALUES ($1, CURRENT_DATE, 1)
		ON CONFLICT (user_id) DO UPDATE
		   SET last_draw = CURRENT_DATE,
		       -- The paid-draw counter is scoped to a day by last_draw, and this
		       -- statement moves last_draw. Without resetting it here, yesterday's
		       -- count was still "today's" the moment the free draw was claimed,
		       -- so the price never fell back to 60 and kept doubling for good.
		       extra_today = 0,
		       streak = CASE WHEN game_draws.last_draw = CURRENT_DATE - 1
		                     THEN game_draws.streak + 1 ELSE 1 END
		 WHERE game_draws.last_draw < CURRENT_DATE
		RETURNING streak`, userID).Scan(&streak)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrExists
	}
	return streak, err
}

// CardDraw is one character as the bot needs to render it.
type CardDraw struct {
	MalCharID int     `db:"mal_char_id"`
	Name      string  `db:"name"`
	ImageURL  string  `db:"image_url"`
	Favorites int     `db:"favorites"`
	Series    *string `db:"series"`
}

// RandomCharacterInRange picks one character inside a rarity band.
//
// maxFavs of 0 means "no ceiling", for the top tier. TABLESAMPLE is deliberately
// not used: the pool is under a thousand rows, so ORDER BY random() is a
// millisecond, and TABLESAMPLE's block sampling would bias towards whatever
// happens to share a page.
func (r *Repo) RandomCharacterInRange(ctx context.Context, minFavs, maxFavs int) (*CardDraw, error) {
	var c CardDraw
	err := pgxscan.Get(ctx, r.pool, &c, `
		SELECT ch.mal_char_id, ch.name, ch.image_url, ch.favorites,
		       coalesce(a.title_english, a.title) AS series
		  FROM game_characters ch
		  LEFT JOIN anime a ON a.id = ch.anime_id
		 WHERE ch.favorites >= $1
		   AND ($2 = 0 OR ch.favorites < $2)
		 ORDER BY random()
		 LIMIT 1`, minFavs, maxFavs)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	return &c, err
}

// AddCard files a drawn character, returning how many copies the player now
// holds. A return of 1 means it was new.
func (r *Repo) AddCard(ctx context.Context, userID, malCharID int) (int, error) {
	var copies int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO game_cards (user_id, mal_char_id) VALUES ($1, $2)
		ON CONFLICT (user_id, mal_char_id) DO UPDATE
		   SET copies = game_cards.copies + 1
		RETURNING copies`, userID, malCharID).Scan(&copies)
	return copies, err
}

// DropCard removes one copy, for converting a duplicate into XP. Refuses to go
// below one: a player converting a spare must keep the card itself.
func (r *Repo) DropCard(ctx context.Context, userID, malCharID int) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE game_cards SET copies = copies - 1
		 WHERE user_id = $1 AND mal_char_id = $2 AND copies > 1`, userID, malCharID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CollectionSize is the "x/914" line under a draw.
func (r *Repo) CollectionSize(ctx context.Context, userID int) (owned int, total int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM game_cards WHERE user_id = $1),
		       (SELECT count(*) FROM game_characters)`, userID).Scan(&owned, &total)
	return
}

// CharacterFavourites is how the bot re-derives a card's rarity from its id,
// so a button pressed hours later still pays the right amount.
func (r *Repo) CharacterFavourites(ctx context.Context, malCharID int) (int, error) {
	var favs int
	err := r.pool.QueryRow(ctx,
		`SELECT favorites FROM game_characters WHERE mal_char_id = $1`, malCharID).Scan(&favs)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return favs, err
}

// OwnedCard is one line of the collection browser.
type OwnedCard struct {
	MalCharID int     `db:"mal_char_id"`
	Name      string  `db:"name"`
	ImageURL  string  `db:"image_url"`
	Favorites int     `db:"favorites"`
	Copies    int     `db:"copies"`
	Series    *string `db:"series"`
}

// Collection is one page of a player's cards, rarest first.
//
// Ordered by favourites rather than by when they were drawn: the collection is
// something to show off, and the card people want at the top is the best one,
// not the newest. minFavs/maxFavs narrow it to a single rarity band when the
// player filters; 0/0 means everything.
func (r *Repo) Collection(ctx context.Context, userID, minFavs, maxFavs, limit, offset int) ([]OwnedCard, int, error) {
	rows := []OwnedCard{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT ch.mal_char_id, ch.name, ch.image_url, ch.favorites, c.copies,
		       coalesce(a.title_english, a.title) AS series
		  FROM game_cards c
		  JOIN game_characters ch ON ch.mal_char_id = c.mal_char_id
		  LEFT JOIN anime a ON a.id = ch.anime_id
		 WHERE c.user_id = $1
		   AND ch.favorites >= $2
		   AND ($3 = 0 OR ch.favorites < $3)
		 ORDER BY ch.favorites DESC, ch.name
		 LIMIT $4 OFFSET $5`, userID, minFavs, maxFavs, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	var total int
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FROM game_cards c
		  JOIN game_characters ch ON ch.mal_char_id = c.mal_char_id
		 WHERE c.user_id = $1 AND ch.favorites >= $2 AND ($3 = 0 OR ch.favorites < $3)`,
		userID, minFavs, maxFavs).Scan(&total)
	return rows, total, err
}

// ClaimHunt takes a treasure-hunt turn if the cooldown has expired, returning
// the time of the claim. ErrExists means it is still on cooldown, and the
// caller reports how long is left.
//
// Stored as a timestamp rather than a date because this is a rolling cooldown,
// not a calendar day: "every four hours" has to survive midnight.
func (r *Repo) ClaimHunt(ctx context.Context, userID int, cooldown time.Duration) (time.Time, error) {
	var last time.Time
	err := r.pool.QueryRow(ctx, `
		INSERT INTO game_hunts (user_id, last_hunt) VALUES ($1, now())
		ON CONFLICT (user_id) DO UPDATE SET last_hunt = now()
		 WHERE game_hunts.last_hunt < now() - $2::interval
		RETURNING last_hunt`, userID, cooldown.String()).Scan(&last)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already on cooldown -- report when it frees up.
		var when time.Time
		if e := r.pool.QueryRow(ctx,
			`SELECT last_hunt + $2::interval FROM game_hunts WHERE user_id = $1`,
			userID, cooldown.String()).Scan(&when); e != nil {
			return time.Time{}, e
		}
		return when, ErrExists
	}
	return last, err
}

// DiscordProfile is everything /profil shows, gathered in one round trip.
//
// One query rather than five: the bot has three seconds to answer an
// interaction before Discord gives up on it, and five sequential round trips to
// a database on another host is a real share of that budget.
type DiscordProfile struct {
	Username  string   `db:"username"`
	Faction   *string  `db:"faction"`
	Gold      int64    `db:"gold"`
	Xp        int64    `db:"xp"`
	PendingXp int64    `db:"pending_xp"`
	Badges    []string `db:"badges"`
	Cards     int      `db:"cards"`
	CardTotal int      `db:"card_total"`
	Rank      int      `db:"rank"`
	Players   int      `db:"players"`
}

func (r *Repo) DiscordProfile(ctx context.Context, userID int) (*DiscordProfile, error) {
	var p DiscordProfile
	err := pgxscan.Get(ctx, r.pool, &p, `
		WITH me AS (
		  SELECT p.user_id, p.faction, p.gold, p.pending_xp,
		         COALESCE(fx.xp, 0) AS xp
		    FROM game_profiles p
		    LEFT JOIN game_faction_xp fx
		           ON fx.user_id = p.user_id AND fx.faction = p.faction
		   WHERE p.user_id = $1
		),
		-- Rank counts everyone strictly ahead, so ties share a position rather
		-- than being ordered arbitrarily by whatever the plan returns first.
		standing AS (
		  SELECT (SELECT count(*) + 1 FROM game_profiles p2
		            LEFT JOIN game_faction_xp f2
		                   ON f2.user_id = p2.user_id AND f2.faction = p2.faction
		           WHERE COALESCE(f2.xp, 0) > (SELECT xp FROM me)) AS rank,
		         (SELECT count(*) FROM game_profiles) AS players
		)
		SELECT u.username, me.faction, me.gold, me.xp, me.pending_xp,
		       COALESCE((SELECT array_agg(code ORDER BY earned_at)
		                   FROM game_badges WHERE user_id = $1), '{}') AS badges,
		       (SELECT count(*) FROM game_cards WHERE user_id = $1) AS cards,
		       (SELECT count(*) FROM game_characters) AS card_total,
		       standing.rank, standing.players
		  FROM me
		  JOIN users u ON u.id = me.user_id
		  CROSS JOIN standing`, userID)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	return &p, err
}

// SpendGold deducts if the player can afford it, and writes the ledger row that
// explains the balance. ErrExists means they cannot.
//
// The WHERE clause is the check: reading the balance and then deducting would
// let two rapid clicks both pass the read and overdraw. AwardXPGold is not used
// with a negative amount because it clamps at zero, which would happily "spend"
// gold nobody had.
func (r *Repo) SpendGold(ctx context.Context, userID, amount int, reason, ref string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var left int64
	err = tx.QueryRow(ctx, `
		UPDATE game_profiles SET gold = gold - $2
		 WHERE user_id = $1 AND gold >= $2
		RETURNING gold`, userID, amount).Scan(&left)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExists // not enough gold
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO game_ledger (user_id, xp_delta, gold_delta, reason, ref)
		VALUES ($1, 0, $2, $3, NULLIF($4, ''))`, userID, -amount, reason, ref); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ExtraDrawsToday is how many paid draws the player has already bought, zero if
// their last draw was on an earlier day.
func (r *Repo) ExtraDrawsToday(ctx context.Context, userID int) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT CASE WHEN last_draw = CURRENT_DATE THEN extra_today ELSE 0 END
		  FROM game_draws WHERE user_id = $1`, userID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// BuyExtraDraw records one paid draw. Returns ErrExists if the daily allowance
// is already gone, checked in the same statement that spends it.
func (r *Repo) BuyExtraDraw(ctx context.Context, userID, maxPerDay int) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE game_draws
		   SET extra_today = CASE WHEN last_draw = CURRENT_DATE THEN extra_today + 1 ELSE 1 END,
		       last_draw = CURRENT_DATE
		 WHERE user_id = $1
		   AND (last_draw < CURRENT_DATE OR extra_today < $2)`, userID, maxPerDay)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrExists
	}
	return nil
}

// UsernameOf is the site name behind a user id.
func (r *Repo) UsernameOf(ctx context.Context, userID int) (string, error) {
	var name string
	err := r.pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}
