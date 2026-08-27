package repo

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

// ── Watchlist ─────────────────────────────────────────────────────────────────

// WatchedFraction is how much of an episode counts as having seen it. It is
// the single source of truth for that threshold: the progress endpoint uses it
// to bump the watchlist, and ContinueWatching uses it to decide resume-vs-next.
// If those two ever disagreed, an episode could be "watched" and still offered
// as the thing to resume.
const WatchedFraction = 0.9

// playableEpisodes is every episode we can actually send someone to: it has a
// row here AND at least one enabled source. Both halves matter — the catalog
// importer creates episode rows for a whole aired season, so counting rows
// would promise episodes nobody has uploaded yet.
const playableEpisodes = `
	SELECT pe.id, pe.anime_id, pe.episode_number
	FROM episodes pe
	WHERE EXISTS (
		SELECT 1 FROM content_links cl
		WHERE cl.episode_id = pe.id AND cl.is_active
	)`

func (r *Repo) Watchlist(ctx context.Context, userID int, status string) ([]model.WatchlistEntry, error) {
	q := `
		SELECT w.id, w.user_id, w.anime_id, w.status, w.score, w.episodes_watched,
		       w.notes, w.started_at, w.completed_at, w.updated_at,
		       (SELECT count(*) FROM (` + playableEpisodes + `) p
		         WHERE p.anime_id = w.anime_id) AS available_episodes,
		       (SELECT min(p.episode_number) FROM (` + playableEpisodes + `) p
		         WHERE p.anime_id = w.anime_id
		           AND p.episode_number > w.episodes_watched) AS next_episode,
		       ` + animeCols("a", "anime.") + `
		FROM watchlist w
		JOIN anime a ON a.id = w.anime_id
		WHERE w.user_id = $1`
	args := []any{userID}
	if status != "" {
		q += ` AND w.status = $2`
		args = append(args, status)
	}
	q += ` ORDER BY w.updated_at DESC`

	rows := []model.WatchlistEntry{}
	if err := pgxscan.Select(ctx, r.pool, &rows, q, args...); err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Anime.Normalize()
	}
	return rows, nil
}

