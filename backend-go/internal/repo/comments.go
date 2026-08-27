package repo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"

	"animekage/backend/internal/model"
)

// commentCols joined with the author; the old backend read avatarUrl from the
// JWT (which never carries it) — here it always comes from the users table.
const commentSelect = `
	SELECT c.id, c.user_id, c.anime_id, c.manga_id, c.episode_id, c.chapter_id,
	       c.parent_id, c.root_id, c.content, c.likes_count, c.dislikes_count,
	       c.replies_count, c.created_at, c.updated_at,
	       u.id AS "user.id", coalesce(u.username, 'Unknown') AS "user.username",
	       u.avatar_url AS "user.avatar_url"
	FROM comments c
	LEFT JOIN users u ON u.id = c.user_id`

var spaceRe = regexp.MustCompile(`\s+`)

// excerptOf is the short quote of the message a nested reply answers.
func excerptOf(text string) *string {
	t := strings.TrimSpace(spaceRe.ReplaceAllString(text, " "))
	if t == "" {
		return nil
	}
	if r := []rune(t); len(r) > 90 {
		t = string(r[:90]) + "…"
	}
	return &t
}

// CommentScope selects which thread a comment list belongs to. Exactly one of
// AnimeID/MangaID is set. No EpisodeID/ChapterID/ReviewID = series-wide
// discussion only. ReviewID is a watchlist entry id for anime, a readlist
// entry id for manga.
type CommentScope struct {
	AnimeID   *int
	MangaID   *int
	EpisodeID *int
	ChapterID *int
	ReviewID  *int
	// A news post's thread. Unlike the others this is a target on its own —
	// there is no parent title — so it short-circuits the switch below.
	AnnouncementID *int
}

func (s CommentScope) where(args *[]any) string {
	var conds []string
	add := func(cond string, v any) {
		*args = append(*args, v)
		conds = append(conds, fmt.Sprintf(cond, len(*args)))
	}
	// An announcement thread stands alone: no anime/manga parent to AND with,
	// and none of the episode/chapter/review narrowing below applies. Returns
	// the same " WHERE …" shape the tail of this function does.
	if s.AnnouncementID != nil {
		add("c.announcement_id = $%d", *s.AnnouncementID)
		conds = append(conds, "c.is_deleted = false", "c.parent_id IS NULL")
		return " WHERE " + strings.Join(conds, " AND ")
	}
	if s.AnimeID != nil {
		add("c.anime_id = $%d", *s.AnimeID)
	} else {
		add("c.manga_id = $%d", *s.MangaID)
	}
	conds = append(conds, "c.is_deleted = false", "c.parent_id IS NULL")

	switch {
	case s.ReviewID != nil && s.AnimeID != nil:
		add("c.watchlist_id = $%d", *s.ReviewID)
	case s.ReviewID != nil:
		add("c.readlist_id = $%d", *s.ReviewID)
	case s.EpisodeID != nil:
		add("c.episode_id = $%d", *s.EpisodeID)
	case s.ChapterID != nil:
		add("c.chapter_id = $%d", *s.ChapterID)
	default:
		conds = append(conds,
			"c.episode_id IS NULL", "c.chapter_id IS NULL",
			"c.watchlist_id IS NULL", "c.readlist_id IS NULL")
	}
	return " WHERE " + strings.Join(conds, " AND ")
}

// Comments returns top-level comments for a scope, newest first, plus total.
func (r *Repo) Comments(ctx context.Context, scope CommentScope, viewerID *int, page, limit int) ([]model.Comment, int, error) {
	var args []any
	where := scope.where(&args)

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM comments c`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := commentSelect + where + fmt.Sprintf(
		` ORDER BY c.created_at DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, limit, (page-1)*limit)

	rows := []model.Comment{}
	if err := pgxscan.Select(ctx, r.pool, &rows, q, args...); err != nil {
		return nil, 0, err
	}
	if err := r.attachViewerVotes(ctx, rows, viewerID); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Replies returns a comment's whole thread (all nested replies, flat, oldest
