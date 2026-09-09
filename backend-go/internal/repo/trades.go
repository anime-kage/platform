package repo

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
)

// SpareCard is one card a player can afford to give away.
type SpareCard struct {
	MalCharID int    `db:"mal_char_id"`
	Name      string `db:"name"`
	Favorites int    `db:"favorites"`
	Copies    int    `db:"copies"`
}

// Spares lists the cards a player holds more than one of, which are the only
// ones tradeable: nobody may trade away their only copy.
func (r *Repo) Spares(ctx context.Context, userID int, like string, limit int) ([]SpareCard, error) {
	rows := []SpareCard{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT c.mal_char_id, ch.name, ch.favorites, c.copies
		  FROM game_cards c JOIN game_characters ch ON ch.mal_char_id = c.mal_char_id
		 WHERE c.user_id = $1 AND c.copies > 1
		   AND ($2 = '' OR ch.name ILIKE '%' || $2 || '%')
		 ORDER BY ch.favorites DESC LIMIT $3`, userID, like, limit)
	return rows, err
}

// OwnedFor lists a player's cards for the other side of a trade to pick from.
func (r *Repo) OwnedFor(ctx context.Context, userID int, like string, limit int) ([]SpareCard, error) {
	rows := []SpareCard{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT c.mal_char_id, ch.name, ch.favorites, c.copies
		  FROM game_cards c JOIN game_characters ch ON ch.mal_char_id = c.mal_char_id
		 WHERE c.user_id = $1 AND c.copies > 1
		   AND ($2 = '' OR ch.name ILIKE '%' || $2 || '%')
		 ORDER BY ch.favorites DESC LIMIT $3`, userID, like, limit)
	return rows, err
}

// Trade is an offer.
type Trade struct {
	ID         int    `db:"id"`
	FromUserID int    `db:"from_user_id"`
	ToUserID   int    `db:"to_user_id"`
	GiveCharID int    `db:"give_char_id"`
	WantCharID int    `db:"want_char_id"`
	Gold       int    `db:"gold"`
	FromPays   bool   `db:"from_pays"`
	FromName   string `db:"from_name"`
	ToName     string `db:"to_name"`
	GiveName   string `db:"give_name"`
	WantName   string `db:"want_name"`
}

const tradeCols = `t.id, t.from_user_id, t.to_user_id, t.give_char_id, t.want_char_id,
	t.gold, t.from_pays,
	uf.username AS from_name, ut.username AS to_name,
	cg.name AS give_name, cw.name AS want_name`

const tradeFrom = ` FROM game_trades t
	JOIN users uf ON uf.id = t.from_user_id
	JOIN users ut ON ut.id = t.to_user_id
	JOIN game_characters cg ON cg.mal_char_id = t.give_char_id
	JOIN game_characters cw ON cw.mal_char_id = t.want_char_id`

// CreateTrade records an offer.
func (r *Repo) CreateTrade(ctx context.Context, from, to, give, want, gold int,
	fromPays bool, ttl time.Duration) (*Trade, error) {

	var t Trade
	err := pgxscan.Get(ctx, r.pool, &t, `
		WITH made AS (
		  INSERT INTO game_trades
		         (from_user_id, to_user_id, give_char_id, want_char_id, gold, from_pays, expires_at)
		  VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)
		  RETURNING *
		)
		SELECT `+tradeCols+`
		  FROM made t
		  JOIN users uf ON uf.id = t.from_user_id
		  JOIN users ut ON ut.id = t.to_user_id
		  JOIN game_characters cg ON cg.mal_char_id = t.give_char_id
		  JOIN game_characters cw ON cw.mal_char_id = t.want_char_id`,
		from, to, give, want, gold, fromPays, ttl.String())
	return &t, err
}

// OpenTrade loads an offer that is still answerable.
func (r *Repo) OpenTrade(ctx context.Context, id int) (*Trade, error) {
	var t Trade
	err := pgxscan.Get(ctx, r.pool, &t, `SELECT `+tradeCols+tradeFrom+`
		 WHERE t.id = $1 AND t.accepted_at IS NULL AND t.declined_at IS NULL
		   AND t.expires_at > now()`, id)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	return &t, err
}

