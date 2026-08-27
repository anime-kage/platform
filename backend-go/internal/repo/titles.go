package repo

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/jikan"
	"animekage/backend/internal/model"
)

// TitleFilters is the shared search/browse filter set for anime and manga.
// One implementation — the old backend had two hand-copied ones, which is how
// fixes kept landing on one side only.
type TitleFilters struct {
	Query  string
	Genres []string
	Year   int
	Season string // winter | spring | summer | fall (anime only)
	Status string
	Type   string
	Letter string
	Page   int
	Limit  int
	Sort   string // score | title | year | createdAt
	Dir    string // asc | desc — empty means the sort's natural direction
}

var letterRe = regexp.MustCompile(`^[a-zA-Z]$`)

// buildTitleWhere renders the filters into a WHERE clause for either table.
func buildTitleWhere(f TitleFilters, args *[]any) string {
	var conds []string
	add := func(cond string, vals ...any) {
		for _, v := range vals {
			*args = append(*args, v)
		}
		conds = append(conds, cond)
	}
	n := func() int { return len(*args) } // last placeholder index

	if f.Query != "" {
		// Match at word starts, not anywhere in the string. A plain %q% made
		// short queries useless: "nana" matched "Osa(nana)jimi", which is a
		// substring but not a word anyone was searching for. `\m` is
		// Postgres's start-of-word anchor, so "nana" finds Nana and Nanatsu
		// but not Osananajimi, and "zero" still finds "Re:Zero" — punctuation
		// counts as a boundary, which a naive "space before it" check misses.
		*args = append(*args, `\m`+regexp.QuoteMeta(f.Query))
		conds = append(conds, fmt.Sprintf(
			`(title ~* $%d OR title_english ~* $%d OR title_romanian ~* $%d)`, n(), n(), n()))
	}
	if len(f.Genres) > 0 {
		var ors []string
		for _, g := range f.Genres {
			*args = append(*args, g)
			ors = append(ors, fmt.Sprintf(`$%d = ANY(genres)`, n()))
		}
		conds = append(conds, "("+strings.Join(ors, " OR ")+")")
	}
	if f.Year != 0 {
		add(fmt.Sprintf(`year = $%d`, n()+1), f.Year)
	}
	if f.Season != "" {
		add(fmt.Sprintf(`season = $%d`, n()+1), strings.ToLower(f.Season))
	}
	if f.Status != "" {
		add(fmt.Sprintf(`status = $%d`, n()+1), f.Status)
	}
	if f.Type != "" {
		add(fmt.Sprintf(`type = $%d`, n()+1), f.Type)
	}
	switch {
	case f.Letter == "0-9":
		conds = append(conds, `title ~ '^[0-9]'`)
	case f.Letter == "other":
		conds = append(conds, `title ~ '^[^a-zA-Z0-9]'`)
	case letterRe.MatchString(f.Letter):
		add(fmt.Sprintf(`title ILIKE $%d`, n()+1), f.Letter+"%")
	}

	if len(conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conds, " AND ")
}

// titleOrder builds the ORDER BY. `dir` only ever picks between two literals
// here — the caller's string never reaches the SQL — and an empty `dir` means
// each sort's natural direction: best/newest first, but A→Z for the title.
func titleOrder(sort, dir string) string {
	natural := "desc"
	if sort == "title" {
		natural = "asc"
	}
	if dir != "asc" && dir != "desc" {
		dir = natural
	}
	d := "DESC"
	if dir == "asc" {
		d = "ASC"
	}
	switch sort {
	case "score":
		// NULLS LAST either way: an unscored title is missing data, not a zero,
		// so it belongs at the bottom of both directions
		return " ORDER BY score " + d + " NULLS LAST, title ASC"
	case "title":
		return " ORDER BY title " + d
	case "year":
		return " ORDER BY year " + d + " NULLS LAST, title ASC"
	default:
		return " ORDER BY created_at " + d
	}
}

