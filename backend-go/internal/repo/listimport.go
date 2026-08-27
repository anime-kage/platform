package repo

// Bulk list import. Deliberately not built on UpsertWatchlist:
// that one logs a watch-history delta per call, and replaying somebody's
// ten-year backlog through it would both flood /istoric with a single day's
// worth of fake activity and cost thousands of round trips.

import (
	"context"

	"animekage/backend/internal/listimport"
)

// ImportList writes entries into watchlist or readlist, matching titles by
// MAL id. Existing rows are overwritten — an import is the member saying
// "this other list is the truth" — except for notes, which are only filled in
// when ours is empty, because reviews live in that column.
func (r *Repo) ImportList(ctx context.Context, userID int, entries []listimport.Entry, manga bool) (listimport.Result, error) {
	res := listimport.Result{Unmatched: []string{}}
	if len(entries) == 0 {
		return res, nil
	}

	table, idCol, progressCol, titleTable := "watchlist", "anime_id", "episodes_watched", "anime"
	if manga {
		table, idCol, progressCol, titleTable = "readlist", "manga_id", "chapters_read", "manga"
	}

	// one lookup for the whole list: mal_id → our id
	malIDs := make([]int, 0, len(entries))
	for _, e := range entries {
		malIDs = append(malIDs, e.MalID)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT mal_id, id FROM `+titleTable+` WHERE mal_id = ANY($1)`, malIDs)
	if err != nil {
		return res, err
	}
	known := make(map[int]int, len(entries))
	for rows.Next() {
		var malID, id int
		if err := rows.Scan(&malID, &id); err != nil {
			rows.Close()
			return res, err
		}
		known[malID] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx)

	// which rows already exist, so the result can distinguish "added" from
	// "updated" — the difference between a first import and a re-run
	existing := make(map[int]bool)
	exRows, err := tx.Query(ctx,
		`SELECT `+idCol+` FROM `+table+` WHERE user_id = $1`, userID)
	if err != nil {
		return res, err
	}
	for exRows.Next() {
		var id int
		if err := exRows.Scan(&id); err != nil {
			exRows.Close()
			return res, err
		}
		existing[id] = true
	}
	exRows.Close()

	stmt := `
		INSERT INTO ` + table + ` (user_id, ` + idCol + `, status, score, ` + progressCol + `,
		                           notes, started_at, completed_at, updated_at, from_import)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, now(), true)
		ON CONFLICT (user_id, ` + idCol + `) DO UPDATE SET
			status = excluded.status,
			score = excluded.score,
			` + progressCol + ` = excluded.` + progressCol + `,
			-- our own notes win: reviews live here (see PLAN 8.2), and an
			-- import must never overwrite one with a MAL comment
			notes = coalesce(` + table + `.notes, excluded.notes),
			started_at = coalesce(excluded.started_at, ` + table + `.started_at),
			completed_at = coalesce(excluded.completed_at, ` + table + `.completed_at),
			updated_at = now(),
			-- Re-importing over a row the member has since edited by hand puts
			-- it back under the import's authorship, which is what it now is.
			from_import = true`

	for _, e := range entries {
		id, ok := known[e.MalID]
		if !ok {
			res.Skipped++
			if len(res.Unmatched) < listimport.UnmatchedSample && e.Title != "" {
				res.Unmatched = append(res.Unmatched, e.Title)
			}
			continue
		}

		var score *int
		if e.Score > 0 {
			s := e.Score
			score = &s
		}
		if _, err := tx.Exec(ctx, stmt,
			userID, id, e.Status, score, e.Progress, e.Notes, e.StartedAt, e.CompletedAt); err != nil {
			return res, err
		}
		if existing[id] {
			res.Updated++
		} else {
			res.Imported++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	return res, nil
}
