package repo

import (
	"time"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"

	"animekage/backend/internal/model"
)

var ErrNotFound = errors.New("not found")

const userCols = `id, username, email, avatar_url, bio, favorite_genres, favorites, role, created_at, updated_at`

// userRow carries favorites as raw jsonb before decoding into the model.
type userRow struct {
	model.User
	FavoritesRaw []byte `db:"favorites"`
}

func (r *userRow) toUser() model.User {
	u := r.User
	if len(r.FavoritesRaw) > 0 {
		_ = json.Unmarshal(r.FavoritesRaw, &u.Favorites)
	}
	if u.Favorites == nil {
		u.Favorites = []model.FavoriteRef{}
	}
	if u.FavoriteGenres == nil {
		u.FavoriteGenres = []string{}
	}
	return u
}

func (r *Repo) userBy(ctx context.Context, where string, arg any) (*model.User, error) {
	var row userRow
	err := pgxscan.Get(ctx, r.pool, &row, `SELECT `+userCols+` FROM users WHERE `+where, arg)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u := row.toUser()
	return &u, nil
}

func (r *Repo) UserByID(ctx context.Context, id int) (*model.User, error) {
	return r.userBy(ctx, "id = $1", id)
}

func (r *Repo) UserByUsername(ctx context.Context, username string) (*model.User, error) {
	return r.userBy(ctx, "username = $1", username)
}

func (r *Repo) UserByEmail(ctx context.Context, email string) (*model.User, error) {
	return r.userBy(ctx, "email = $1", email)
}

// PasswordHashByEmail returns (userId, hash) for login; the full row never
// travels with the hash attached (code-review fix 0.3).
func (r *Repo) PasswordHashByEmail(ctx context.Context, email string) (int, string, error) {
	var row struct {
		ID           int    `db:"id"`
		PasswordHash string `db:"password_hash"`
	}
	err := pgxscan.Get(ctx, r.pool, &row, `SELECT id, password_hash FROM users WHERE email = $1`, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return row.ID, row.PasswordHash, err
}

func (r *Repo) CreateUser(ctx context.Context, username, email, passwordHash string) (*model.User, error) {
	var row userRow
	err := pgxscan.Get(ctx, r.pool, &row, `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING `+userCols, username, email, passwordHash)
	if err != nil {
		return nil, err
	}
	u := row.toUser()
	return &u, nil
}

func (r *Repo) UsernameTaken(ctx context.Context, username string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)`, username).Scan(&exists)
	return exists, err
}

func (r *Repo) EmailTaken(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists)
	return exists, err
}

// ProfileUpdate carries only the fields the caller wants changed (nil = keep).
type ProfileUpdate struct {
	Username       *string
	Bio            *string
	FavoriteGenres []string
	Favorites      []model.FavoriteRef
	AvatarURL      *string
}

func (r *Repo) UpdateProfile(ctx context.Context, userID int, upd ProfileUpdate) (*model.User, error) {
	sets := []string{"updated_at = now()"}
	args := []any{userID}
	add := func(expr string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf(expr, len(args)))
	}
	if upd.Username != nil {
		add("username = $%d", *upd.Username)
	}
	if upd.Bio != nil {
		add("bio = $%d", *upd.Bio)
	}
	if upd.FavoriteGenres != nil {
		add("favorite_genres = $%d", upd.FavoriteGenres)
	}
	if upd.Favorites != nil {
		favJSON, err := json.Marshal(upd.Favorites)
		if err != nil {
			return nil, err
		}
		add("favorites = $%d", favJSON)
	}
	if upd.AvatarURL != nil {
		add("avatar_url = $%d", *upd.AvatarURL)
	}

	var row userRow
	err := pgxscan.Get(ctx, r.pool, &row, `
		UPDATE users SET `+strings.Join(sets, ", ")+`
		WHERE id = $1
		RETURNING `+userCols, args...)
	if err != nil {
		return nil, err
	}
	u := row.toUser()
	return &u, nil
}

// IsUniqueViolation reports whether err is a Postgres unique-constraint error,
// used to turn insert races into 409s instead of 500s.
func IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}

// IsForeignKeyViolation reports a Postgres FK error — an insert referencing a
// row that doesn't exist, which handlers usually turn into a 404.
func IsForeignKeyViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23503")
}

// ── Stats ────────────────────────────────────────────────────────────────────

// UserStats mirrors the old getStats exactly: every number is computed over
// COMPLETED entries only, and hours assume ~24 min per episode.
func (r *Repo) UserStats(ctx context.Context, userID int) (*model.UserStats, error) {
	s := &model.UserStats{}
	var avgAnime, avgManga *float64

	err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int, coalesce(sum(episodes_watched), 0)::int, avg(score)
		FROM watchlist WHERE user_id = $1 AND status = 'completed'`, userID).
		Scan(&s.TotalAnimeWatched, &s.TotalEpisodesWatched, &avgAnime)
	if err != nil {
		return nil, err
	}
	s.TotalHoursWatched = int(float64(s.TotalEpisodesWatched)*24/60 + 0.5)
	if avgAnime != nil {
		s.AverageAnimeScore = *avgAnime
	}

	err = r.pool.QueryRow(ctx, `
		SELECT count(*)::int, coalesce(sum(chapters_read), 0)::int, avg(score)
		FROM readlist WHERE user_id = $1 AND status = 'completed'`, userID).
		Scan(&s.TotalMangaRead, &s.TotalChaptersRead, &avgManga)
	if err != nil {
		return nil, err
	}
	if avgManga != nil {
		s.AverageMangaScore = *avgManga
	}
	return s, nil
}

