package repo

// Password reset. The token is a bearer credential mailed to an
// inbox, so the rules here are deliberately strict: hashed at rest, single
// use, short lived, and spending one resets the password in the same
// transaction so the two can never disagree.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrResetUnknown = errors.New("reset token not found")
	ErrResetUsed    = errors.New("reset token already used")
	ErrResetExpired = errors.New("reset token expired")
)

// NewResetToken returns a URL-safe 32-byte random token. Unlike an invite
// code this is never typed by hand — it travels in a link — so it is sized
// for unguessability rather than readability.
func NewResetToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashResetToken is a plain SHA-256, not bcrypt. bcrypt's slowness exists to
// defend low-entropy human passwords against offline guessing; a 256-bit
// random token has nothing to guess, and the lookup happens on every
// redemption where a deliberate 100ms would just be a free DoS lever.
func HashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreatePasswordReset stores the hash of a freshly minted token and
// invalidates any earlier outstanding ones for that user. Only the newest
// link works — otherwise clicking "trimite din nou" would leave every
// previous mail live, widening the window for no benefit.
func (r *Repo) CreatePasswordReset(ctx context.Context, userID int, tokenHash string, expiresAt time.Time, ip string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE password_resets SET used_at = now()
		 WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO password_resets (user_id, token_hash, expires_at, requested_ip)
		 VALUES ($1, $2, $3, NULLIF($4, ''))`, userID, tokenHash, expiresAt, ip); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ConsumePasswordReset spends the token and sets the new password in one
// transaction, returning the user it belonged to. Claim-then-update rather
// than check-then-update: the UPDATE ... WHERE used_at IS NULL RETURNING
// takes the row lock, so two concurrent redemptions of the same link cannot
// both succeed.
func (r *Repo) ConsumePasswordReset(ctx context.Context, tokenHash, newPasswordHash string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var userID int
	err = tx.QueryRow(ctx, `
		UPDATE password_resets SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, classifyReset(ctx, tx, tokenHash)
	}
	if err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`,
		userID, newPasswordHash); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return userID, nil
}

// classifyReset turns "the claim matched nothing" into the specific reason,
// so the page can say "linkul a expirat" instead of a shrug.
func classifyReset(ctx context.Context, tx pgx.Tx, tokenHash string) error {
	var usedAt *time.Time
	var expiresAt time.Time
	err := tx.QueryRow(ctx,
		`SELECT used_at, expires_at FROM password_resets WHERE token_hash = $1`,
		tokenHash).Scan(&usedAt, &expiresAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrResetUnknown
	case err != nil:
		return err
	case usedAt != nil:
		return ErrResetUsed
	case !expiresAt.After(time.Now()):
		return ErrResetExpired
	default:
		// Claimed by a concurrent request between the UPDATE and this read.
		return ErrResetUsed
	}
}
