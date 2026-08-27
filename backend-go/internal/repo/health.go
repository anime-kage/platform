package repo

// Health dashboard queries. All three read what the rest of the
// system already writes: the health checker's last_ok (3.8), the stream
// endpoint's source ordering (3.3), and the published subtitles (3.5).

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"
)

// DeadSource is an active episode source whose last health check failed.
type DeadSource struct {
	ID            int     `db:"id" json:"id"`
	Kind          string  `db:"kind" json:"kind"`
	Provider      *string `db:"provider" json:"provider,omitempty"`
	HostingURL    string  `db:"hosting_url" json:"hostingUrl"`
	Quality       *string `db:"quality" json:"quality,omitempty"`
	LastCheckedAt *string `db:"last_checked_at" json:"lastCheckedAt,omitempty"`
	EpisodeID     int     `db:"episode_id" json:"episodeId"`
	EpisodeNumber int     `db:"episode_number" json:"episodeNumber"`
	AnimeID       int     `db:"anime_id" json:"animeId"`
	AnimeTitle    string  `db:"anime_title" json:"animeTitle"`
}

// EpisodeGap is an episode missing something (a working source, an RO sub).
type EpisodeGap struct {
	EpisodeID     int    `db:"episode_id" json:"episodeId"`
	EpisodeNumber int    `db:"episode_number" json:"episodeNumber"`
	AnimeID       int    `db:"anime_id" json:"animeId"`
	AnimeTitle    string `db:"anime_title" json:"animeTitle"`
}

type GapList struct {
	Total    int          `json:"total"`
	Episodes []EpisodeGap `json:"episodes"`
}

type HealthReport struct {
	DeadSources   []DeadSource `json:"deadSources"`
	MissingSource GapList      `json:"missingSource"`
	MissingRoSub  GapList      `json:"missingRoSub"`
}

const gapCols = `e.id AS episode_id, e.episode_number, a.id AS anime_id, a.title AS anime_title`

// no active link that isn't known-dead — mirrors the stream endpoint's
// (last_ok IS NOT FALSE) ordering, so "missing" here means "nothing playable"
const missingSourceWhere = `NOT EXISTS (
	SELECT 1 FROM content_links cl
	WHERE cl.episode_id = e.id AND cl.is_active = true AND cl.last_ok IS NOT FALSE)`

const missingRoSubWhere = `NOT EXISTS (
	SELECT 1 FROM subtitles s
	WHERE s.episode_id = e.id AND s.language = 'ro' AND s.status = 'published')`

// gapWheres maps the kind a caller asks for to the predicate. Keeping this a
// lookup rather than a string parameter is the point: the WHERE clause is
// never built from user input.
var gapWheres = map[string]string{
	"source": missingSourceWhere,
	"rosub":  missingRoSubWhere,
}

// EpisodeGaps pages through one of the health report's gap lists. The report
// itself only ever carries the first page — these lists run to thousands of
// rows on a fresh catalog, and nobody reads 1,300 items in a dashboard.
func (r *Repo) EpisodeGaps(ctx context.Context, kind string, limit, offset int) (list GapList, err error) {
	where, ok := gapWheres[kind]
	if !ok {
		return list, ErrNotFound
	}
	list.Episodes = []EpisodeGap{}
	if err = r.pool.QueryRow(ctx,
		`SELECT count(*) FROM episodes e WHERE `+where).Scan(&list.Total); err != nil {
		return list, err
	}
	err = pgxscan.Select(ctx, r.pool, &list.Episodes, `
		SELECT `+gapCols+`
		FROM episodes e JOIN anime a ON a.id = e.anime_id
		WHERE `+where+`
		ORDER BY a.title, e.episode_number
		LIMIT $1 OFFSET $2`, limit, offset)
	return list, err
}

// Overview is the admin panel's stat strip: catalog and team headcounts.
// Pipeline numbers come from the releases list, not from here.
type Overview struct {
	AnimeCount int `json:"animeCount"`
	MangaCount int `json:"mangaCount"`
	TeamCount  int `json:"teamCount"`
}

func (r *Repo) AdminOverview(ctx context.Context) (*Overview, error) {
	var o Overview
	err := r.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM anime),
		       (SELECT count(*) FROM manga),
		       (SELECT count(*) FROM users WHERE role <> 'user')`).
		Scan(&o.AnimeCount, &o.MangaCount, &o.TeamCount)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repo) HealthReport(ctx context.Context, limit int) (*HealthReport, error) {
	rep := &HealthReport{
		DeadSources:   []DeadSource{},
		MissingSource: GapList{Episodes: []EpisodeGap{}},
		MissingRoSub:  GapList{Episodes: []EpisodeGap{}},
	}

	err := pgxscan.Select(ctx, r.pool, &rep.DeadSources, `
		SELECT cl.id, cl.kind, cl.provider, cl.hosting_url, cl.quality,
		       cl.last_checked_at::text AS last_checked_at, `+gapCols+`
		FROM content_links cl
		JOIN episodes e ON e.id = cl.episode_id
		JOIN anime a ON a.id = e.anime_id
		WHERE cl.is_active = true AND cl.last_ok = false
		ORDER BY cl.last_checked_at DESC NULLS LAST, cl.id
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}

	for _, gap := range []struct {
		where string
		list  *GapList
	}{
		{missingSourceWhere, &rep.MissingSource},
		{missingRoSubWhere, &rep.MissingRoSub},
	} {
		err := r.pool.QueryRow(ctx,
			`SELECT count(*) FROM episodes e WHERE `+gap.where).Scan(&gap.list.Total)
		if err != nil {
			return nil, err
		}
		err = pgxscan.Select(ctx, r.pool, &gap.list.Episodes, `
			SELECT `+gapCols+`
			FROM episodes e JOIN anime a ON a.id = e.anime_id
			WHERE `+gap.where+`
			ORDER BY a.title, e.episode_number
			LIMIT $1`, limit)
		if err != nil {
			return nil, err
		}
	}
	return rep, nil
}
