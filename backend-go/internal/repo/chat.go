package repo

// Live chat persistence. The panel reads a short backlog on open and
// then follows the SSE stream; everything here serves one of those two, plus
// the retention sweep that keeps the table small.

import (
	"context"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

const chatCols = `m.id, m.body, m.reply_to_user, m.reply_to_excerpt, m.reply_to_id, m.created_at,
	u.id AS user_id, u.username, u.role, u.avatar_url`

// RecentChatMessages returns the newest messages oldest-first — the order the
// panel renders. Deleted rows are tombstones and never come back.
func (r *Repo) RecentChatMessages(ctx context.Context, limit int) ([]model.ChatMessage, error) {
	rows := []model.ChatMessage{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT * FROM (
			SELECT `+chatCols+`
			FROM chat_messages m JOIN users u ON u.id = m.user_id
			WHERE m.deleted_at IS NULL
			ORDER BY m.id DESC
			LIMIT $1
		) recent ORDER BY id`, limit)
	return rows, err
}

// InsertChatMessage stores a message and returns it fully joined, so the hub can
// broadcast exactly what a reader would have fetched.
func (r *Repo) InsertChatMessage(ctx context.Context, userID int, body string, replyUser, replyExcerpt *string, replyToID *int64) (*model.ChatMessage, error) {
	var id int64
	// reply_to_id is a foreign key, so a client that sends an id for a message
	// that has since been deleted would fail the whole insert. Resolve it to
	// NULL instead: losing the jump target must not lose the message.
	err := r.pool.QueryRow(ctx, `
		INSERT INTO chat_messages (user_id, body, reply_to_user, reply_to_excerpt, reply_to_id)
		VALUES ($1, $2, $3, $4,
		        (SELECT id FROM chat_messages WHERE id = $5 AND deleted_at IS NULL))
		RETURNING id`,
		userID, body, replyUser, replyExcerpt, replyToID).Scan(&id)
	if err != nil {
		return nil, err
	}
	var m model.ChatMessage
	err = pgxscan.Get(ctx, r.pool, &m, `
		SELECT `+chatCols+`
		FROM chat_messages m JOIN users u ON u.id = m.user_id
		WHERE m.id = $1`, id)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DeleteChatMessage tombstones one message (moderator action). Returns the
// author so the caller can decide whether the requester was allowed.
func (r *Repo) DeleteChatMessage(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE chat_messages SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ChatMessageAuthor is the ownership check for deletes.
func (r *Repo) ChatMessageAuthor(ctx context.Context, id int64) (int, error) {
	var userID int
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM chat_messages WHERE id = $1`, id).Scan(&userID)
	if err != nil {
		return 0, ErrNotFound
	}
	return userID, nil
}

// PurgeChatBefore drops messages older than the cutoff for good — tombstones
// included, since a deleted message past retention has no audience left.
func (r *Repo) PurgeChatBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM chat_messages WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ── Chat restrictions (timeouts / bans, chat-only) ────────────────────────────

const restrictionCols = `c.user_id, u.username, c.expires_at, c.reason, c.created_at,
	b.username AS by_name`

// ChatRestrictionFor returns the live restriction on a user, or nil. A lapsed
// timeout is filtered out here rather than deleted: the send path must stay a
// read, and the janitor collects the rows.
func (r *Repo) ChatRestrictionFor(ctx context.Context, userID int) (*model.ChatRestriction, error) {
	var c model.ChatRestriction
	err := pgxscan.Get(ctx, r.pool, &c, `
		SELECT `+restrictionCols+`
		FROM chat_restrictions c
		JOIN users u ON u.id = c.user_id
		LEFT JOIN users b ON b.id = c.created_by
		WHERE c.user_id = $1 AND (c.expires_at IS NULL OR c.expires_at > now())`, userID)
	if err != nil {
		if pgxscan.NotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// SetChatRestriction mutes a user. A nil `until` is a permanent ban; otherwise
// it's a timeout that expires on its own. One row per user — re-muting someone
// replaces the previous restriction instead of stacking.
func (r *Repo) SetChatRestriction(ctx context.Context, userID int, until *time.Time, by int, reason *string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO chat_restrictions (user_id, expires_at, reason, created_by, created_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id) DO UPDATE
		   SET expires_at = EXCLUDED.expires_at,
		       reason     = EXCLUDED.reason,
		       created_by = EXCLUDED.created_by,
		       created_at = now()`, userID, until, reason, by)
	return err
}

// ClearChatRestriction lifts a timeout or ban early.
func (r *Repo) ClearChatRestriction(ctx context.Context, userID int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM chat_restrictions WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// PurgeExpiredChatRestrictions collects timeouts that have already lapsed.
// Nothing depends on it running — an expired row is already inert — it just
// keeps the table from accumulating one row per timeout ever handed out.
func (r *Repo) PurgeExpiredChatRestrictions(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM chat_restrictions WHERE expires_at IS NOT NULL AND expires_at <= now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
