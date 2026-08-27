package repo

// Custom user lists ("Liste") — curated collections with per-item notes.
// Public lists are readable by anyone; visibility is enforced in the handler
// (the repo returns rows, the handler decides who may see them).

import (
	"context"
	"errors"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"

	"animekage/backend/internal/model"
)

// userListCols selects a list row with its derived display fields: owner
// name and avatar, item count, the first five item posters for the card fans,
// the like count, and whether `viewerParam` (a SQL placeholder like "$2", or
// "0" for a guest) has liked it.
//
// The avatar is joined here because every list card shows a byline. Without it
// the UI could only draw a monogram of the first letter, which is what /liste
// did for every list including real members' own.
func userListCols(viewerParam string) string {
	return `
	ul.id, ul.user_id, ul.title, ul.description, ul.is_public,
	ul.created_at, ul.updated_at,
	u.username AS owner_name,
	u.avatar_url AS owner_avatar_url,
	(SELECT count(*) FROM user_list_items i WHERE i.list_id = ul.id) AS item_count,
	ARRAY(
		SELECT coalesce(a.image_url, m.image_url)
		FROM user_list_items i
		LEFT JOIN anime a ON a.id = i.anime_id
		LEFT JOIN manga m ON m.id = i.manga_id
		WHERE i.list_id = ul.id AND coalesce(a.image_url, m.image_url) IS NOT NULL
		ORDER BY i.position, i.id
		LIMIT 5
	) AS covers,
	(SELECT count(*) FROM list_likes ll WHERE ll.list_id = ul.id) AS like_count,
	EXISTS(SELECT 1 FROM list_likes ll WHERE ll.list_id = ul.id AND ll.user_id = ` + viewerParam + `) AS liked`
}

func (r *Repo) UserListsByOwner(ctx context.Context, userID, viewerID int) ([]model.UserList, error) {
	rows := []model.UserList{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+userListCols("$2")+`
		FROM user_lists ul JOIN users u ON u.id = ul.user_id
		WHERE ul.user_id = $1
		ORDER BY ul.updated_at DESC`, userID, viewerID)
	return rows, err
}

// PublicUserLists feeds the browse tab: public lists that actually have
// content, most-liked first (ties broken by recency).
func (r *Repo) PublicUserLists(ctx context.Context, viewerID, limit int) ([]model.UserList, error) {
	rows := []model.UserList{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+userListCols("$1")+`
		FROM user_lists ul JOIN users u ON u.id = ul.user_id
		WHERE ul.is_public
		  AND EXISTS (SELECT 1 FROM user_list_items i WHERE i.list_id = ul.id)
		ORDER BY like_count DESC, ul.updated_at DESC
		LIMIT $2`, viewerID, limit)
	return rows, err
}

func (r *Repo) UserListByID(ctx context.Context, id, viewerID int) (*model.UserList, error) {
	var ul model.UserList
	err := pgxscan.Get(ctx, r.pool, &ul, `
		SELECT `+userListCols("$2")+`
		FROM user_lists ul JOIN users u ON u.id = ul.user_id
		WHERE ul.id = $1`, id, viewerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ul, nil
}

// LikeList records a like (idempotent) and returns the new like count.
func (r *Repo) LikeList(ctx context.Context, listID, userID int) (int, error) {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO list_likes (list_id, user_id) VALUES ($1, $2)
		 ON CONFLICT (list_id, user_id) DO NOTHING`, listID, userID); err != nil {
		return 0, err
	}
	return r.listLikeCount(ctx, listID)
}

// UnlikeList removes a like and returns the new like count.
func (r *Repo) UnlikeList(ctx context.Context, listID, userID int) (int, error) {
	if _, err := r.pool.Exec(ctx,
		`DELETE FROM list_likes WHERE list_id = $1 AND user_id = $2`, listID, userID); err != nil {
		return 0, err
	}
	return r.listLikeCount(ctx, listID)
}

func (r *Repo) listLikeCount(ctx context.Context, listID int) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM list_likes WHERE list_id = $1`, listID).Scan(&n)
	return n, err
}

