package repo

// In-app notifications. Rows are written best-effort from the event handlers
// (follow, reply, release lifecycle); reads power the header bell and the
// /notificari inbox. The body is stored pre-rendered so the list needs only a
// single left join to the actor for the avatar.

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

// CreateNotification inserts one inbox row. actorID/link may be nil (system
// events, or events with no navigable target). Callers treat failures as
// non-fatal — a dropped notification must never fail the action that caused it.
func (r *Repo) CreateNotification(ctx context.Context, userID int, typ, body string, actorID *int, link *string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notifications (user_id, type, actor_id, body, link)
		VALUES ($1, $2, $3, $4, $5)`,
		userID, typ, actorID, body, link)
	return err
}

// Notifications returns the recipient's most recent rows, newest first.
func (r *Repo) Notifications(ctx context.Context, userID, limit int) ([]model.Notification, error) {
	rows := []model.Notification{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT n.id, n.type, n.body, n.link,
		       au.username AS actor,
		       (n.read_at IS NULL) AS unread,
		       n.created_at
		FROM notifications n
		LEFT JOIN users au ON au.id = n.actor_id
		WHERE n.user_id = $1
		ORDER BY n.created_at DESC
		LIMIT $2`, userID, limit)
	return rows, err
}

// UnreadCount is the header badge number.
func (r *Repo) UnreadCount(ctx context.Context, userID int) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`,
		userID).Scan(&n)
	return n, err
}

// MarkAllRead clears the badge in one shot.
func (r *Repo) MarkAllRead(ctx context.Context, userID int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`,
		userID)
	return err
}

// MarkNotificationRead clears a single row. Scoped to userID so a member can
// only touch their own inbox.
func (r *Repo) MarkNotificationRead(ctx context.Context, userID, id int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET read_at = now()
		 WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		id, userID)
	return err
}