// ContinueWatching builds the home row: for every series you have playback on
// (or are actively watching), the one episode to open next.
//
// The rule is Netflix's, and it is entirely about the last episode you
// touched: if you did not finish it, you resume it at your position; if you
// did, you get the next one you have not seen. A series with nothing left to
// play drops out rather than showing a dead card.
func (r *Repo) ContinueWatching(ctx context.Context, userID, limit int) ([]model.ContinueEntry, error) {
	rows := []model.ContinueEntry{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		WITH playable AS (`+playableEpisodes+`
		),
		-- the episode most recently played per series, whatever its number:
		-- "where was I" is a question about time, not about episode order
		last_pos AS (
			SELECT DISTINCT ON (e.anime_id)
			       e.anime_id, p.episode_id, e.episode_number,
			       p.position_s, p.duration_s, p.updated_at
			FROM playback_positions p
			JOIN episodes e ON e.id = p.episode_id
			WHERE p.user_id = $1
			ORDER BY e.anime_id, p.updated_at DESC
		),
		cand AS (
			SELECT anime_id FROM last_pos
			UNION
			SELECT anime_id FROM watchlist WHERE user_id = $1 AND status = 'watching'
		)
		SELECT `+animeCols("a", "anime.")+`,
		       coalesce(rp.id, np.id)                         AS episode_id,
		       coalesce(rp.episode_number, np.episode_number)  AS episode_number,
		       CASE WHEN rp.id IS NOT NULL THEN lp.position_s ELSE 0 END AS position_s,
		       CASE WHEN rp.id IS NOT NULL THEN lp.duration_s ELSE NULL END AS duration_s,
		       (SELECT count(*) FROM playable pl WHERE pl.anime_id = c.anime_id) AS available_episodes,
		       coalesce(w.episodes_watched, 0) AS watched_episodes,
		       greatest(
		           coalesce(lp.updated_at, to_timestamp(0)),
		           -- watchlist.updated_at is a naive timestamp; say which zone
		           -- it is in rather than letting greatest() coerce silently
		           coalesce(w.updated_at AT TIME ZONE 'UTC', to_timestamp(0))
		       ) AS last_activity
		FROM cand c
		JOIN anime a ON a.id = c.anime_id
		LEFT JOIN last_pos lp ON lp.anime_id = c.anime_id
		LEFT JOIN watchlist w ON w.user_id = $1 AND w.anime_id = c.anime_id
		-- resume: the episode you stopped part-way through, if it still plays
		LEFT JOIN playable rp
		       ON rp.id = lp.episode_id
		      AND lp.duration_s IS NOT NULL AND lp.duration_s > 0
		      AND lp.position_s / lp.duration_s < $3
		-- otherwise the lowest published episode ahead of you
		LEFT JOIN LATERAL (
			SELECT pl.id, pl.episode_number
			FROM playable pl
			WHERE pl.anime_id = c.anime_id
			  AND pl.episode_number > greatest(
			        coalesce(w.episodes_watched, 0),
			        coalesce(lp.episode_number, 0))
			ORDER BY pl.episode_number
			LIMIT 1
		) np ON true
		WHERE coalesce(rp.id, np.id) IS NOT NULL
		ORDER BY last_activity DESC
		LIMIT $2`, userID, limit, WatchedFraction)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Anime.Normalize()
	}
	return rows, nil
}

func (r *Repo) WatchlistEntry(ctx context.Context, userID, animeID int) (*model.WatchlistEntry, error) {
	entries, err := r.Watchlist(ctx, userID, "")
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].AnimeID == animeID {
			return &entries[i], nil
		}
	}
	return nil, ErrNotFound
}

// ListUpsert carries the partial-merge semantics: nil pointer = leave the
// column untouched on conflict. This behavior is load-bearing — reviews live
// in notes, so a rating-only update must never wipe them.
type ListUpsert struct {
	Status   string
	Score    *int
	Progress *int // episodesWatched / chaptersRead
	Volumes  *int // readlist only
	Notes    *string
}

func (r *Repo) UpsertWatchlist(ctx context.Context, userID, animeID int, u ListUpsert) (*model.WatchlistEntry, error) {
	var totalEpisodes *int
	err := r.pool.QueryRow(ctx, `SELECT episodes FROM anime WHERE id = $1`, animeID).Scan(&totalEpisodes)
	if err != nil {
		return nil, wrapNotFound(err, fmt.Sprintf("anime with ID %d not found", animeID))
	}

	status := u.Status
	autoComplete := u.Progress != nil && totalEpisodes != nil && *u.Progress >= *totalEpisodes
	if autoComplete {
		status = "completed"
	}

	// previous progress: feeds the watch-history delta
	var prevProgress int
	_ = r.pool.QueryRow(ctx,
		`SELECT episodes_watched FROM watchlist WHERE user_id = $1 AND anime_id = $2`,
		userID, animeID).Scan(&prevProgress)

	_, err = r.pool.Exec(ctx, `
		INSERT INTO watchlist (user_id, anime_id, status, score, episodes_watched, notes, started_at, completed_at, updated_at)
		VALUES ($1, $2, $3::text, $4, coalesce($5, 0), $6,
		        CASE WHEN $3::text = 'watching' THEN now() END,
		        CASE WHEN $7 THEN now() END,
		        now())
		ON CONFLICT (user_id, anime_id) DO UPDATE SET
			status = excluded.status,
			-- $4 has three meanings: NULL = leave alone (the field was not
			-- sent), 0 = the member removed their rating, anything else = the
			-- new rating. Without the 0 case there is no way to un-rate.
			score = CASE WHEN $4::int = 0 THEN NULL
			             ELSE coalesce($4::int, watchlist.score) END,
			episodes_watched = coalesce($5, watchlist.episodes_watched),
			notes = coalesce($6, watchlist.notes),
			completed_at = CASE WHEN $7 THEN now() ELSE watchlist.completed_at END,
			updated_at = now(),
			-- A hand edit makes it the member's own again, so it belongs in
			-- the activity feed from here on.
			from_import = false`,
		userID, animeID, status, u.Score, u.Progress, u.Notes, autoComplete)
	if err != nil {
		return nil, err
	}

	if u.Progress != nil {
		if delta := *u.Progress - prevProgress; delta > 0 {
			if err := r.LogHistory(ctx, userID, &animeID, nil, delta); err != nil {
				return nil, err
			}
		}
	}
	return r.WatchlistEntry(ctx, userID, animeID)
}

func (r *Repo) RemoveWatchlist(ctx context.Context, userID, animeID int) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM watchlist WHERE user_id = $1 AND anime_id = $2`, userID, animeID)
	return tag.RowsAffected() > 0, err
}