// searchTitles runs the filtered, paginated query for one table.
func searchTitles[T any](ctx context.Context, r *Repo, table, cols string, f TitleFilters, sortDefault string) ([]T, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 {
		f.Limit = 25
	}
	sort := f.Sort
	if sort == "" {
		sort = sortDefault
	}

	var args []any
	where := buildTitleWhere(f, &args)

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM `+table+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := titleOrder(sort, f.Dir)
	// Relevance first when the user actually typed something: a title that
	// *starts* with the query beats one that merely contains the word later
	// on, whatever their scores are. Without this, "naru" ranked
	// "Naruto: Shippuuden" above "Naruto" purely because it scores higher.
	// Added after the count query on purpose — count has no ORDER BY, so it
	// must not see this placeholder.
	if f.Query != "" {
		args = append(args, f.Query+"%")
		// coalesce the nullable title columns: `NULL ILIKE …` is NULL, so
		// `false OR NULL` is NULL — and DESC sorts NULLS FIRST in Postgres,
		// which put every *non*-matching row at the top and inverted the
		// ranking outright.
		order = strings.Replace(order, " ORDER BY ",
			fmt.Sprintf(" ORDER BY (title ILIKE $%[1]d OR coalesce(title_english, '') ILIKE $%[1]d OR coalesce(title_romanian, '') ILIKE $%[1]d) DESC, ", len(args)),
			1)
	}

	q := `SELECT ` + cols + ` FROM ` + table + where + order +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, f.Limit, (f.Page-1)*f.Limit)

	rows := []T{}
	if err := pgxscan.Select(ctx, r.pool, &rows, q, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ── Anime ─────────────────────────────────────────────────────────────────────

func (r *Repo) SearchAnime(ctx context.Context, f TitleFilters) ([]model.Anime, int, error) {
	// search endpoints historically sorted by score first
	rows, total, err := searchTitles[model.Anime](ctx, r, "anime", animeCols("anime", ""), f, "score")
	for i := range rows {
		rows[i].Normalize()
	}
	return rows, total, err
}

func (r *Repo) AnimeByID(ctx context.Context, id int) (*model.Anime, error) {
	return r.animeBy(ctx, "id", id)
}

func (r *Repo) AnimeByMalID(ctx context.Context, malID int) (*model.Anime, error) {
	return r.animeBy(ctx, "mal_id", malID)
}

// AnimeBySlug resolves a pretty URL segment ("91-days") to its row.
func (r *Repo) AnimeBySlug(ctx context.Context, s string) (*model.Anime, error) {
	var a model.Anime
	err := pgxscan.Get(ctx, r.pool, &a,
		`SELECT `+animeCols("anime", "")+` FROM anime WHERE slug = $1`, s)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.Normalize()
	return &a, nil
}

// MangaBySlug is the manga counterpart.
func (r *Repo) MangaBySlug(ctx context.Context, s string) (*model.Manga, error) {
	var m model.Manga
	err := pgxscan.Get(ctx, r.pool, &m,
		`SELECT `+mangaCols("manga", "")+` FROM manga WHERE slug = $1`, s)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.Normalize()
	return &m, nil
}

// TitlesMissingSlug lists rows the slug backfill still has to fill.
func (r *Repo) TitlesMissingSlug(ctx context.Context, manga bool) ([]struct {
	ID    int    `db:"id"`
	Title string `db:"title"`
	MalID *int   `db:"mal_id"`
}, error) {
	table := "anime"
	if manga {
		table = "manga"
	}
	rows := []struct {
		ID    int    `db:"id"`
		Title string `db:"title"`
		MalID *int   `db:"mal_id"`
	}{}
	err := pgxscan.Select(ctx, r.pool, &rows,
		`SELECT id, title, mal_id FROM `+table+` WHERE slug IS NULL ORDER BY id`)
	return rows, err
}

// SetSlug stores a slug, returning ErrExists when it is taken. The caller
// disambiguates rather than this silently mangling the input, so the suffix it
// picks is predictable.
func (r *Repo) SetSlug(ctx context.Context, manga bool, id int, s string) error {
	table := "anime"
	if manga {
		table = "manga"
	}
	_, err := r.pool.Exec(ctx, `UPDATE `+table+` SET slug = $2 WHERE id = $1`, id, s)
	if err != nil && IsUniqueViolation(err) {
		return ErrExists
	}
	return err
}

func (r *Repo) animeBy(ctx context.Context, col string, v int) (*model.Anime, error) {
	var a model.Anime
	err := pgxscan.Get(ctx, r.pool, &a,
		`SELECT `+animeCols("anime", "")+` FROM anime WHERE `+col+` = $1`, v)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.Normalize()
	return &a, nil
}

func (r *Repo) RandomAnime(ctx context.Context) (*model.Anime, error) {
	var a model.Anime
	err := pgxscan.Get(ctx, r.pool, &a,
		`SELECT `+animeCols("anime", "")+` FROM anime ORDER BY RANDOM() LIMIT 1`)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.Normalize()
	return &a, nil
}

// LeaderboardRow is a catalog row plus the popularity metric for the requested
// window ("points"). Points is a view count in every window — one member
// opening one episode, counted once — so the three tabs differ only by the
// range of time they cover and can be read against each other.
type LeaderboardRow struct {
	model.Anime
	Points int `db:"points" json:"points"`
}

// LeaderboardAnime ranks anime by episode views over a window.
//
// All three windows count rows in episode_views, whose primary key already
// enforces one view per member per episode, so the query needs no DISTINCT and
// nothing here can be inflated by a re-watch.
//
// This used to read watch_history for "today"/"month" and watchlist for "all",
// which was wrong twice over: watch_history stores progress *deltas*, so marking
// a 24-episode series watched contributed 24 in a single row, and "all" counted
// trackers instead of views, putting the tabs in different units entirely.
//
// The join stays INNER for the time windows — a quiet day should yield an honest
// empty list rather than a chart of zeroes — and for "all" too, since a catalog
// nobody has opened has no ranking to show.
func (r *Repo) LeaderboardAnime(ctx context.Context, window string, limit int) ([]LeaderboardRow, error) {
	rows := []LeaderboardRow{}

	// Only the window predicate varies; keeping one query shape means the three
	// tabs cannot drift apart again the way they did before.
	since := ""
	switch window {
	case "today":
		since = `AND v.created_at >= date_trunc('day', now())`
	case "month":
		since = `AND v.created_at >= now() - interval '30 days'`
	default: // "all" — no lower bound
	}

	q := `SELECT ` + animeCols("a", "") + `, count(*)::int AS points
		FROM anime a
		JOIN episode_views v ON v.anime_id = a.id ` + since + `
		GROUP BY a.id
		ORDER BY points DESC, a.score DESC NULLS LAST
		LIMIT $1`

	err := pgxscan.Select(ctx, r.pool, &rows, q, limit)
	for i := range rows {
		rows[i].Normalize()
	}
	return rows, err
}

// Schedule groups currently-airing anime by broadcast day.
func (r *Repo) Schedule(ctx context.Context) (map[string][]model.Anime, error) {
	rows := []model.Anime{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT `+animeCols("anime", "")+` FROM anime
		WHERE status = 'airing' AND broadcast_day IS NOT NULL
		ORDER BY broadcast_time, title`)
	if err != nil {
		return nil, err
	}
	schedule := map[string][]model.Anime{
		"monday": {}, "tuesday": {}, "wednesday": {}, "thursday": {},
		"friday": {}, "saturday": {}, "sunday": {},
	}
	for i := range rows {
		rows[i].Normalize()
		day := strings.ToLower(deref(rows[i].BroadcastDay))
		if _, ok := schedule[day]; ok {
			schedule[day] = append(schedule[day], rows[i])
		}
	}
	return schedule, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// InsertAnime saves a Jikan result; on a mal_id conflict the existing row wins.
