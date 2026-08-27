package repo

// Release pipeline data layer. State transitions are enforced in
// SetReleaseState with an expected-from list so two concurrent reviewers can't
// double-apply; everything else is plain CRUD.

import (
	"context"
	"errors"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"

	"animekage/backend/internal/model"
)

const releaseCols = `r.id, r.medium, r.anime_id, r.manga_id, r.proposed_title, r.episode_number,
	r.chapter_number, r.uploader_id, r.reviewer_id,
	r.assigned_verifier_id, r.state, r.staging_path, r.r2_key, r.review_notes, r.created_at, r.updated_at,
	a.title AS anime_title, a.image_url AS anime_image,
	m.title AS manga_title, m.image_url AS manga_image, u.username AS uploader_name,
	rev.username AS reviewer_name, av.username AS assigned_verifier_name,
	(SELECT count(*) FROM subtitle_events se WHERE se.release_id = r.id) AS total_events,
	(SELECT count(*) FROM subtitle_events se WHERE se.release_id = r.id AND se.ro_text <> '') AS done_events`

// anime/manga are LEFT JOINs: a release may name a series that isn't in the
// catalog yet (proposed_title) — the coordinator links it at publish time.
const releaseFrom = ` FROM releases r
	LEFT JOIN anime a ON a.id = r.anime_id
	LEFT JOIN manga m ON m.id = r.manga_id
	JOIN users u ON u.id = r.uploader_id
	LEFT JOIN users rev ON rev.id = r.reviewer_id
	LEFT JOIN users av ON av.id = r.assigned_verifier_id`

// CreateReleaseInput carries the medium-specific target: episode number for
// anime, chapter number for manga (the DB CHECKs enforce consistency).
type CreateReleaseInput struct {
	Medium        string // 'anime' | 'manga'
	AnimeID       *int
	MangaID       *int
	ProposedTitle *string
	EpisodeNumber *int
	ChapterNumber *float64
	UploaderID    int
	VerifierID    *int
}

// CreateRelease inserts a release routed to VerifierID; when nil it falls
// back to the uploader's last pick (users.last_verifier_id) so the common
// "same verifier as always" case needs no input.
func (r *Repo) CreateRelease(ctx context.Context, in CreateReleaseInput) (*model.Release, error) {
	var rel model.Release
	err := pgxscan.Get(ctx, r.pool, &rel, `
		WITH ins AS (
			INSERT INTO releases (medium, anime_id, manga_id, proposed_title, episode_number,
			                      chapter_number, uploader_id, assigned_verifier_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7,
			        coalesce($8, (SELECT last_verifier_id FROM users WHERE id = $7)))
			RETURNING *
		)
		SELECT `+releaseCols+` FROM ins r
		LEFT JOIN anime a ON a.id = r.anime_id
		LEFT JOIN manga m ON m.id = r.manga_id
		JOIN users u ON u.id = r.uploader_id
		LEFT JOIN users rev ON rev.id = r.reviewer_id
		LEFT JOIN users av ON av.id = r.assigned_verifier_id`,
		in.Medium, in.AnimeID, in.MangaID, in.ProposedTitle, in.EpisodeNumber,
		in.ChapterNumber, in.UploaderID, in.VerifierID)
	if err != nil {
		return nil, err
	}
	return &rel, nil
}

// SetReleaseTarget is the coordinator's publish-time mapping: pin the release
// to a real catalog series (clearing any proposed title) and the confirmed
// episode number.
func (r *Repo) SetReleaseTarget(ctx context.Context, id, animeID, episodeNumber int) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE releases SET
			anime_id = $2, proposed_title = NULL, episode_number = $3, updated_at = now()
		WHERE id = $1`, id, animeID, episodeNumber)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetReleaseTargetManga is the manga twin: pin to a catalog manga + chapter.
func (r *Repo) SetReleaseTargetManga(ctx context.Context, id, mangaID int, chapterNumber float64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE releases SET
			manga_id = $2, proposed_title = NULL, chapter_number = $3, updated_at = now()
		WHERE id = $1`, id, mangaID, chapterNumber)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) ReleaseByID(ctx context.Context, id int) (*model.Release, error) {
	var rel model.Release
	err := pgxscan.Get(ctx, r.pool, &rel,
		`SELECT `+releaseCols+releaseFrom+` WHERE r.id = $1`, id)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rel, nil
}

