package repo

// Subtitle storage. Published tracks feed the stream endpoint;
// draft states (machine/edited) belong to the release pipeline.

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

const subtitleCols = `id, episode_id, language, label, format, url, status, translator_id, source_sub, created_at`

// PublishedSubtitles returns the live tracks for an episode, RO first.
func (r *Repo) PublishedSubtitles(ctx context.Context, episodeID int) ([]model.Subtitle, error) {
	subs := []model.Subtitle{}
	err := pgxscan.Select(ctx, r.pool, &subs,
		`SELECT `+subtitleCols+` FROM subtitles
		 WHERE episode_id = $1 AND status = 'published'
		 ORDER BY (language = 'ro') DESC, language`, episodeID)
	return subs, err
}

// SubtitlesForEpisode returns every subtitle row for an episode (admin view).
func (r *Repo) SubtitlesForEpisode(ctx context.Context, episodeID int) ([]model.Subtitle, error) {
	subs := []model.Subtitle{}
	err := pgxscan.Select(ctx, r.pool, &subs,
		`SELECT `+subtitleCols+` FROM subtitles
		 WHERE episode_id = $1 ORDER BY (language = 'ro') DESC, language, status`, episodeID)
	return subs, err
}

type SubtitleInput struct {
	EpisodeID    int
	Language     string
	Label        *string
	Format       string
	URL          string
	Status       string
	TranslatorID *int
	SourceSub    *string
}

func (r *Repo) AddSubtitle(ctx context.Context, in SubtitleInput) (*model.Subtitle, error) {
	var s model.Subtitle
	err := pgxscan.Get(ctx, r.pool, &s, `
		INSERT INTO subtitles (episode_id, language, label, format, url, status, translator_id, source_sub)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+subtitleCols,
		in.EpisodeID, in.Language, in.Label, in.Format, in.URL, in.Status, in.TranslatorID, in.SourceSub)
	if err != nil {
		if IsUniqueViolation(err) {
			return nil, ErrExists
		}
		return nil, err
	}
	return &s, nil
}

// UpsertPublishedSubtitle makes a track the live one for its episode+language,
// replacing whatever was published before — the release pipeline's publish
// step re-approving an episode must not 409.
func (r *Repo) UpsertPublishedSubtitle(ctx context.Context, in SubtitleInput) (*model.Subtitle, error) {
	var s model.Subtitle
	err := pgxscan.Get(ctx, r.pool, &s, `
		INSERT INTO subtitles (episode_id, language, label, format, url, status, translator_id, source_sub)
		VALUES ($1, $2, $3, $4, $5, 'published', $6, $7)
		ON CONFLICT (episode_id, language, status) DO UPDATE SET
			label = EXCLUDED.label, format = EXCLUDED.format, url = EXCLUDED.url,
			translator_id = EXCLUDED.translator_id, source_sub = EXCLUDED.source_sub,
			created_at = now()
		RETURNING `+subtitleCols,
		in.EpisodeID, in.Language, in.Label, in.Format, in.URL, in.TranslatorID, in.SourceSub)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repo) DeleteSubtitle(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM subtitles WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
