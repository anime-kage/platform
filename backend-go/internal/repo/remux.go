package repo

// MKV→MP4 rewrap bookkeeping. See 0032_release_remux.sql for why
// this is its own state machine rather than a second mode of the hardsub one.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// RemuxJob is a claimed rewrap: which release, and where its MKV currently is.
type RemuxJob struct {
	ReleaseID   int
	StagingPath *string
	// R2Key is set when the source lives in the bucket; the rewrap then reads it
	// over https and writes the MP4 back under a new key.
	R2Key *string
}

// QueueRemux marks a release's video for rewrapping.
//
// Idempotent like QueueHardsub: re-queueing something already queued or running
// is a no-op, so a retried ingest cannot enqueue twice. Unlike hardsub, a 'done'
// row is *not* overwritten — the rewrap is a one-shot at ingest and the file it
// pointed at no longer exists, so re-running it would fail on a missing source.
func (r *Repo) QueueRemux(ctx context.Context, releaseID int) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE releases
		SET remux_state = 'queued',
		    remux_error = NULL,
		    remux_queued_at = now(),
		    remux_finished_at = NULL
		WHERE id = $1
		  AND medium = 'anime'
		  AND (staging_path IS NOT NULL OR r2_key IS NOT NULL)
		  AND (remux_state IS NULL OR remux_state = 'failed')`,
		releaseID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ClaimNextRemux takes the oldest queued rewrap and marks it running, atomically.
// Returns nil when the queue is empty.
func (r *Repo) ClaimNextRemux(ctx context.Context) (*RemuxJob, error) {
	var job RemuxJob
	err := r.pool.QueryRow(ctx, `
		UPDATE releases SET remux_state = 'running'
		WHERE id = (
			SELECT id FROM releases
			WHERE remux_state = 'queued'
			ORDER BY remux_queued_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, staging_path, r2_key`).Scan(&job.ReleaseID, &job.StagingPath, &job.R2Key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

// FinishRemux records a completed rewrap. The new location is written by the
// caller through SetReleaseR2Key / SetReleaseStagingPath before this is called,
// so that a crash between the two leaves the row pointing at a file that exists.
func (r *Repo) FinishRemux(ctx context.Context, releaseID int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE releases
		SET remux_state = 'done', remux_error = NULL, remux_finished_at = now()
		WHERE id = $1`, releaseID)
	return err
}

// FailRemux records why a rewrap did not happen. The message is shown to the
// translator who uploaded the file, so it should say what they can do about it.
func (r *Repo) FailRemux(ctx context.Context, releaseID int, reason string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE releases
		SET remux_state = 'failed', remux_error = $2, remux_finished_at = now()
		WHERE id = $1`, releaseID, reason)
	return err
}

// RequeueRunningRemuxes puts rewraps that were mid-flight back in the queue.
//
// Called once at startup, for the same reason as RequeueRunningHardsubs: a
// 'running' row with no worker behind it is a lie the UI would show forever. The
// partial MP4 is overwritten on the retry (-y) and the source MKV is only deleted
// after a successful swap, so there is nothing to clean up first.
func (r *Repo) RequeueRunningRemuxes(ctx context.Context) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE releases SET remux_state = 'queued'
		WHERE remux_state = 'running'`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// RemuxState is what the status endpoint reports from the database, before the
// worker's in-memory progress is merged in.
type RemuxState struct {
	State *string
	Error *string
	// Position is 1-based among queued jobs; 0 when not queued.
	Position int
}

func (r *Repo) RemuxStatus(ctx context.Context, releaseID int) (*RemuxState, error) {
	var s RemuxState
	err := r.pool.QueryRow(ctx, `
		SELECT r.remux_state, r.remux_error,
		       CASE WHEN r.remux_state = 'queued' THEN (
		         SELECT count(*) FROM releases q
		         WHERE q.remux_state = 'queued'
		           AND q.remux_queued_at <= r.remux_queued_at
		       ) ELSE 0 END AS position
		FROM releases r WHERE r.id = $1`, releaseID).
		Scan(&s.State, &s.Error, &s.Position)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}