// Releases lists newest-first, optionally filtered by state and/or uploader.
// The verify queue is state='in_review'; a translator's dashboard is
// uploaderID=me. assignedTo filters to a verifier's working queue: releases
// routed to them plus unrouted ones (nobody's work should be invisible).
func (r *Repo) Releases(ctx context.Context, state string, uploaderID, assignedTo *int) ([]model.Release, error) {
	rels := []model.Release{}
	err := pgxscan.Select(ctx, r.pool, &rels, `
		SELECT `+releaseCols+releaseFrom+`
		WHERE ($1 = '' OR r.state = $1)
		  AND ($2::int IS NULL OR r.uploader_id = $2)
		  AND ($3::int IS NULL OR r.assigned_verifier_id = $3 OR r.assigned_verifier_id IS NULL)
		ORDER BY r.updated_at DESC`, state, uploaderID, assignedTo)
	return rels, err
}

// AssignVerifier reroutes a release (nil clears the assignment).
func (r *Repo) AssignVerifier(ctx context.Context, releaseID int, verifierID *int) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE releases SET assigned_verifier_id = $2, updated_at = now() WHERE id = $1`,
		releaseID, verifierID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// VerifierOption is one person a release can be routed to.
type VerifierOption struct {
	ID       int    `db:"id" json:"id"`
	Username string `db:"username" json:"username"`
	Role     string `db:"role" json:"role"`
}

// Verifiers lists the assignable people: verifier-role users plus
// coordinators and admins (the owner's spec), banned excluded.
func (r *Repo) Verifiers(ctx context.Context) ([]VerifierOption, error) {
	rows := []VerifierOption{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT id, username, role FROM users
		WHERE role IN ('verifier', 'coordinator', 'admin') AND is_banned = false
		ORDER BY array_position(ARRAY['verifier', 'coordinator', 'admin'], role), username`)
	return rows, err
}

// LastVerifierID is the user's most recent explicit pick (the UI default).
func (r *Repo) LastVerifierID(ctx context.Context, userID int) (*int, error) {
	var id *int
	err := r.pool.QueryRow(ctx,
		`SELECT last_verifier_id FROM users WHERE id = $1`, userID).Scan(&id)
	return id, err
}

func (r *Repo) SetLastVerifier(ctx context.Context, userID, verifierID int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET last_verifier_id = $2 WHERE id = $1`, userID, verifierID)
	return err
}

// SetReleaseState moves a release along the state machine, but only when its
// current state is in `from` — a stale transition returns ErrNotFound.
//
// actorID is the person making the move; it is recorded as the publisher only
// on the transition to 'published', which is the one step whose
// actor is not already captured by uploader_id or reviewer_id.
func (r *Repo) SetReleaseState(ctx context.Context, id int, from []string, to string, reviewerID *int, notes *string, actorID ...int) error {
	var publisher *int
	if to == "published" && len(actorID) > 0 {
		publisher = &actorID[0]
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE releases SET
			state        = $3,
			reviewer_id  = coalesce($4, reviewer_id),
			review_notes = coalesce($5, review_notes),
			published_by = coalesce($6, published_by),
			published_at = CASE WHEN $3 = 'published' AND published_at IS NULL
			                    THEN now() ELSE published_at END,
			updated_at   = now()
		WHERE id = $1 AND state = ANY($2)`, id, from, to, reviewerID, notes, publisher)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetReleaseStagingPath records where the uploaded video landed (relative to
// STAGING_DIR); nil marks it cleaned up.
func (r *Repo) SetReleaseStagingPath(ctx context.Context, id int, path *string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE releases SET staging_path = $2, updated_at = now() WHERE id = $1`, id, path)
	return err
}

// SetReleaseR2Key records that the source video lives in the bucket rather than
// on local disk. Exactly one of r2_key / staging_path should be set.
func (r *Repo) SetReleaseR2Key(ctx context.Context, id int, key *string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE releases SET r2_key = $2, updated_at = now() WHERE id = $1`, id, key)
	return err
}