// DeclineTrade closes an offer without moving anything.
func (r *Repo) DeclineTrade(ctx context.Context, id int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE game_trades SET declined_at = now()
		 WHERE id = $1 AND accepted_at IS NULL AND declined_at IS NULL`, id)
	return err
}

// AcceptTrade performs the swap: both spares move, gold moves, in one
// transaction.
//
// Everything is rechecked here rather than trusted from the offer. Between the
// offer and the answer either side may have melted the spare, traded it to
// somebody else, or spent the gold, and an offer that was valid when written is
// not evidence that it still is.
func (r *Repo) AcceptTrade(ctx context.Context, id int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var t Trade
	err = pgxscan.Get(ctx, tx, &t, `SELECT `+tradeCols+tradeFrom+`
		 WHERE t.id = $1 AND t.accepted_at IS NULL AND t.declined_at IS NULL
		   AND t.expires_at > now()
		 FOR UPDATE OF t`, id)
	if pgxscan.NotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// Each side must still hold a spare of what they are giving.
	for _, side := range []struct {
		user, char int
	}{{t.FromUserID, t.GiveCharID}, {t.ToUserID, t.WantCharID}} {
		tag, derr := tx.Exec(ctx, `
			UPDATE game_cards SET copies = copies - 1
			 WHERE user_id = $1 AND mal_char_id = $2 AND copies > 1`, side.user, side.char)
		if derr != nil {
			return derr
		}
		if tag.RowsAffected() == 0 {
			return ErrExists // no longer has a spare
		}
	}

	// And each receives the other's.
	for _, side := range []struct {
		user, char int
	}{{t.FromUserID, t.WantCharID}, {t.ToUserID, t.GiveCharID}} {
		if _, ierr := tx.Exec(ctx, `
			INSERT INTO game_cards (user_id, mal_char_id) VALUES ($1, $2)
			ON CONFLICT (user_id, mal_char_id) DO UPDATE
			   SET copies = game_cards.copies + 1`, side.user, side.char); ierr != nil {
			return ierr
		}
	}

	if t.Gold > 0 {
		payer, payee := t.ToUserID, t.FromUserID
		if t.FromPays {
			payer, payee = t.FromUserID, t.ToUserID
		}
		tag, gerr := tx.Exec(ctx, `
			UPDATE game_profiles SET gold = gold - $2
			 WHERE user_id = $1 AND gold >= $2`, payer, t.Gold)
		if gerr != nil {
			return gerr
		}
		if tag.RowsAffected() == 0 {
			return ErrConflictGold
		}
		if _, gerr = tx.Exec(ctx, `
			UPDATE game_profiles SET gold = gold + $2 WHERE user_id = $1`, payee, t.Gold); gerr != nil {
			return gerr
		}
		// Both halves in the ledger, so a balance is always explainable.
		if _, gerr = tx.Exec(ctx, `
			INSERT INTO game_ledger (user_id, xp_delta, gold_delta, reason, ref)
			VALUES ($1, 0, $3, 'trade', $5), ($2, 0, $4, 'trade', $5)`,
			payer, payee, -t.Gold, t.Gold, strconv.Itoa(id)); gerr != nil {
			return gerr
		}
	}

	if _, err = tx.Exec(ctx,
		`UPDATE game_trades SET accepted_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ErrConflictGold is the accepting side not having the gold any more.
var ErrConflictGold = errors.New("not enough gold for the trade")

// CardDetail is one card a player holds, used to price a trade. ErrNotFound
// when they do not hold a spare of it.
func (r *Repo) CardDetail(ctx context.Context, userID, malCharID int) (*SpareCard, error) {
	var c SpareCard
	err := pgxscan.Get(ctx, r.pool, &c, `
		SELECT c.mal_char_id, ch.name, ch.favorites, c.copies
		  FROM game_cards c JOIN game_characters ch ON ch.mal_char_id = c.mal_char_id
		 WHERE c.user_id = $1 AND c.mal_char_id = $2 AND c.copies > 1`, userID, malCharID)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	return &c, err
}

// GoldOf is the player's balance, for checking an offer is payable before it is
// put to the other side.
func (r *Repo) GoldOf(ctx context.Context, userID int) (int, error) {
	var g int
	err := r.pool.QueryRow(ctx,
		`SELECT gold FROM game_profiles WHERE user_id = $1`, userID).Scan(&g)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return g, err
}