// ── Readlist ──────────────────────────────────────────────────────────────────

func (r *Repo) Readlist(ctx context.Context, userID int, status string) ([]model.ReadlistEntry, error) {
	q := `
		SELECT rl.id, rl.user_id, rl.manga_id, rl.status, rl.score, rl.chapters_read,
		       rl.volumes_read, rl.notes, rl.started_at, rl.completed_at, rl.updated_at,
		       ` + mangaCols("m", "manga.") + `
		FROM readlist rl
		JOIN manga m ON m.id = rl.manga_id
		WHERE rl.user_id = $1`
	args := []any{userID}
	if status != "" {
		q += ` AND rl.status = $2`
		args = append(args, status)
	}
	q += ` ORDER BY rl.updated_at DESC`

	rows := []model.ReadlistEntry{}
	if err := pgxscan.Select(ctx, r.pool, &rows, q, args...); err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Manga.Normalize()
	}
	return rows, nil
}

func (r *Repo) ReadlistEntry(ctx context.Context, userID, mangaID int) (*model.ReadlistEntry, error) {
	entries, err := r.Readlist(ctx, userID, "")
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].MangaID == mangaID {
			return &entries[i], nil
		}
	}
	return nil, ErrNotFound
}

func (r *Repo) UpsertReadlist(ctx context.Context, userID, mangaID int, u ListUpsert) (*model.ReadlistEntry, error) {
	var totalChapters *int
	err := r.pool.QueryRow(ctx, `SELECT chapters FROM manga WHERE id = $1`, mangaID).Scan(&totalChapters)
	if err != nil {
		return nil, wrapNotFound(err, fmt.Sprintf("manga with ID %d not found", mangaID))
	}

	status := u.Status
	autoComplete := u.Progress != nil && totalChapters != nil && *u.Progress >= *totalChapters
	if autoComplete {
		status = "completed"
	}

	var prevProgress int
	_ = r.pool.QueryRow(ctx,
		`SELECT chapters_read FROM readlist WHERE user_id = $1 AND manga_id = $2`,
		userID, mangaID).Scan(&prevProgress)

	_, err = r.pool.Exec(ctx, `
		INSERT INTO readlist (user_id, manga_id, status, score, chapters_read, volumes_read, notes, started_at, completed_at, updated_at)
		VALUES ($1, $2, $3::text, $4, coalesce($5, 0), coalesce($6, 0), $7,
		        CASE WHEN $3::text = 'reading' THEN now() END,
		        CASE WHEN $8 THEN now() END,
		        now())
		ON CONFLICT (user_id, manga_id) DO UPDATE SET
			status = excluded.status,
			-- $4 has three meanings: NULL = leave alone (the field was not
			-- sent), 0 = the member removed their rating, anything else = the
			-- new rating. Without the 0 case there is no way to un-rate.
			score = CASE WHEN $4::int = 0 THEN NULL
			             ELSE coalesce($4::int, readlist.score) END,
			chapters_read = coalesce($5, readlist.chapters_read),
			volumes_read = coalesce($6, readlist.volumes_read),
			notes = coalesce($7, readlist.notes),
			completed_at = CASE WHEN $8 THEN now() ELSE readlist.completed_at END,
			updated_at = now(),
			from_import = false`,
		userID, mangaID, status, u.Score, u.Progress, u.Volumes, u.Notes, autoComplete)
	if err != nil {
		return nil, err
	}

	if u.Progress != nil {
		if delta := *u.Progress - prevProgress; delta > 0 {
			if err := r.LogHistory(ctx, userID, nil, &mangaID, delta); err != nil {
				return nil, err
			}
		}
	}
	return r.ReadlistEntry(ctx, userID, mangaID)
}