// ── History ──────────────────────────────────────────────────────────────────

// History returns per-day totals for days that have events (no zero-padding —
// the old backend didn't pad either; the frontend fills gaps).
func (r *Repo) History(ctx context.Context, userID, days int) ([]model.HistoryDay, error) {
	rows := []model.HistoryDay{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT
			to_char(created_at, 'YYYY-MM-DD') AS date,
			coalesce(sum(amount) FILTER (WHERE anime_id IS NOT NULL), 0)::int AS episodes,
			coalesce(sum(amount) FILTER (WHERE manga_id IS NOT NULL), 0)::int AS chapters
		FROM watch_history
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - ($2::int - 1)
		GROUP BY 1 ORDER BY 1`, userID, days)
	return rows, err
}

func (r *Repo) LogHistory(ctx context.Context, userID int, animeID, mangaID *int, amount int) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO watch_history (user_id, anime_id, manga_id, amount) VALUES ($1, $2, $3, $4)`,
		userID, animeID, mangaID, amount)
	return err
}

// ── Follows ──────────────────────────────────────────────────────────────────

func (r *Repo) FollowCounts(ctx context.Context, userID int) (followers, following int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM follows WHERE following_id = $1),
			(SELECT count(*) FROM follows WHERE follower_id = $1)`, userID).
		Scan(&followers, &following)
	return
}

func (r *Repo) IsFollowing(ctx context.Context, followerID, followingID int) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id = $1 AND following_id = $2)`,
		followerID, followingID).Scan(&exists)
	return exists, err
}

func (r *Repo) Follow(ctx context.Context, followerID, followingID int) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO follows (follower_id, following_id) VALUES ($1, $2)
		ON CONFLICT (follower_id, following_id) DO NOTHING`, followerID, followingID)
	return err
}

func (r *Repo) Unfollow(ctx context.Context, followerID, followingID int) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM follows WHERE follower_id = $1 AND following_id = $2`, followerID, followingID)
	return err
}

// FollowList returns the followers of / accounts followed by userID.
// viewerID personalizes each row's isFollowing (0 = guest).
func (r *Repo) FollowList(ctx context.Context, userID int, kind string, viewerID int) ([]model.FollowUser, error) {
	// followers: join the follower side, anchor on following_id — and vice versa
	side, anchor := "follower_id", "following_id"
	if kind == "following" {
		side, anchor = "following_id", "follower_id"
	}
	rows := []model.FollowUser{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT
			u.id, u.username, u.bio, u.avatar_url, u.role,
			(SELECT count(*) FROM follows f2 WHERE f2.following_id = u.id)::int AS followers_count,
			CASE WHEN $2 = 0 THEN false
			     ELSE EXISTS(SELECT 1 FROM follows f3 WHERE f3.follower_id = $2 AND f3.following_id = u.id)
			END AS is_following
		FROM follows f
		JOIN users u ON u.id = f.`+side+`
		WHERE f.`+anchor+` = $1
		ORDER BY f.created_at DESC`, userID, viewerID)
	return rows, err
}

// TouchLastSeen records that a member made a request just now.
//
// Called from the auth middleware on every authenticated request, so it is
// deliberately a blind UPDATE with no read: the caller throttles to at most one
// write per user per minute, which is what keeps this off the hot path.
func (r *Repo) TouchLastSeen(ctx context.Context, userID int) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET last_seen_at = now() WHERE id = $1`, userID)
	return err
}

// OnlineCount is how many members made an authenticated request inside the
// window. Not the chat hub's viewer count, which only ever saw members with the
// chat panel open and counted two tabs as two people.
func (r *Repo) OnlineCount(ctx context.Context, window time.Duration) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE last_seen_at > now() - $1::interval`,
		window.String()).Scan(&n)
	return n, err
}