// ReleasesWithR2Objects lists releases still holding a bucket object, for the
// purge and the janitor — the bucket twin of PublishedWithStaging.
func (r *Repo) ReleasesWithR2Objects(ctx context.Context, publishedBefore time.Time) ([]Release2Key, error) {
	rows := []Release2Key{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT id, r2_key FROM releases
		WHERE r2_key IS NOT NULL AND state = 'published' AND updated_at < $1`, publishedBefore)
	return rows, err
}

// Release2Key is the minimal shape the purge needs.
type Release2Key struct {
	ID    int    `db:"id"`
	R2Key string `db:"r2_key"`
}

// DeleteRelease removes the row (events cascade) and returns the staging path
// so the caller can delete the files.
func (r *Repo) DeleteRelease(ctx context.Context, id int) (stagingPath *string, err error) {
	err = r.pool.QueryRow(ctx,
		`DELETE FROM releases WHERE id = $1 RETURNING staging_path`, id).Scan(&stagingPath)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return stagingPath, err
}

// StaleReleases returns unfinished releases untouched since the cutoff — the
// 30-day staging auto-expiry (architecture §3: staging is strictly transient).
func (r *Repo) StaleReleases(ctx context.Context, cutoff time.Time) ([]model.Release, error) {
	rels := []model.Release{}
	err := pgxscan.Select(ctx, r.pool, &rels, `
		SELECT `+releaseCols+releaseFrom+`
		WHERE r.state IN ('draft', 'changes_requested') AND r.updated_at < $1`, cutoff)
	return rels, err
}

// UnpublishedAnimeReleases counts what one translator has in flight, plus their
// personal cap override (nil = use the global default). One query, because the
// upload gate needs both on every request.
//
// Anime only: a staged episode is gigabytes, a staged chapter is a handful of
// page images. The cap exists to bound disk, and manga does not threaten it.
//
// Every unpublished state counts, not just the ones the translator can still
// act on — an approved release waiting on a coordinator occupies exactly as
// much disk as a draft.
func (r *Repo) UnpublishedAnimeReleases(ctx context.Context, uploaderID int) (n int, cap *int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM releases
		         WHERE uploader_id = $1 AND medium = 'anime' AND state <> 'published'),
		       (SELECT release_cap FROM users WHERE id = $1)`,
		uploaderID).Scan(&n, &cap)
	return n, cap, err
}

