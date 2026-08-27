package repo

// Hardsub job bookkeeping. See 0030_hardsub.sql for why this lives on
// the release row and why progress does not live here at all.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// HardsubJob is a claimed unit of work: which release, and where its video and
// staging directory are.
type HardsubJob struct {
	ReleaseID   int     `db:"id"`
	StagingPath *string `db:"staging_path"`
	// R2Key is set when the source video lives in the bucket; the burn then
	// reads it over https instead of from disk.
	R2Key *string `db:"r2_key"`
}

// QueueHardsub marks a release for burning.
//
// Idempotent by design: queueing something already queued or running is a no-op
// rather than an error, so a double-click cannot enqueue twice. A previous
// 'done' or 'failed' *is* overwritten — that is a deliberate re-burn, which is
// how a restyle or a corrected subtitle gets applied.
func (r *Repo) QueueHardsub(ctx context.Context, releaseID int) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE releases
		SET hardsub_state = 'queued',
		    hardsub_error = NULL,
		    hardsub_queued_at = now(),
		    hardsub_finished_at = NULL
		WHERE id = $1
		  AND medium = 'anime'
		  AND (staging_path IS NOT NULL OR r2_key IS NOT NULL)
		  AND (hardsub_state IS NULL OR hardsub_state IN ('done', 'failed'))`,
		releaseID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ClaimNextHardsub takes the oldest queued job and marks it running, atomically.
//
// The UPDATE ... WHERE id = (SELECT ... FOR UPDATE SKIP LOCKED) shape is what
// makes this safe if a second worker ever exists — today concurrency is one, but
// writing the claim correctly now costs nothing and removes a landmine.
// Returns nil when the queue is empty.
func (r *Repo) ClaimNextHardsub(ctx context.Context) (*HardsubJob, error) {
	var job HardsubJob
	err := r.pool.QueryRow(ctx, `
		UPDATE releases SET hardsub_state = 'running'
		WHERE id = (
			SELECT id FROM releases
			WHERE hardsub_state = 'queued'
			ORDER BY hardsub_queued_at
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

// FinishHardsub records a completed burn and where the artefact landed.
func (r *Repo) FinishHardsub(ctx context.Context, releaseID int, path string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE releases
		SET hardsub_state = 'done', hardsub_path = $2,
		    hardsub_error = NULL, hardsub_finished_at = now()
		WHERE id = $1`, releaseID, path)
	return err
}

// FailHardsub records why a burn did not happen. The message is shown to a
// coordinator, so callers should pass something a human can act on.
func (r *Repo) FailHardsub(ctx context.Context, releaseID int, reason string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE releases
		SET hardsub_state = 'failed', hardsub_error = $2, hardsub_finished_at = now()
		WHERE id = $1`, releaseID, reason)
	return err
}

// ClearHardsub returns a release to "never burned" — used when a coordinator
// stops a job. Deliberately also clears hardsub_path: a stopped burn leaves no
// usable artefact, and a path pointing at a file that was deleted would show a
// download button that 404s.
func (r *Repo) ClearHardsub(ctx context.Context, releaseID int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE releases
		SET hardsub_state = NULL, hardsub_path = NULL, hardsub_error = NULL,
		    hardsub_queued_at = NULL, hardsub_finished_at = NULL
		WHERE id = $1`, releaseID)
	return err
}

// RequeueRunningHardsubs puts jobs that were mid-burn back in the queue.
//
// Called once at startup. A 'running' row with no worker behind it is a lie the
// publish page would show forever — the process that owned it died with the
// container. ffmpeg's partial output is overwritten on the retry (-y), so there
// is nothing to clean up first.
func (r *Repo) RequeueRunningHardsubs(ctx context.Context) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE releases SET hardsub_state = 'queued'
		WHERE hardsub_state = 'running'`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// HardsubState is what the status endpoint reports from the database, before the
// worker's in-memory progress is merged in.
type HardsubState struct {
	State      *string    `db:"hardsub_state"`
	Path       *string    `db:"hardsub_path"`
	Error      *string    `db:"hardsub_error"`
	QueuedAt   *time.Time `db:"hardsub_queued_at"`
	FinishedAt *time.Time `db:"hardsub_finished_at"`
	// Position is 1-based among queued jobs; 0 when not queued. Lets the UI say
	// "3rd in line" instead of an indefinite spinner.
	Position int `db:"position"`
}

func (r *Repo) HardsubStatus(ctx context.Context, releaseID int) (*HardsubState, error) {
	var s HardsubState
	err := r.pool.QueryRow(ctx, `
		SELECT r.hardsub_state, r.hardsub_path, r.hardsub_error,
		       r.hardsub_queued_at, r.hardsub_finished_at,
		       CASE WHEN r.hardsub_state = 'queued' THEN (
		         SELECT count(*) FROM releases q
		         WHERE q.hardsub_state = 'queued'
		           AND q.hardsub_queued_at <= r.hardsub_queued_at
		       ) ELSE 0 END AS position
		FROM releases r WHERE r.id = $1`, releaseID).
		Scan(&s.State, &s.Path, &s.Error, &s.QueuedAt, &s.FinishedAt, &s.Position)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}
