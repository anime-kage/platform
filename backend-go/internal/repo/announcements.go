package repo

// Site announcements — the "Știri & anunțuri" strip on /home and its editor in
// the admin panel. Small, hand-written rows: no derivation, no cache.

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

// announcementCols is shared by every read so the admin list and the home feed
// can never drift into showing different fields.
const announcementCols = `
	a.id, a.tag, a.title, a.body, a.cover_url, a.slug, a.url, a.is_published,
	u.username AS author_name, a.created_at, a.updated_at,
	(SELECT count(*)::int FROM comments c
	  WHERE c.announcement_id = a.id AND c.is_deleted = false) AS comment_count`

// ListAnnouncements returns announcements newest first. includeDrafts is the
// admin view; the home feed passes false and sees only what went out.
func (r *Repo) ListAnnouncements(ctx context.Context, includeDrafts bool, limit int) ([]model.Announcement, error) {
	rows := []model.Announcement{}
	where := "WHERE a.is_published"
	if includeDrafts {
		where = ""
	}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+announcementCols+`
		FROM announcements a
		LEFT JOIN users u ON u.id = a.author_id
		`+where+`
		ORDER BY a.created_at DESC
		LIMIT $1`, limit)
	return rows, err
}

// AnnouncementByID reads one row back after a write, so the client always gets
// the stored version (including the joined author) rather than its own input.
func (r *Repo) AnnouncementByID(ctx context.Context, id int) (model.Announcement, error) {
	var a model.Announcement
	err := pgxscan.Get(ctx, r.pool, &a, `
		SELECT `+announcementCols+`
		FROM announcements a
		LEFT JOIN users u ON u.id = a.author_id
		WHERE a.id = $1`, id)
	return a, err
}

// AnnouncementBySlug resolves a pretty URL segment to its post.
func (r *Repo) AnnouncementBySlug(ctx context.Context, slug string) (model.Announcement, error) {
	var a model.Announcement
	err := pgxscan.Get(ctx, r.pool, &a, `
		SELECT `+announcementCols+`
		FROM announcements a
		LEFT JOIN users u ON u.id = a.author_id
		WHERE a.slug = $1`, slug)
	if pgxscan.NotFound(err) {
		return a, ErrNotFound
	}
	return a, err
}

// SetAnnouncementSlug stores a slug, reporting ErrExists so the caller can
// disambiguate rather than this silently mangling it.
func (r *Repo) SetAnnouncementSlug(ctx context.Context, id int, slug string) error {
	_, err := r.pool.Exec(ctx, `UPDATE announcements SET slug = $2 WHERE id = $1`, id, slug)
	if err != nil && IsUniqueViolation(err) {
		return ErrExists
	}
	return err
}

// CreateAnnouncement inserts and returns the stored row.
func (r *Repo) CreateAnnouncement(ctx context.Context, authorID int, tag, title string, body, url, cover *string, published bool) (model.Announcement, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO announcements (tag, title, body, url, cover_url, is_published, author_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`, tag, title, body, url, cover, published, authorID).Scan(&id)
	if err != nil {
		return model.Announcement{}, err
	}
	return r.AnnouncementByID(ctx, id)
}

// UpdateAnnouncement replaces every editable field. ErrNotFound when the row is
// gone (someone deleted it in another tab).
func (r *Repo) UpdateAnnouncement(ctx context.Context, id int, tag, title string, body, url, cover *string, published bool) (model.Announcement, error) {
	tagRes, err := r.pool.Exec(ctx, `
		UPDATE announcements
		SET tag = $2, title = $3, body = $4, url = $5, cover_url = $6,
		    is_published = $7, updated_at = now()
		WHERE id = $1`, id, tag, title, body, url, cover, published)
	if err != nil {
		return model.Announcement{}, err
	}
	if tagRes.RowsAffected() == 0 {
		return model.Announcement{}, ErrNotFound
	}
	return r.AnnouncementByID(ctx, id)
}

// DeleteAnnouncement removes a row for good.
func (r *Repo) DeleteAnnouncement(ctx context.Context, id int) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM announcements WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