func (r *Repo) InsertAnime(ctx context.Context, d jikan.AnimeData) (*model.Anime, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO anime (mal_id, title, title_english, synopsis, genres, studios,
		                   status, type, episodes, score, year, season, image_url,
		                   trailer_url, broadcast_day, broadcast_time)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (mal_id) DO UPDATE SET updated_at = anime.updated_at
		RETURNING id`,
		d.MalID, d.Title, d.TitleEnglish, d.Synopsis, d.Genres, d.Studios,
		d.Status, d.Type, d.Episodes, d.Score, d.Year, d.Season, d.ImageURL,
		d.TrailerURL, d.BroadcastDay, d.BroadcastTime).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.AnimeByID(ctx, id)
}

// InsertAnimeManual creates a catalog entry with no MAL identity (mal_id
// NULL, so manual entries never collide on the unique index) — the
// coordinator fallback for when neither Jikan nor AniList can serve. A later
// MAL import can be linked by hand.
func (r *Repo) InsertAnimeManual(ctx context.Context, d jikan.AnimeData) (*model.Anime, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO anime (title, title_english, synopsis, genres, studios,
		                   status, type, episodes, year, season, image_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id`,
		d.Title, d.TitleEnglish, d.Synopsis, d.Genres, d.Studios,
		d.Status, d.Type, d.Episodes, d.Year, d.Season, d.ImageURL).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.AnimeByID(ctx, id)
}

