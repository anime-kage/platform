package repo

// Moderation queue: reported comments and user sanctions. Reports
// come from POST /api/comments/:id/report (any logged-in user, since the
// baseline); this is the side that finally consumes them.

import (
	"context"
	"strings"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

// ReportedComment is one row of the moderation queue: the comment plus enough
// context to judge it without leaving the page.
type ReportedComment struct {
	ID           int     `db:"id" json:"id"`
	Content      string  `db:"content" json:"content"`
	CreatedAt    string  `db:"created_at" json:"createdAt"`
	UserID       int     `db:"user_id" json:"userId"`
	Username     string  `db:"username" json:"username"`
	UserRole     string  `db:"user_role" json:"userRole"`
	UserBanned   bool    `db:"user_banned" json:"userBanned"`
	AnimeID      *int    `db:"anime_id" json:"animeId,omitempty"`
	MangaID      *int    `db:"manga_id" json:"mangaId,omitempty"`
	ContextTitle *string `db:"context_title" json:"contextTitle,omitempty"`
}

func (r *Repo) ReportedComments(ctx context.Context, limit, offset int) ([]ReportedComment, int, error) {
	rows := []ReportedComment{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT c.id, c.content, c.created_at::text AS created_at,
		       c.user_id, u.username, u.role AS user_role, u.is_banned AS user_banned,
		       c.anime_id, c.manga_id,
		       coalesce(a.title, m.title) AS context_title
		FROM comments c
		JOIN users u ON u.id = c.user_id
		LEFT JOIN anime a ON a.id = c.anime_id
		LEFT JOIN manga m ON m.id = c.manga_id
		WHERE c.is_reported = true AND c.is_deleted = false
		ORDER BY c.created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	var total int
	err = r.pool.QueryRow(ctx,
		`SELECT count(*) FROM comments WHERE is_reported = true AND is_deleted = false`).Scan(&total)
	return rows, total, err
}

// DismissReport keeps the comment and clears the flag.
func (r *Repo) DismissReport(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE comments SET is_reported = false WHERE id = $1 AND is_reported = true`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ModDeleteComment soft-deletes ANY comment (no owner check — moderator
// power) and clears its report flag, with the same thread-count shrink as an
// owner delete.
func (r *Repo) ModDeleteComment(ctx context.Context, id int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var parentID, rootID *int
	err = tx.QueryRow(ctx,
		`SELECT parent_id, root_id FROM comments WHERE id = $1 AND is_deleted = false`,
		id).Scan(&parentID, &rootID)
	if pgxscan.NotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE comments SET is_deleted = true, is_reported = false, updated_at = now()
		 WHERE id = $1`, id); err != nil {
		return err
	}
	target := rootID
	if target == nil {
		target = parentID
	}
	if target != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE comments SET replies_count = GREATEST(0, replies_count - 1) WHERE id = $1`, *target); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ModUser is a row of the admin user manager.
type ModUser struct {
	ID       int    `db:"id" json:"id"`
	Username string `db:"username" json:"username"`
	Role     string `db:"role" json:"role"`
	IsBanned bool   `db:"is_banned" json:"isBanned"`
}

// TeamMember is a row of the admin team tab: everyone holding a team role.
type TeamMember struct {
	ID       int    `db:"id" json:"id"`
	Username string `db:"username" json:"username"`
	Role     string `db:"role" json:"role"`
	IsBanned bool   `db:"is_banned" json:"isBanned"`
	// ReleaseCap is the per-user override; nil means the global default.
	ReleaseCap *int `db:"release_cap" json:"releaseCap"`
	// InFlight is how many unpublished anime releases they hold right now —
	// the cap is meaningless to look at without it.
	InFlight  int       `db:"in_flight" json:"inFlight"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// TeamMembers lists users with a team role (anything above plain 'user'),
// admins first, then by seniority.
func (r *Repo) TeamMembers(ctx context.Context) ([]TeamMember, error) {
	rows := []TeamMember{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT u.id, u.username, u.role, u.is_banned, u.release_cap, u.created_at,
		       (SELECT count(*) FROM releases r
		         WHERE r.uploader_id = u.id AND r.medium = 'anime' AND r.state <> 'published')
		         AS in_flight
		FROM users u
		WHERE u.role <> 'user'
		ORDER BY array_position(ARRAY['admin','moderator','coordinator','verifier','translator'], u.role), u.created_at`)
	return rows, err
}

func (r *Repo) FindUsers(ctx context.Context, q string, limit int) ([]ModUser, error) {
	rows := []ModUser{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT id, username, role, is_banned
		FROM users
		WHERE username ILIKE '%' || $1 || '%'
		ORDER BY username
		LIMIT $2`, q, limit)
	return rows, err
}

// UserSanction reads the fields moderation decisions hinge on.
func (r *Repo) UserSanction(ctx context.Context, id int) (*ModUser, error) {
	var u ModUser
	err := pgxscan.Get(ctx, r.pool, &u,
		`SELECT id, username, role, is_banned FROM users WHERE id = $1`, id)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) SetUserBanned(ctx context.Context, id int, banned bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET is_banned = $2, updated_at = now() WHERE id = $1`, id, banned)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) SetUserRole(ctx context.Context, id int, role string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET role = $2, updated_at = now() WHERE id = $1`, id, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IsUserBanned gates login and posting. JWTs are stateless, so a ban takes
// effect at the next write attempt rather than by revoking tokens.
func (r *Repo) IsUserBanned(ctx context.Context, id int) (bool, error) {
	var banned bool
	err := r.pool.QueryRow(ctx, `SELECT is_banned FROM users WHERE id = $1`, id).Scan(&banned)
	return banned, err
}

// UserRole reads the live role for the auth middleware's RoleLookup — role
// changes from the admin panel apply on the next request, not the next login.
func (r *Repo) UserRole(ctx context.Context, id int) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, id).Scan(&role)
	return role, err
}

// ── episode reports ───────────────────────────────────────────────────────────

// CreateEpisodeReport files a member's report. ErrNotFound when the episode is
// gone, so a stale page cannot create an orphan.
func (r *Repo) CreateEpisodeReport(ctx context.Context, episodeID, userID int, body string) (int, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO episode_reports (episode_id, user_id, body)
		SELECT $1, $2, $3 WHERE EXISTS (SELECT 1 FROM episodes WHERE id = $1)
		RETURNING id`, episodeID, userID, body).Scan(&id)
	if err != nil && strings.Contains(err.Error(), "no rows") {
		return 0, ErrNotFound
	}
	return id, err
}

const episodeReportCols = `r.id, r.episode_id, e.episode_number, a.id AS anime_id,
	a.slug AS anime_slug, a.title AS anime_title, r.body, r.status,
	u.username AS reporter, r.created_at, r.resolved_at`

// EpisodeReports lists the queue, open first and newest first within that.
func (r *Repo) EpisodeReports(ctx context.Context, status string, limit, offset int) ([]model.EpisodeReport, int, error) {
	where := ""
	args := []any{limit, offset}
	if status == "open" || status == "resolved" {
		where = " WHERE r.status = $3"
		args = append(args, status)
	}
	rows := []model.EpisodeReport{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+episodeReportCols+`
		FROM episode_reports r
		JOIN episodes e ON e.id = r.episode_id
		JOIN anime a ON a.id = e.anime_id
		LEFT JOIN users u ON u.id = r.user_id`+where+`
		ORDER BY (r.status = 'open') DESC, r.created_at DESC
		LIMIT $1 OFFSET $2`, args...)
	if err != nil {
		return nil, 0, err
	}
	var total int
	countSQL := `SELECT count(*) FROM episode_reports r`
	if where != "" {
		countSQL += ` WHERE r.status = $1`
		err = r.pool.QueryRow(ctx, countSQL, status).Scan(&total)
	} else {
		err = r.pool.QueryRow(ctx, countSQL).Scan(&total)
	}
	return rows, total, err
}

// ResolveEpisodeReport closes one report. ErrNotFound when it was already
// closed, so a double-click cannot silently look like it did something.
func (r *Repo) ResolveEpisodeReport(ctx context.Context, id, moderatorID int) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE episode_reports
		SET status = 'resolved', resolved_at = now(), resolved_by = $2
		WHERE id = $1 AND status = 'open'`, id, moderatorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