// SetReleaseCap overrides one user's in-flight cap; nil restores the default.
func (r *Repo) SetReleaseCap(ctx context.Context, userID int, cap *int) error {
	tag, err := r.pool.Exec(ctx, `UPDATE users SET release_cap = $2 WHERE id = $1`, userID, cap)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// PublishedWithStaging returns published releases whose staged video is still
// on disk and that went live before the cutoff. Once an episode is published
// the file is on a host and the subtitle is in Postgres, so the local copy is
// only a re-upload convenience — it must not live forever.
//
// The release row itself stays: credits and the hall of fame are derived from
// it. Only the bytes go.
func (r *Repo) PublishedWithStaging(ctx context.Context, cutoff time.Time) ([]model.Release, error) {
	rels := []model.Release{}
	err := pgxscan.Select(ctx, r.pool, &rels, `
		SELECT `+releaseCols+releaseFrom+`
		WHERE r.state = 'published' AND r.updated_at < $1
		  AND (r.staging_path IS NOT NULL OR r.r2_key IS NOT NULL)`, cutoff)
	return rels, err
}

// ClearStagingPath forgets a release's staged file after the bytes are gone,
// so hasVideo stops advertising a download that would 404.
//
// Deliberately not SetReleaseStagingPath(id, nil): that bumps updated_at, and
// on a published release updated_at *is* the publish time — it drives the hall
// of fame's monthly window and the "published recently" ordering. A janitor
// must not rewrite when something went live.
func (r *Repo) ClearStagingPath(ctx context.Context, id int) error {
	_, err := r.pool.Exec(ctx, `UPDATE releases SET staging_path = NULL WHERE id = $1`, id)
	return err
}

// ── Credits ─────────────────────────────────────────────────────────────────

// EpisodeCredits returns the translator (uploader) and verifier (reviewer)
// behind the latest published release for an anime episode. Nil fields when
// nothing is published there (or the reviewer wasn't recorded).
func (r *Repo) EpisodeCredits(ctx context.Context, animeID, episodeNumber int) (*model.ReleaseCredits, error) {
	return r.releaseCredits(ctx,
		`r.medium = 'anime' AND r.anime_id = $1 AND r.episode_number = $2`, animeID, episodeNumber)
}

// ChapterCredits is the manga twin of EpisodeCredits.
func (r *Repo) ChapterCredits(ctx context.Context, mangaID int, chapterNumber float64) (*model.ReleaseCredits, error) {
	return r.releaseCredits(ctx,
		`r.medium = 'manga' AND r.manga_id = $1 AND r.chapter_number = $2`, mangaID, chapterNumber)
}

func (r *Repo) releaseCredits(ctx context.Context, where string, args ...any) (*model.ReleaseCredits, error) {
	var row struct {
		TID   int     `db:"t_id"`
		TName string  `db:"t_name"`
		TAv   *string `db:"t_av"`
		VID   *int    `db:"v_id"`
		VName *string `db:"v_name"`
		VAv   *string `db:"v_av"`
		PID   *int    `db:"p_id"`
		PName *string `db:"p_name"`
		PAv   *string `db:"p_av"`
	}
	err := pgxscan.Get(ctx, r.pool, &row, `
		SELECT tu.id AS t_id, tu.username AS t_name, tu.avatar_url AS t_av,
		       vu.id AS v_id, vu.username AS v_name, vu.avatar_url AS v_av,
		       pu.id AS p_id, pu.username AS p_name, pu.avatar_url AS p_av
		FROM releases r
		JOIN users tu ON tu.id = r.uploader_id
		LEFT JOIN users vu ON vu.id = r.reviewer_id
		LEFT JOIN users pu ON pu.id = r.published_by
		WHERE r.state = 'published' AND `+where+`
		ORDER BY r.updated_at DESC LIMIT 1`, args...)
	if pgxscan.NotFound(err) {
		return &model.ReleaseCredits{}, nil // nothing published there yet
	}
	if err != nil {
		return nil, err
	}
	c := &model.ReleaseCredits{
		Translator: &model.CommentUser{ID: row.TID, Username: row.TName, AvatarURL: row.TAv},
	}
	if row.VID != nil {
		c.Verifier = &model.CommentUser{ID: *row.VID, Username: *row.VName, AvatarURL: row.VAv}
	}
	if row.PID != nil {
		c.Coordinator = &model.CommentUser{ID: *row.PID, Username: *row.PName, AvatarURL: row.PAv}
	}
	return c, nil
}

// ── Subtitle events ───────────────────────────────────────────────────────────

type EventInput struct {
	Idx     int
	StartMs int
	EndMs   int
	EnText  string
	RoText  string
	Edited  bool
}

// ReplaceEvents swaps a release's entire event set (initial parse, or
// re-upload of a sub file) in one transaction.
func (r *Repo) ReplaceEvents(ctx context.Context, releaseID int, events []EventInput) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM subtitle_events WHERE release_id = $1`, releaseID); err != nil {
		return err
	}
	rows := make([][]any, len(events))
	for i, e := range events {
		rows[i] = []any{releaseID, e.Idx, e.StartMs, e.EndMs, e.EnText, e.RoText, e.Edited}
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"subtitle_events"},
		[]string{"release_id", "idx", "start_ms", "end_ms", "en_text", "ro_text", "edited"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repo) EventsByRelease(ctx context.Context, releaseID int) ([]model.SubtitleEvent, error) {
	events := []model.SubtitleEvent{}
	err := pgxscan.Select(ctx, r.pool, &events, `
		SELECT id, release_id, idx, start_ms, end_ms, en_text, ro_text, edited
		FROM subtitle_events WHERE release_id = $1 ORDER BY idx`, releaseID)
	return events, err
}

// UpdateEventRO is the editor's per-row autosave: sets the RO text and marks
// the row human-edited.
func (r *Repo) UpdateEventRO(ctx context.Context, releaseID, idx int, roText string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE subtitle_events SET ro_text = $3, edited = true
		WHERE release_id = $1 AND idx = $2`, releaseID, idx, roText)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FillEventRO writes machine translation into a row ONLY if its ro_text is
// still empty — a human edit that landed while the model was
// running always wins. edited stays false: machine lines are reviewable.
func (r *Repo) FillEventRO(ctx context.Context, releaseID, idx int, roText string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE subtitle_events SET ro_text = $3
		WHERE release_id = $1 AND idx = $2 AND ro_text = ''`, releaseID, idx, roText)
	return err
}

// TranslationContext returns the anime title + per-series glossary for the
// auto-translate prompt prefix.
func (r *Repo) TranslationContext(ctx context.Context, animeID int) (title string, glossary *string, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT title, translation_glossary FROM anime WHERE id = $1`, animeID).Scan(&title, &glossary)
	return title, glossary, err
}

// TranslatedEventCount reports how many rows have RO text — the submit guard
// (an all-empty draft can't go to review).
func (r *Repo) TranslatedEventCount(ctx context.Context, releaseID int) (translated, total int, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE ro_text <> ''), count(*)
		FROM subtitle_events WHERE release_id = $1`, releaseID).Scan(&translated, &total)
	return translated, total, err
}