// SetAnimeSynopsisRo stores an auto-translated synopsis. The IS NULL guard
// means a background translation never clobbers a hand-written description
// that landed while the model was running.
func (r *Repo) SetAnimeSynopsisRo(ctx context.Context, id int, text string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE anime SET synopsis_romanian = $2, updated_at = now()
		 WHERE id = $1 AND synopsis_romanian IS NULL`, id, text)
	return err
}

// UpdateAnimeFromJikan refreshes a row in place (titleRomanian is ours, kept).
func (r *Repo) UpdateAnimeFromJikan(ctx context.Context, id int, d jikan.AnimeData) (*model.Anime, error) {
	_, err := r.pool.Exec(ctx, `
		UPDATE anime SET mal_id=$2, title=$3, title_english=$4, synopsis=$5,
			genres=$6, studios=$7, status=$8, type=$9, episodes=$10, score=$11,
			year=$12, season=$13, image_url=$14, trailer_url=$15,
			broadcast_day=$16, broadcast_time=$17, updated_at=now()
		WHERE id=$1`,
		id, d.MalID, d.Title, d.TitleEnglish, d.Synopsis, d.Genres, d.Studios,
		d.Status, d.Type, d.Episodes, d.Score, d.Year, d.Season, d.ImageURL,
		d.TrailerURL, d.BroadcastDay, d.BroadcastTime)
	if err != nil {
		return nil, err
	}
	return r.AnimeByID(ctx, id)
}

// ── Manga ─────────────────────────────────────────────────────────────────────

func (r *Repo) SearchManga(ctx context.Context, f TitleFilters) ([]model.Manga, int, error) {
	rows, total, err := searchTitles[model.Manga](ctx, r, "manga", mangaCols("manga", ""), f, "score")
	for i := range rows {
		rows[i].Normalize()
	}
	return rows, total, err
}

func (r *Repo) MangaByID(ctx context.Context, id int) (*model.Manga, error) {
	return r.mangaBy(ctx, "id", id)
}

func (r *Repo) MangaByMalID(ctx context.Context, malID int) (*model.Manga, error) {
	return r.mangaBy(ctx, "mal_id", malID)
}

func (r *Repo) mangaBy(ctx context.Context, col string, v int) (*model.Manga, error) {
	var m model.Manga
	err := pgxscan.Get(ctx, r.pool, &m,
		`SELECT `+mangaCols("manga", "")+` FROM manga WHERE `+col+` = $1`, v)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.Normalize()
	return &m, nil
}

func (r *Repo) InsertManga(ctx context.Context, d jikan.MangaData) (*model.Manga, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO manga (mal_id, title, title_english, synopsis, genres, authors,
		                   status, type, chapters, volumes, score, year, image_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (mal_id) DO UPDATE SET updated_at = manga.updated_at
		RETURNING id`,
		d.MalID, d.Title, d.TitleEnglish, d.Synopsis, d.Genres, d.Authors,
		d.Status, d.Type, d.Chapters, d.Volumes, d.Score, d.Year, d.ImageURL).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.MangaByID(ctx, id)
}

// InsertMangaManual mirrors InsertAnimeManual: no MAL identity, no conflict.
func (r *Repo) InsertMangaManual(ctx context.Context, d jikan.MangaData) (*model.Manga, error) {
	var id int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO manga (title, title_english, synopsis, genres, authors,
		                   status, type, chapters, volumes, year, image_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id`,
		d.Title, d.TitleEnglish, d.Synopsis, d.Genres, d.Authors,
		d.Status, d.Type, d.Chapters, d.Volumes, d.Year, d.ImageURL).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.MangaByID(ctx, id)
}

// SetMangaSynopsisRo — see SetAnimeSynopsisRo.
func (r *Repo) SetMangaSynopsisRo(ctx context.Context, id int, text string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE manga SET synopsis_romanian = $2, updated_at = now()
		 WHERE id = $1 AND synopsis_romanian IS NULL`, id, text)
	return err
}

