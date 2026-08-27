package repo

// Persisted community forum: threads + replies. reply_count and
// last_activity_at are kept up to date on the thread so the list view is a
// single query. Category validation lives in the handler.

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

// forumThreadCols selects a thread row with its author joined. `body` is
// included; the list handler drops it from the JSON, the detail keeps it.
const forumThreadCols = `
	ft.id, ft.category, ft.title, ft.body, ft.is_pinned, ft.is_locked,
	ft.reply_count, ft.last_activity_at, ft.created_at,
	u.id AS "author.id", u.username AS "author.username", u.avatar_url AS "author.avatar_url"`

// ListThreads returns threads, pinned first then most-recently-active. An
// empty category means "Toate".
func (r *Repo) ListThreads(ctx context.Context, category string, limit int) ([]model.ForumThread, error) {
	rows := []model.ForumThread{}
	where := ""
	args := []any{limit}
	if category != "" {
		where = "WHERE ft.category = $2"
		args = append(args, category)
	}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+forumThreadCols+`
		FROM forum_threads ft
		JOIN users u ON u.id = ft.user_id
		`+where+`
		ORDER BY ft.is_pinned DESC, ft.last_activity_at DESC
		LIMIT $1`, args...)
	return rows, err
}

// ThreadByID fetches a single thread (with body) for the detail view.
func (r *Repo) ThreadByID(ctx context.Context, id int) (*model.ForumThread, error) {
	var t model.ForumThread
	err := pgxscan.Get(ctx, r.pool, &t, `
		SELECT `+forumThreadCols+`
		FROM forum_threads ft
		JOIN users u ON u.id = ft.user_id
		WHERE ft.id = $1`, id)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ThreadReplies returns a thread's replies oldest-first (reading order).
func (r *Repo) ThreadReplies(ctx context.Context, threadID int) ([]model.ForumReply, error) {
	rows := []model.ForumReply{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT fr.id, fr.body, fr.created_at,
		       u.id AS "author.id", u.username AS "author.username", u.avatar_url AS "author.avatar_url"
		FROM forum_replies fr
		JOIN users u ON u.id = fr.user_id
		WHERE fr.thread_id = $1
		ORDER BY fr.created_at`, threadID)
	return rows, err
}

// CreateThread inserts a new topic and returns it (author joined).
func (r *Repo) CreateThread(ctx context.Context, userID int, category, title, body string) (*model.ForumThread, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO forum_threads (user_id, category, title, body)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		userID, category, title, body).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.ThreadByID(ctx, id)
}

// CreateThreadReply appends a reply and bumps the thread's counters in one tx.
// Returns ErrNotFound if the thread is gone, ErrLocked if it's locked.
func (r *Repo) CreateThreadReply(ctx context.Context, threadID, userID int, body string) (*model.ForumReply, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var locked bool
	err = tx.QueryRow(ctx, `SELECT is_locked FROM forum_threads WHERE id = $1`, threadID).Scan(&locked)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, ErrLocked
	}

	var id int
	err = tx.QueryRow(ctx, `
		INSERT INTO forum_replies (thread_id, user_id, body)
		VALUES ($1, $2, $3) RETURNING id`, threadID, userID, body).Scan(&id)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE forum_threads
		SET reply_count = reply_count + 1, last_activity_at = now()
		WHERE id = $1`, threadID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	var reply model.ForumReply
	err = pgxscan.Get(ctx, r.pool, &reply, `
		SELECT fr.id, fr.body, fr.created_at,
		       u.id AS "author.id", u.username AS "author.username", u.avatar_url AS "author.avatar_url"
		FROM forum_replies fr JOIN users u ON u.id = fr.user_id
		WHERE fr.id = $1`, id)
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

// SetThreadPinned / SetThreadLocked are moderator toggles.
func (r *Repo) SetThreadPinned(ctx context.Context, id int, pinned bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE forum_threads SET is_pinned = $2 WHERE id = $1`, id, pinned)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *Repo) SetThreadLocked(ctx context.Context, id int, locked bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE forum_threads SET is_locked = $2 WHERE id = $1`, id, locked)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ThreadAuthor returns who started a thread (for the delete permission check).
func (r *Repo) ThreadAuthor(ctx context.Context, id int) (int, error) {
	var uid int
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM forum_threads WHERE id = $1`, id).Scan(&uid)
	if pgxscan.NotFound(err) {
		return 0, ErrNotFound
	}
	return uid, err
}

// DeleteThread removes a thread (replies cascade).
func (r *Repo) DeleteThread(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM forum_threads WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
