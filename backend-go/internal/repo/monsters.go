package repo

import (
	"context"
	"errors"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
)

// Monster is a live spawn.
type Monster struct {
	ID        int        `db:"id"`
	Code      string     `db:"code"`
	Name      string     `db:"name"`
	ChannelID string     `db:"channel_id"`
	MessageID *string    `db:"message_id"`
	HPTotal   int        `db:"hp_total"`
	HPLeft    int        `db:"hp_left"`
	XPPot     int        `db:"xp_pot"`
	GoldPot   int        `db:"gold_pot"`
	ExpiresAt time.Time  `db:"expires_at"`
	KilledAt  *time.Time `db:"killed_at"`
}

const monsterCols = `id, code, name, channel_id, message_id, hp_total, hp_left,
	xp_pot, gold_pot, expires_at, killed_at`

// SpawnMonster creates one, or returns ErrExists if the channel already has a
// live monster. The unique partial index is what enforces that, so two
// simultaneous spawns cannot both win.
//
// A monster lives until the end of the day, not for its tier's window.
//
// The tier window (20/35/60 minutes) was sized as "how long a fight should take
// if the channel shows up", which is a different question from "how long should
// people have to notice". A Kurama spawned at 04:51 expired at 05:51 having
// taken 4% damage, because the server was asleep -- the fight was never lost,
// it was never seen. Aligning expiry with the daily reset means a spawn is
// there when people wake up, and the tier window survives as a floor via
// GREATEST so something spawned at 23:50 still gets its full time.
func (r *Repo) SpawnMonster(ctx context.Context, code, name, channelID string,
	hp, xp, gold int, ttl time.Duration) (*Monster, error) {

	var m Monster
	err := pgxscan.Get(ctx, r.pool, &m, `
		INSERT INTO game_monsters (code, name, channel_id, hp_total, hp_left, xp_pot, gold_pot, expires_at)
		VALUES ($1, $2, $3, $4, $4, $5, $6,
		        GREATEST(date_trunc('day', now()) + interval '1 day', now() + $7::interval))
		RETURNING `+monsterCols, code, name, channelID, hp, xp, gold, ttl.String())
	if IsUniqueViolation(err) {
		return nil, ErrExists
	}
	return &m, err
}

// LiveMonster is the channel's current fight, if any.
func (r *Repo) LiveMonster(ctx context.Context, channelID string) (*Monster, error) {
	var m Monster
	err := pgxscan.Get(ctx, r.pool, &m, `
		SELECT `+monsterCols+` FROM game_monsters
		 WHERE channel_id = $1 AND killed_at IS NULL`, channelID)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	return &m, err
}

// SetMonsterMessage remembers which message shows the health bar.
func (r *Repo) SetMonsterMessage(ctx context.Context, id int, messageID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE game_monsters SET message_id = $2 WHERE id = $1`, id, messageID)
	return err
}

// HitMonster applies damage and returns the HP left plus whether this blow
// killed it.
//
// One statement for the subtraction so simultaneous attacks cannot both read
// the same HP and each think they left it standing. GREATEST clamps at zero:
// the last hit is usually an overkill and a negative bar would look broken.
func (r *Repo) HitMonster(ctx context.Context, monsterID, userID, damage int) (left int, killed bool, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		UPDATE game_monsters
		   SET hp_left = GREATEST(hp_left - $2, 0)
		 WHERE id = $1 AND killed_at IS NULL
		RETURNING hp_left`, monsterID, damage).Scan(&left)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, ErrNotFound // already dead, or expired
	}
	if err != nil {
		return 0, false, err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO game_monster_hits (monster_id, user_id, damage, hits)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (monster_id, user_id) DO UPDATE
		   SET damage = game_monster_hits.damage + $3,
		       hits = game_monster_hits.hits + 1`, monsterID, userID, damage); err != nil {
		return 0, false, err
	}

	if left == 0 {
		if _, err = tx.Exec(ctx,
			`UPDATE game_monsters SET killed_at = now() WHERE id = $1`, monsterID); err != nil {
			return 0, false, err
		}
		killed = true
	}
	return left, killed, tx.Commit(ctx)
}

// Hitter is one participant's share of a fight.
type Hitter struct {
	UserID   int    `db:"user_id"`
	Username string `db:"username"`
	Damage   int    `db:"damage"`
	Hits     int    `db:"hits"`
}

// Hitters lists everyone who landed a blow, hardest hitter first.
func (r *Repo) Hitters(ctx context.Context, monsterID int) ([]Hitter, int, error) {
	rows := []Hitter{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT h.user_id, u.username, h.damage, h.hits
		  FROM game_monster_hits h JOIN users u ON u.id = h.user_id
		 WHERE h.monster_id = $1
		 ORDER BY h.damage DESC, u.username`, monsterID)
	total := 0
	for _, h := range rows {
		total += h.Damage
	}
	return rows, total, err
}

// ExpireMonsters closes out anything the channel failed to kill in time and
// returns those fights, so the caller can pay the people who turned up and say
// in the channel that it got away.
//
// Returning the rows rather than a count: a fight nobody finished still had
// people in it, and both the payout and the message need to know which.
func (r *Repo) ExpireMonsters(ctx context.Context) ([]Monster, error) {
	rows := []Monster{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		UPDATE game_monsters SET killed_at = now()
		 WHERE killed_at IS NULL AND expires_at < now()
		RETURNING `+monsterCols)
	return rows, err
}

// TryAttack takes the player's turn if their cooldown has expired.
//
// The claim and the check are one statement so two rapid /atac commands cannot
// both pass: the WHERE clause is the lock. Returns ErrExists and the time the
// rest ends when it is still running.
func (r *Repo) TryAttack(ctx context.Context, userID int, rest time.Duration) (time.Time, error) {
	var next time.Time
	err := r.pool.QueryRow(ctx, `
		INSERT INTO game_attackers (user_id, next_attack_at)
		VALUES ($1, now() + $2::interval)
		ON CONFLICT (user_id) DO UPDATE
		   SET next_attack_at = now() + $2::interval
		 WHERE game_attackers.next_attack_at <= now()
		RETURNING next_attack_at`, userID, rest.String()).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		var when time.Time
		if e := r.pool.QueryRow(ctx,
			`SELECT next_attack_at FROM game_attackers WHERE user_id = $1`, userID).Scan(&when); e != nil {
			return time.Time{}, e
		}
		return when, ErrExists
	}
	return next, err
}

// PenaliseAttacker extends a player's rest after the monster hits back.
func (r *Repo) PenaliseAttacker(ctx context.Context, userID int, rest time.Duration) (time.Time, error) {
	var next time.Time
	err := r.pool.QueryRow(ctx, `
		UPDATE game_attackers
		   SET next_attack_at = now() + $2::interval, counters = counters + 1
		 WHERE user_id = $1
		RETURNING next_attack_at`, userID, rest.String()).Scan(&next)
	return next, err
}
