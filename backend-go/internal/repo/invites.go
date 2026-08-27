package repo

// Invite-only registration: the Discord bot mints codes here and
// register redeems them. Both sides share this file rather than each holding
// their own copy of the rules — that is the whole reason the bot is a sibling
// binary in this module and not a separate service.

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"

	"animekage/backend/internal/model"
)

// Redemption failures worth telling the user apart — "that code doesn't
// exist" and "you already used it" are very different mistakes.
var (
	ErrInviteUnknown = errors.New("invite code not found")
	ErrInviteUsed    = errors.New("invite code already used")
	ErrInviteExpired = errors.New("invite code expired")
)

const inviteCols = `id, code, discord_user_id, discord_username,
	created_at, expires_at, used_by_user_id, used_at`

// codeAlphabet omits 0/O and 1/I/L: these codes get read off a screen and
// typed by hand, and those are the pairs people get wrong.
const codeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// NewInviteCode returns a code shaped like the one the landing page
// advertises: KAGE-7F2A-9X. Six random characters over a 31-symbol alphabet
// is ~8.9e8 combinations — brute-forcing it past the auth rate limiter, to
// land on the far smaller set of codes that are actually outstanding, is not
// a realistic attack.
func NewInviteCode() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, 6)
	for i, b := range buf {
		out[i] = codeAlphabet[int(b)%len(codeAlphabet)]
	}
	return fmt.Sprintf("KAGE-%s-%s", out[:4], out[4:]), nil
}

// OutstandingInvite returns this member's still-unclaimed, unexpired code if
// they have one. The bot shows it again instead of minting a second: it keeps
// the daily quota honest and stops the table filling with dead codes when
// someone runs the command twice.
func (r *Repo) OutstandingInvite(ctx context.Context, discordUserID string) (*model.Invite, error) {
	var inv model.Invite
	err := pgxscan.Get(ctx, r.pool, &inv,
		`SELECT `+inviteCols+` FROM invites
		 WHERE discord_user_id = $1 AND used_at IS NULL
		   AND (expires_at IS NULL OR expires_at > now())
		 ORDER BY created_at DESC
		 LIMIT 1`, discordUserID)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// LastInviteAt is when this member last minted a code, for the daily quota.
// Deliberately counts codes *created*, not codes still unused: spending your
// invite does not earn you another one the same day.
func (r *Repo) LastInviteAt(ctx context.Context, discordUserID string) (time.Time, error) {
	var at time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT created_at FROM invites
		 WHERE discord_user_id = $1
		 ORDER BY created_at DESC LIMIT 1`, discordUserID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	return at, err
}

type CreateInviteInput struct {
	Code            string
	DiscordUserID   string
	DiscordUsername string
	ExpiresAt       *time.Time
}

func (r *Repo) CreateInvite(ctx context.Context, in CreateInviteInput) (*model.Invite, error) {
	var username *string
	if in.DiscordUsername != "" {
		username = &in.DiscordUsername
	}
	var inv model.Invite
	err := pgxscan.Get(ctx, r.pool, &inv, `
		INSERT INTO invites (code, discord_user_id, discord_username, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING `+inviteCols,
		in.Code, in.DiscordUserID, username, in.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// CreateUserWithInvite creates the account and spends the code in one
// transaction.
//
// Order matters: the user is inserted first and the invite claimed second,
// because the claim is what can lose a race. `UPDATE … WHERE used_at IS NULL`
// takes the row lock, so a second registration racing on the same code
// updates zero rows and rolls the whole thing back — no orphaned account, and
// no code burned by a signup that then failed. Doing it the other way round
// would trip the invites_used_together CHECK, since used_at and
// used_by_user_id have to be set in the same statement.
func (r *Repo) CreateUserWithInvite(ctx context.Context, username, email, passwordHash, code string) (*model.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var row userRow
	if err := pgxscan.Get(ctx, tx, &row, `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING `+userCols, username, email, passwordHash); err != nil {
		return nil, err
	}

	var claimed int
	err = tx.QueryRow(ctx, `
		UPDATE invites SET used_at = now(), used_by_user_id = $2
		WHERE code = $1 AND used_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		RETURNING id`, code, row.ID).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, classifyInvite(ctx, tx, code)
	}
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	u := row.toUser()
	return &u, nil
}

// classifyInvite turns a failed claim into the reason, so the user is told
// whether the code is wrong, spent or stale rather than a flat "invalid".
func classifyInvite(ctx context.Context, tx pgx.Tx, code string) error {
	var used, expired bool
	err := tx.QueryRow(ctx, `
		SELECT used_at IS NOT NULL,
		       expires_at IS NOT NULL AND expires_at <= now()
		FROM invites WHERE code = $1`, code).Scan(&used, &expired)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrInviteUnknown
	case err != nil:
		return err
	case used:
		return ErrInviteUsed
	case expired:
		return ErrInviteExpired
	default:
		return ErrInviteUnknown
	}
}