func (r *Repo) UpdateMangaFromJikan(ctx context.Context, id int, d jikan.MangaData) (*model.Manga, error) {
	_, err := r.pool.Exec(ctx, `
		UPDATE manga SET mal_id=$2, title=$3, title_english=$4, synopsis=$5,
			genres=$6, authors=$7, status=$8, type=$9, chapters=$10, volumes=$11,
			score=$12, year=$13, image_url=$14, updated_at=now()
		WHERE id=$1`,
		id, d.MalID, d.Title, d.TitleEnglish, d.Synopsis, d.Genres, d.Authors,
		d.Status, d.Type, d.Chapters, d.Volumes, d.Score, d.Year, d.ImageURL)
	if err != nil {
		return nil, err
	}
	return r.MangaByID(ctx, id)
}

// ── Manual catalog edits + deletes ─────────────────────────────────

// TitlePatch carries the admin panel's manual field edits; nil = untouched.
// Jikan sync (UpdateAnimeFromJikan) overwrites these on the next refresh —
// manual edits are for fixing data Jikan doesn't have or gets wrong.
type TitlePatch struct {
	Title         *string
	TitleEnglish  *string
	TitleRomanian *string
	Synopsis      *string
	SynopsisRo    *string
	ImageURL      *string
	Status        *string
	Type          *string
	Year          *int
	Genres        *[]string
	Episodes      *int      // anime only
	Studios       *[]string // anime only
	Chapters      *int      // manga only
	Volumes       *int      // manga only
	Authors       *[]string // manga only
}

func (r *Repo) PatchAnime(ctx context.Context, id int, p TitlePatch) (*model.Anime, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE anime SET
			title             = coalesce($2, title),
			title_english     = coalesce($3, title_english),
			title_romanian    = coalesce($4, title_romanian),
			synopsis          = coalesce($5, synopsis),
			synopsis_romanian = coalesce($6, synopsis_romanian),
			image_url         = coalesce($7, image_url),
			status            = coalesce($8, status),
			type              = coalesce($9, type),
			year              = coalesce($10, year),
			genres            = coalesce($11, genres),
			episodes          = coalesce($12, episodes),
			studios           = coalesce($13, studios),
			updated_at        = now()
		WHERE id = $1`,
		id, p.Title, p.TitleEnglish, p.TitleRomanian, p.Synopsis, p.SynopsisRo,
		p.ImageURL, p.Status, p.Type, p.Year, p.Genres, p.Episodes, p.Studios)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.AnimeByID(ctx, id)
}

func (r *Repo) PatchManga(ctx context.Context, id int, p TitlePatch) (*model.Manga, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE manga SET
			title             = coalesce($2, title),
			title_english     = coalesce($3, title_english),
			title_romanian    = coalesce($4, title_romanian),
			synopsis          = coalesce($5, synopsis),
			synopsis_romanian = coalesce($6, synopsis_romanian),
			image_url         = coalesce($7, image_url),
			status            = coalesce($8, status),
			type              = coalesce($9, type),
			year              = coalesce($10, year),
			genres            = coalesce($11, genres),
			chapters          = coalesce($12, chapters),
			volumes           = coalesce($13, volumes),
			authors           = coalesce($14, authors),
			updated_at        = now()
		WHERE id = $1`,
		id, p.Title, p.TitleEnglish, p.TitleRomanian, p.Synopsis, p.SynopsisRo,
		p.ImageURL, p.Status, p.Type, p.Year, p.Genres, p.Chapters, p.Volumes, p.Authors)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.MangaByID(ctx, id)
}

// DeleteAnime removes the title; episodes, links, subtitles, skip marks,
// watchlist rows, comments, and releases all cascade (0009).
func (r *Repo) DeleteAnime(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM anime WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) DeleteManga(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM manga WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
