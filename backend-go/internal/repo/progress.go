package repo

// Playback progress: resume positions from our own player, plus
// the auto-mark-watched hook into the existing watchlist upsert (which owns
// the auto-complete and watch_history delta semantics — see lists.go).

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"
)

type PlaybackPosition struct {
	PositionS float64  `db:"position_s" json:"position"`
	DurationS *float64 `db:"duration_s" json:"duration,omitempty"`
}

func (r *Repo) PlaybackPosition(ctx context.Context, userID, episodeID int) (*PlaybackPosition, error) {
	var p PlaybackPosition
	err := pgxscan.Get(ctx, r.pool, &p,
		`SELECT position_s, duration_s FROM playback_positions
		 WHERE user_id = $1 AND episode_id = $2`, userID, episodeID)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repo) SavePlaybackPosition(ctx context.Context, userID, episodeID int, position float64, duration *float64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO playback_positions (user_id, episode_id, position_s, duration_s, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id, episode_id) DO UPDATE SET
			position_s = excluded.position_s,
			duration_s = coalesce(excluded.duration_s, playback_positions.duration_s),
			updated_at = now()`,
		userID, episodeID, position, duration)
	return err
}

// RecordEpisodeView counts one member opening one episode, for the home
// leaderboards. anime_id is read from the episode rather than trusted from the
// caller, so a client cannot attribute a view to a title it likes better.
//
// Idempotent by primary key: a re-watch, a refresh or switching source on the
// same episode all conflict and are dropped. Returns whether this was the first
// time — useful for tests, and it keeps the "once per user" rule visible at the
// call site rather than buried in the SQL.
//
// Deliberately separate from MarkEpisodeWatched below. Marking a series watched
// moves watchlist progress in bulk and must never mint views; that conflation is
// exactly what made the old leaderboards meaningless.
func (r *Repo) RecordEpisodeView(ctx context.Context, userID, episodeID int) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO episode_views (user_id, episode_id, anime_id)
		SELECT $1, e.id, e.anime_id FROM episodes e WHERE e.id = $2
		ON CONFLICT (user_id, episode_id) DO NOTHING`,
		userID, episodeID)
	if err != nil {
		return false, err
	}
	// Zero rows means either "already counted" or "no such episode". The SELECT
	// is what distinguishes them, and only the caller cares about the difference
	// for a 404, so check existence only in that case.
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := r.pool.QueryRow(ctx,
			`SELECT exists(SELECT 1 FROM episodes WHERE id = $1)`, episodeID).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, ErrNotFound
		}
	}
	return tag.RowsAffected() > 0, nil
}

// MarkEpisodeWatched bumps the watchlist to this episode number, creating the
// entry (as 'watching') if needed. It never moves progress backwards and never
// demotes a 'completed' entry (a rewatch is not a regression). Auto-complete
// and the watch_history delta come from UpsertWatchlist.
func (r *Repo) MarkEpisodeWatched(ctx context.Context, userID, episodeID int) (bool, error) {
	var ref struct {
		AnimeID       int `db:"anime_id"`
		EpisodeNumber int `db:"episode_number"`
	}
	err := pgxscan.Get(ctx, r.pool, &ref,
		`SELECT anime_id, episode_number FROM episodes WHERE id = $1`, episodeID)
	if pgxscan.NotFound(err) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}

	status := "watching"
	var prevProgress int
	var prevStatus *string
	_ = r.pool.QueryRow(ctx,
		`SELECT episodes_watched, status FROM watchlist WHERE user_id = $1 AND anime_id = $2`,
		userID, ref.AnimeID).Scan(&prevProgress, &prevStatus)
	if prevStatus != nil {
		if *prevStatus == "completed" || ref.EpisodeNumber <= prevProgress {
			return false, nil
		}
		status = *prevStatus
	}

	_, err = r.UpsertWatchlist(ctx, userID, ref.AnimeID, ListUpsert{
		Status: status, Progress: &ref.EpisodeNumber,
	})
	return err == nil, err
}
