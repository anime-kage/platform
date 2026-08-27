package repo

// Script-support queries for cmd/populate and cmd/autoupdate.

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"
)

type AiringTitle struct {
	ID     int    `db:"id"`
	MalID  *int   `db:"mal_id"`
	Title  string `db:"title"`
	Status string `db:"status"`
}

// AiringAnime lists the anime worth polling Jikan for: currently airing plus
// upcoming (so premieres flip to airing on refresh without manual edits).
func (r *Repo) AiringAnime(ctx context.Context) ([]AiringTitle, error) {
	var out []AiringTitle
	err := pgxscan.Select(ctx, r.pool, &out,
		`SELECT id, mal_id, title, status FROM anime
		 WHERE status IN ('airing', 'upcoming') ORDER BY id`)
	return out, err
}

// AllAnimeWithMalID is every catalog entry we could ask MAL about — the input
// to the manual episode-metadata backfill. Unlike AiringAnime it does not filter
// on status, which is the whole point: a completed series is exactly the case
// the nightly job cannot reach.
func (r *Repo) AllAnimeWithMalID(ctx context.Context) ([]AiringTitle, error) {
	var out []AiringTitle
	err := pgxscan.Select(ctx, r.pool, &out,
		`SELECT id, mal_id, title, status FROM anime
		 WHERE mal_id IS NOT NULL AND mal_id > 0 ORDER BY id`)
	return out, err
}

// EpisodeNumbers returns the set of episode numbers already stored.
func (r *Repo) EpisodeNumbers(ctx context.Context, animeID int) (map[int]bool, error) {
	var nums []int
	if err := pgxscan.Select(ctx, r.pool, &nums,
		`SELECT episode_number FROM episodes WHERE anime_id = $1`, animeID); err != nil {
		return nil, err
	}
	set := make(map[int]bool, len(nums))
	for _, n := range nums {
		set[n] = true
	}
	return set, nil
}