// first). replyToUsername/-Excerpt identify who a nested reply answers; the
// root's direct replies obviously answer the root, so they stay empty there.
func (r *Repo) Replies(ctx context.Context, rootID int, viewerID *int) ([]model.Comment, error) {
	type replyRow struct {
		model.Comment
		ParentContent  *string `db:"parent_content"`
		ParentUsername *string `db:"parent_username"`
	}
	raw := []replyRow{}
	err := pgxscan.Select(ctx, r.pool, &raw, `
		SELECT c.id, c.user_id, c.anime_id, c.manga_id, c.episode_id, c.chapter_id,
		       c.parent_id, c.root_id, c.content, c.likes_count, c.dislikes_count,
		       c.replies_count, c.created_at, c.updated_at,
		       u.id AS "user.id", coalesce(u.username, 'Unknown') AS "user.username",
		       u.avatar_url AS "user.avatar_url",
		       pc.content AS parent_content, pu.username AS parent_username
		FROM comments c
		LEFT JOIN users u ON u.id = c.user_id
		LEFT JOIN comments pc ON pc.id = c.parent_id
		LEFT JOIN users pu ON pu.id = pc.user_id
		WHERE c.root_id = $1 AND c.is_deleted = false
		ORDER BY c.created_at ASC`, rootID)
	if err != nil {
		return nil, err
	}

	rows := make([]model.Comment, len(raw))
	for i, rr := range raw {
		rows[i] = rr.Comment
		if rr.ParentID != nil && *rr.ParentID != rootID {
			rows[i].ReplyToUsername = rr.ParentUsername
			if rr.ParentContent != nil {
				rows[i].ReplyToExcerpt = excerptOf(*rr.ParentContent)
			}
		}
	}
	if err := r.attachViewerVotes(ctx, rows, viewerID); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) attachViewerVotes(ctx context.Context, rows []model.Comment, viewerID *int) error {
	if viewerID == nil || len(rows) == 0 {
		return nil
	}
	ids := make([]int, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	type vote struct {
		CommentID int    `db:"comment_id"`
		VoteType  string `db:"vote_type"`
	}
	votes := []vote{}
	err := pgxscan.Select(ctx, r.pool, &votes,
		`SELECT comment_id, vote_type FROM comment_votes
		 WHERE user_id = $1 AND comment_id = ANY($2)`, *viewerID, ids)
	if err != nil {
		return err
	}
	byID := make(map[int]string, len(votes))
	for _, v := range votes {
		byID[v.CommentID] = v.VoteType
	}
	for i := range rows {
		if vt, ok := byID[rows[i].ID]; ok {
			v := vt
			rows[i].UserVote = &v
		}
	}
	return nil
}

// ReviewBelongsToTitle checks that a review-thread comment points at a review
// of the title it's being posted under.
func (r *Repo) ReviewBelongsToTitle(ctx context.Context, kind string, reviewID, titleID int) (bool, error) {
	q := `SELECT EXISTS(SELECT 1 FROM watchlist WHERE id = $1 AND anime_id = $2)`
	if kind == "manga" {
		q = `SELECT EXISTS(SELECT 1 FROM readlist WHERE id = $1 AND manga_id = $2)`
	}
	var ok bool
	err := r.pool.QueryRow(ctx, q, reviewID, titleID).Scan(&ok)
	return ok, err
}

// CommentAuthorID returns the user who wrote a comment — used to address the
// reply notification. 0 (not found) is a valid, silent no-op for the caller.
func (r *Repo) CommentAuthorID(ctx context.Context, id int) (int, error) {
	var uid int
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM comments WHERE id = $1`, id).Scan(&uid)
	if pgxscan.NotFound(err) {
		return 0, nil
	}
	return uid, err
}

// CreateComment inserts a top-level comment and returns it with the author
// attached. kind is "anime" or "manga"; subID is an episode/chapter id.
func (r *Repo) CreateComment(ctx context.Context, kind string, userID, titleID int, subID, reviewID *int, content string) (*model.Comment, error) {
	titleCol, subCol, reviewCol := "anime_id", "episode_id", "watchlist_id"
	if kind == "manga" {
		titleCol, subCol, reviewCol = "manga_id", "chapter_id", "readlist_id"
	}
	if reviewID != nil {
		subID = nil // review threads don't carry an episode/chapter scope
	}

	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO comments (user_id, `+titleCol+`, `+subCol+`, `+reviewCol+`, content)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, titleID, subID, reviewID, content).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.commentByID(ctx, id)
}

// CreateAnnouncementComment starts a thread on a news post. Separate from
// CreateComment because that one is built around a title id plus an optional
// sub-scope, and a post has neither.
func (r *Repo) CreateAnnouncementComment(ctx context.Context, announcementID, userID int, content string) (*model.Comment, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO comments (user_id, announcement_id, content)
		VALUES ($1, $2, $3) RETURNING id`, userID, announcementID, content).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.commentByID(ctx, id)
}