func (r *Repo) CreateUserList(ctx context.Context, userID int, title string, description *string, isPublic bool) (*model.UserList, error) {
	var ul model.UserList
	err := pgxscan.Get(ctx, r.pool, &ul, `
		WITH ins AS (
			INSERT INTO user_lists (user_id, title, description, is_public)
			VALUES ($1, $2, $3, $4)
			RETURNING *
		)
		SELECT `+userListCols("$1")+`
		FROM ins ul JOIN users u ON u.id = ul.user_id`,
		userID, title, description, isPublic)
	if err != nil {
		return nil, err
	}
	return &ul, nil
}

func (r *Repo) UpdateUserList(ctx context.Context, id int, title string, description *string, isPublic bool) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE user_lists
		SET title = $2, description = $3, is_public = $4, updated_at = now()
		WHERE id = $1`, id, title, description, isPublic)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) DeleteUserList(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM user_lists WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// userListItemCols joins the catalog row behind each item, whichever medium
// it points at.
const userListItemCols = `
	i.id, i.list_id, i.anime_id, i.manga_id, i.note, i.position, i.added_at,
	coalesce(a.title, m.title) AS title,
	coalesce(a.title_romanian, m.title_romanian) AS title_romanian,
	coalesce(a.image_url, m.image_url) AS image_url,
	coalesce(a.year, m.year) AS year,
	coalesce(a.score, m.score) AS score,
	coalesce(a.genres, m.genres, '{}') AS genres`

func (r *Repo) UserListItems(ctx context.Context, listID int) ([]model.UserListItem, error) {
	rows := []model.UserListItem{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+userListItemCols+`
		FROM user_list_items i
		LEFT JOIN anime a ON a.id = i.anime_id
		LEFT JOIN manga m ON m.id = i.manga_id
		WHERE i.list_id = $1
		ORDER BY i.position, i.id`, listID)
	return rows, err
}

// AddUserListItem appends a title (position = end of list). The partial
// unique indexes surface duplicates as ErrExists.
func (r *Repo) AddUserListItem(ctx context.Context, listID int, animeID, mangaID *int, note *string) (*model.UserListItem, error) {
	var it model.UserListItem
	err := pgxscan.Get(ctx, r.pool, &it, `
		WITH ins AS (
			INSERT INTO user_list_items (list_id, anime_id, manga_id, note, position)
			VALUES ($1, $2, $3, $4,
			        (SELECT coalesce(max(position), 0) + 1 FROM user_list_items WHERE list_id = $1))
			RETURNING *
		)
		SELECT `+userListItemCols+`
		FROM ins i
		LEFT JOIN anime a ON a.id = i.anime_id
		LEFT JOIN manga m ON m.id = i.manga_id`,
		listID, animeID, mangaID, note)
	if err != nil {
		if IsUniqueViolation(err) {
			return nil, ErrExists
		}
		return nil, err
	}
	r.touchUserList(ctx, listID)
	return &it, nil
}

func (r *Repo) UpdateUserListItemNote(ctx context.Context, listID, itemID int, note *string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE user_list_items SET note = $3 WHERE id = $2 AND list_id = $1`,
		listID, itemID, note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	r.touchUserList(ctx, listID)
	return nil
}

func (r *Repo) RemoveUserListItem(ctx context.Context, listID, itemID int) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM user_list_items WHERE id = $2 AND list_id = $1`, listID, itemID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	r.touchUserList(ctx, listID)
	return nil
}

// touchUserList bumps updated_at so "newest public lists" reflects item
// activity, not just metadata edits. Best-effort.
func (r *Repo) touchUserList(ctx context.Context, listID int) {
	_, _ = r.pool.Exec(ctx, `UPDATE user_lists SET updated_at = now() WHERE id = $1`, listID)
}
