package repo

// Skip-mark storage. Manual marks always win: the aniskip upsert
// path only fires when the episode has no marks at all, and manual upserts
// overwrite cached aniskip rows.

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

const skipMarkCols = `id, episode_id, kind, start_s, end_s, source, created_at`

func (r *Repo) SkipMarks(ctx context.Context, episodeID int) ([]model.SkipMark, error) {
	marks := []model.SkipMark{}
	err := pgxscan.Select(ctx, r.pool, &marks,
		`SELECT `+skipMarkCols+` FROM skip_marks WHERE episode_id = $1 ORDER BY kind`, episodeID)
	return marks, err
}

type SkipMarkInput struct {
	EpisodeID int
	Kind      string // 'intro' | 'outro'
	StartS    float64
	EndS      float64
	Source    string // 'manual' | 'aniskip'
}

func (r *Repo) UpsertSkipMark(ctx context.Context, in SkipMarkInput) (*model.SkipMark, error) {
	var m model.SkipMark
	err := pgxscan.Get(ctx, r.pool, &m, `
		INSERT INTO skip_marks (episode_id, kind, start_s, end_s, source)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (episode_id, kind)
		DO UPDATE SET start_s = EXCLUDED.start_s, end_s = EXCLUDED.end_s, source = EXCLUDED.source
		RETURNING `+skipMarkCols,
		in.EpisodeID, in.Kind, in.StartS, in.EndS, in.Source)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repo) DeleteSkipMark(ctx context.Context, episodeID int, kind string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM skip_marks WHERE episode_id = $1 AND kind = $2`, episodeID, kind)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// EpisodeMALRef returns the anime's MAL id (nil when unknown) and the episode
// number — the key AniSkip is queried on.
func (r *Repo) EpisodeMALRef(ctx context.Context, episodeID int) (*int, int, error) {
	var ref struct {
		MalID         *int `db:"mal_id"`
		EpisodeNumber int  `db:"episode_number"`
	}
	err := pgxscan.Get(ctx, r.pool, &ref, `
		SELECT a.mal_id, e.episode_number FROM episodes e
		JOIN anime a ON a.id = e.anime_id WHERE e.id = $1`, episodeID)
	if pgxscan.NotFound(err) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	return ref.MalID, ref.EpisodeNumber, nil
}