// CreateReply inserts a reply under parentID, inheriting the parent's scope
// and joining the parent's thread; bumps the thread root's replies count.
func (r *Repo) CreateReply(ctx context.Context, parentID, userID int, content string) (*model.Comment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var (
		parent struct {
			AnimeID     *int `db:"anime_id"`
			MangaID     *int `db:"manga_id"`
			EpisodeID   *int `db:"episode_id"`
			ChapterID   *int `db:"chapter_id"`
			WatchlistID *int `db:"watchlist_id"`
			ReadlistID  *int `db:"readlist_id"`
			// without this a reply on a news post inherits no scope at all and
			// vanishes from the thread it was posted in
			AnnouncementID *int    `db:"announcement_id"`
			RootID         *int    `db:"root_id"`
			Content        string  `db:"content"`
			Username       *string `db:"username"`
		}
	)
	err = pgxscan.Get(ctx, tx, &parent, `
		SELECT c.anime_id, c.manga_id, c.episode_id, c.chapter_id,
		       c.watchlist_id, c.readlist_id, c.announcement_id, c.root_id,
		       c.content, u.username
		FROM comments c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.id = $1 AND c.is_deleted = false`, parentID)
	if pgxscan.NotFound(err) {
		return nil, fmt.Errorf("parent comment: %w", ErrNotFound)
	}
	if err != nil {
		return nil, err
	}

	rootID := parentID
	if parent.RootID != nil {
		rootID = *parent.RootID
	}

	var id int
	err = tx.QueryRow(ctx, `
		INSERT INTO comments (user_id, anime_id, manga_id, episode_id, chapter_id,
		                      watchlist_id, readlist_id, announcement_id,
		                      parent_id, root_id, content)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		userID, parent.AnimeID, parent.MangaID, parent.EpisodeID, parent.ChapterID,
		parent.WatchlistID, parent.ReadlistID, parent.AnnouncementID,
		parentID, rootID, content).Scan(&id)
	if err != nil {
		return nil, err
	}

	// repliesCount lives on the thread root = total thread size
	if _, err := tx.Exec(ctx,
		`UPDATE comments SET replies_count = replies_count + 1 WHERE id = $1`, rootID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	reply, err := r.commentByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rootID != parentID {
		reply.ReplyToUsername = parent.Username
		reply.ReplyToExcerpt = excerptOf(parent.Content)
	}
	return reply, nil
}

func (r *Repo) commentByID(ctx context.Context, id int) (*model.Comment, error) {
	var cm model.Comment
	err := pgxscan.Get(ctx, r.pool, &cm, commentSelect+` WHERE c.id = $1`, id)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &cm, nil
}

// UpdateComment edits the caller's own comment.
func (r *Repo) UpdateComment(ctx context.Context, id, userID int, content string) (*model.Comment, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE comments SET content = $3, updated_at = now()
		WHERE id = $1 AND user_id = $2 AND is_deleted = false`, id, userID, content)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.commentByID(ctx, id)
}

// SoftDeleteComment hides the caller's own comment and shrinks its thread.
func (r *Repo) SoftDeleteComment(ctx context.Context, id, userID int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var parentID, rootID *int
	err = tx.QueryRow(ctx,
		`SELECT parent_id, root_id FROM comments WHERE id = $1 AND user_id = $2`,
		id, userID).Scan(&parentID, &rootID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE comments SET is_deleted = true, updated_at = now() WHERE id = $1`, id); err != nil {
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

// Vote toggles/updates a like/dislike. Returns the message and resulting vote
// (nil when the vote was removed), matching the old API's response.
func (r *Repo) Vote(ctx context.Context, commentID, userID int, voteType string) (string, *string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx)

	counter := func(vt string) string {
		if vt == "like" {
			return "likes_count"
		}
		return "dislikes_count"
	}

	var existingID int
	var existingType string
	err = tx.QueryRow(ctx,
		`SELECT id, vote_type FROM comment_votes WHERE user_id = $1 AND comment_id = $2`,
		userID, commentID).Scan(&existingID, &existingType)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := tx.Exec(ctx,
			`INSERT INTO comment_votes (user_id, comment_id, vote_type) VALUES ($1, $2, $3)`,
			userID, commentID, voteType); err != nil {
			return "", nil, err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE comments SET `+counter(voteType)+` = `+counter(voteType)+` + 1 WHERE id = $1`,
			commentID); err != nil {
			return "", nil, err
		}
		return "Vote added", &voteType, tx.Commit(ctx)

	case err != nil:
		return "", nil, err

	case existingType == voteType: // toggle off
		if _, err := tx.Exec(ctx, `DELETE FROM comment_votes WHERE id = $1`, existingID); err != nil {
			return "", nil, err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE comments SET `+counter(voteType)+` = `+counter(voteType)+` - 1 WHERE id = $1`,
			commentID); err != nil {
			return "", nil, err
		}
		return "Vote removed", nil, tx.Commit(ctx)

	default: // switch vote direction
		if _, err := tx.Exec(ctx,
			`UPDATE comment_votes SET vote_type = $2 WHERE id = $1`, existingID, voteType); err != nil {
			return "", nil, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE comments SET `+counter(voteType)+` = `+counter(voteType)+` + 1,
			                    `+counter(existingType)+` = `+counter(existingType)+` - 1
			WHERE id = $1`, commentID); err != nil {
			return "", nil, err
		}
		return "Vote updated", &voteType, tx.Commit(ctx)
	}
}

func (r *Repo) ReportComment(ctx context.Context, id int) error {
	_, err := r.pool.Exec(ctx, `UPDATE comments SET is_reported = true WHERE id = $1`, id)
	return err
}
