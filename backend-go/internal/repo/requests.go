package repo

// Translation requests ("Cereri") — members ask for a series to be subtitled
// and vote on the queue. Deduped by canonical MAL id (resolved at the handler
// via Jikan/AniList) so the same series can't be requested twice.

import (
	"context"
	"errors"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"

	"animekage/backend/internal/model"
)

// requestCols selects a request row plus its requester name, vote tally, and
// whether `viewerParam` (a SQL placeholder, or "0" for a guest) has voted.
func requestCols(viewerParam string) string {
	return `
		tr.id, tr.user_id, tr.medium, tr.mal_id, tr.title, tr.image_url, tr.note,
		tr.status, tr.created_at, tr.updated_at,
		u.username AS requester_name,
		(SELECT count(*) FROM request_votes rv WHERE rv.request_id = tr.id) AS vote_count,
		EXISTS(SELECT 1 FROM request_votes rv WHERE rv.request_id = tr.id AND rv.user_id = ` + viewerParam + `) AS voted`
}

// ListRequests returns a page of requests. status "" = all; sort "votes" ranks
// by tally, anything else by recency. Returns the page plus the full count.
func (r *Repo) ListRequests(ctx context.Context, viewerID int, status, sort string, limit, offset int) ([]model.TranslationRequest, int, error) {
	order := "vote_count DESC, tr.created_at DESC"
	if sort == "recent" {
		order = "tr.created_at DESC"
	}
	rows := []model.TranslationRequest{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+requestCols("$1")+`
		FROM translation_requests tr JOIN users u ON u.id = tr.user_id
		WHERE ($2 = '' OR tr.status = $2)
		ORDER BY `+order+`
		LIMIT $3 OFFSET $4`, viewerID, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	var total int
	err = r.pool.QueryRow(ctx,
		`SELECT count(*) FROM translation_requests WHERE ($1 = '' OR status = $1)`, status).Scan(&total)
	return rows, total, err
}

func (r *Repo) RequestByID(ctx context.Context, id, viewerID int) (*model.TranslationRequest, error) {
	var tr model.TranslationRequest
	err := pgxscan.Get(ctx, r.pool, &tr, `
		SELECT `+requestCols("$2")+`
		FROM translation_requests tr JOIN users u ON u.id = tr.user_id
		WHERE tr.id = $1`, id, viewerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &tr, err
}

// FindRequest locates an existing request to merge into: by canonical MAL id
// when resolved, else by case-insensitive title. Returns ErrNotFound if none.
func (r *Repo) FindRequest(ctx context.Context, medium string, malID *int, title string) (int, error) {
	var id int
	var err error
	if malID != nil {
		err = r.pool.QueryRow(ctx,
			`SELECT id FROM translation_requests WHERE medium = $1 AND mal_id = $2`, medium, *malID).Scan(&id)
	} else {
		err = r.pool.QueryRow(ctx,
			`SELECT id FROM translation_requests WHERE medium = $1 AND mal_id IS NULL AND lower(title) = lower($2)`,
			medium, title).Scan(&id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// CreateRequest inserts a request. The caller has already deduped via
// FindRequest; a race that slips past still trips the unique indexes → ErrExists.
func (r *Repo) CreateRequest(ctx context.Context, userID int, medium string, malID *int, title string, imageURL, note *string) (int, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO translation_requests (user_id, medium, mal_id, title, image_url, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`, userID, medium, malID, title, imageURL, note).Scan(&id)
	if IsUniqueViolation(err) {
		return 0, ErrExists
	}
	return id, err
}

// VoteRequest records a vote (idempotent) and returns the new tally.
func (r *Repo) VoteRequest(ctx context.Context, reqID, userID int) (int, error) {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO request_votes (request_id, user_id) VALUES ($1, $2)
		 ON CONFLICT (request_id, user_id) DO NOTHING`, reqID, userID); err != nil {
		return 0, err
	}
	return r.requestVoteCount(ctx, reqID)
}

func (r *Repo) UnvoteRequest(ctx context.Context, reqID, userID int) (int, error) {
	if _, err := r.pool.Exec(ctx,
		`DELETE FROM request_votes WHERE request_id = $1 AND user_id = $2`, reqID, userID); err != nil {
		return 0, err
	}
	return r.requestVoteCount(ctx, reqID)
}

func (r *Repo) requestVoteCount(ctx context.Context, reqID int) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM request_votes WHERE request_id = $1`, reqID).Scan(&n)
	return n, err
}

// SetRequestStatus moves a request through the queue (coordinator/admin action).
func (r *Repo) SetRequestStatus(ctx context.Context, id int, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE translation_requests SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