func (r *Repo) RemoveReadlist(ctx context.Context, userID, mangaID int) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM readlist WHERE user_id = $1 AND manga_id = $2`, userID, mangaID)
	return tag.RowsAffected() > 0, err
}

// ── Reviews (list entries with notes) ─────────────────────────────────────────

func (r *Repo) TitleReviews(ctx context.Context, kind string, titleID, limit int) ([]model.Review, error) {
	table, fk, commentFK := "watchlist", "anime_id", "watchlist_id"
	if kind == "manga" {
		table, fk, commentFK = "readlist", "manga_id", "readlist_id"
	}
	rows := []model.Review{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT l.id AS entry_id, l.user_id, l.score, l.notes, l.updated_at,
		       u.id AS "user.id", u.username AS "user.username", u.avatar_url AS "user.avatar_url",
		       (SELECT count(*)::int FROM comments c WHERE c.`+commentFK+` = l.id AND c.is_deleted = false) AS reply_count
		FROM `+table+` l
		JOIN users u ON u.id = l.user_id
		WHERE l.`+fk+` = $1 AND l.notes IS NOT NULL AND l.notes <> ''
		ORDER BY l.updated_at DESC
		LIMIT $2`, titleID, limit)
	return rows, err
}

// UserReviews merges a user's anime + manga reviews, newest first.
func (r *Repo) UserReviews(ctx context.Context, userID int) ([]model.UserReview, error) {
	out := []model.UserReview{}
	for _, side := range []struct{ kind, table, fk, titleTable, commentFK string }{
		{"anime", "watchlist", "anime_id", "anime", "watchlist_id"},
		{"manga", "readlist", "manga_id", "manga", "readlist_id"},
	} {
		var rows []model.UserReview
		err := pgxscan.Select(ctx, r.pool, &rows, `
			SELECT '`+side.kind+`' AS kind, l.id AS entry_id, l.score, l.notes, l.updated_at,
			       t.id AS "title.id", t.title AS "title.title",
			       t.title_romanian AS "title.title_romanian",
			       t.image_url AS "title.image_url", t.year AS "title.year",
			       (SELECT count(*)::int FROM comments c WHERE c.`+side.commentFK+` = l.id AND c.is_deleted = false) AS reply_count
			FROM `+side.table+` l
			JOIN `+side.titleTable+` t ON t.id = l.`+side.fk+`
			WHERE l.user_id = $1 AND l.notes IS NOT NULL AND l.notes <> ''`, userID)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// wrapNotFound converts pgx's no-rows error into ErrNotFound with context.
func wrapNotFound(err error, msg string) error {
	if err != nil && strings.Contains(err.Error(), "no rows") {
		return fmt.Errorf("%s: %w", msg, ErrNotFound)
	}
	return err
}

// AnimeWatcherIDs returns the members to notify when an episode of animeID goes
// live: those with it on their watchlist as 'watching' or 'plan-to-watch'.
//
// Deliberately NOT 'completed' or 'dropped' — those say "I am done with this",
// so a notification would be noise. Rows added by a list import do count: a
// member who imported their MAL list is expressing the same intent as one who
// used the button.
//
// Only ever called from the human publish path. Content imports write episodes
// and links straight to the database and never reach it, so importing a
// thousand series cannot notify anybody.
func (r *Repo) AnimeWatcherIDs(ctx context.Context, animeID int, exclude []int) ([]int, error) {
	skip := make(map[int]bool, len(exclude))
	for _, id := range exclude {
		skip[id] = true
	}
	var ids []int
	if err := pgxscan.Select(ctx, r.pool, &ids, `
		SELECT user_id FROM watchlist
		 WHERE anime_id = $1 AND status IN ('watching', 'plan-to-watch')`,
		animeID); err != nil {
		return nil, err
	}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if !skip[id] {
			out = append(out, id)
		}
	}
	return out, nil
}
