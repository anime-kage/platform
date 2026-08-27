package repo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

// ErrExists signals a unique-constraint style conflict (409).
var ErrExists = errors.New("already exists")

// ErrLocked is returned when writing to a locked resource (e.g. a locked
// forum thread).
var ErrLocked = errors.New("locked")

const episodeCols = `id, anime_id, episode_number, title, air_date::text AS air_date, duration,
	synopsis, is_filler, is_recap, created_at`
const chapterCols = `id, manga_id, chapter_number::text AS chapter_number, title, release_date::text AS release_date, pages, created_at`
const linkCols = `id, episode_id, chapter_id, hosting_url, quality, language, is_active,
	kind, provider, provider_ref, priority, last_checked_at, last_ok`

// ── Episodes ──────────────────────────────────────────────────────────────────

// EpisodesByAnime returns all episodes with their active links attached
// (batched in one query, not per-episode like the old backend).
func (r *Repo) EpisodesByAnime(ctx context.Context, animeID int) ([]model.Episode, error) {
	eps := []model.Episode{}
	err := pgxscan.Select(ctx, r.pool, &eps,
		`SELECT `+episodeCols+` FROM episodes WHERE anime_id = $1 ORDER BY episode_number ASC`, animeID)
	if err != nil {
		return nil, err
	}
	if len(eps) == 0 {
		return eps, nil
	}

	ids := make([]int, len(eps))
	byID := make(map[int]*model.Episode, len(eps))
	for i := range eps {
		eps[i].Links = []model.ContentLink{}
		ids[i] = eps[i].ID
		byID[eps[i].ID] = &eps[i]
	}
	links := []model.ContentLink{}
	err = pgxscan.Select(ctx, r.pool, &links,
		`SELECT `+linkCols+` FROM content_links
		 WHERE episode_id = ANY($1) AND is_active = true ORDER BY priority DESC, id`, ids)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		if ep, ok := byID[*l.EpisodeID]; ok {
			ep.Links = append(ep.Links, l)
		}
	}
	return eps, nil
}

// RecentReleases returns the latest published RO-subtitle releases (anime and
// manga interleaved by publish time), newest first — the homepage feed. We only
// publish with a RO track, so this is the true "latest released" list.
func (r *Repo) RecentReleases(ctx context.Context, limit int) ([]model.PublishedRelease, error) {
	// anime and manga live in different catalog tables, so scanning both nested
	// structs in one LEFT-JOIN query would hit NULLs; fetch each medium, merge.
	type animeRow struct {
		ID            int         `db:"id"`
		EpisodeNumber *int        `db:"episode_number"`
		PublishedAt   time.Time   `db:"published_at"`
		Anime         model.Anime `db:"anime"`
	}
	type mangaRow struct {
		ID            int         `db:"id"`
		ChapterNumber *string     `db:"chapter_number"`
		PublishedAt   time.Time   `db:"published_at"`
		Manga         model.Manga `db:"manga"`
	}

	aRows := []animeRow{}
	if err := pgxscan.Select(ctx, r.pool, &aRows, `
		SELECT r.id, r.episode_number, r.updated_at AS published_at,
		       `+animeCols("a", "anime.")+`
		FROM releases r JOIN anime a ON a.id = r.anime_id
		WHERE r.state = 'published' AND r.medium = 'anime' AND a.image_url IS NOT NULL
		ORDER BY r.updated_at DESC LIMIT $1`, limit); err != nil {
		return nil, err
	}
	mRows := []mangaRow{}
	if err := pgxscan.Select(ctx, r.pool, &mRows, `
		SELECT r.id, r.chapter_number::text AS chapter_number, r.updated_at AS published_at,
		       `+mangaCols("m", "manga.")+`
		FROM releases r JOIN manga m ON m.id = r.manga_id
		WHERE r.state = 'published' AND r.medium = 'manga' AND m.image_url IS NOT NULL
		ORDER BY r.updated_at DESC LIMIT $1`, limit); err != nil {
		return nil, err
	}

	out := make([]model.PublishedRelease, 0, len(aRows)+len(mRows))
	for i := range aRows {
		aRows[i].Anime.Normalize()
		a := aRows[i].Anime
		out = append(out, model.PublishedRelease{
			ID: aRows[i].ID, Medium: "anime", EpisodeNumber: aRows[i].EpisodeNumber,
			PublishedAt: aRows[i].PublishedAt, Anime: &a,
		})
	}
	for i := range mRows {
		mRows[i].Manga.Normalize()
		m := mRows[i].Manga
		out = append(out, model.PublishedRelease{
			ID: mRows[i].ID, Medium: "manga", ChapterNumber: mRows[i].ChapterNumber,
			PublishedAt: mRows[i].PublishedAt, Manga: &m,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishedAt.After(out[j].PublishedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// EpisodeLabel returns the anime title + episode number for an episode id, to
// name a subtitle download. ErrNotFound if the episode no longer exists.
func (r *Repo) EpisodeLabel(ctx context.Context, episodeID int) (title string, num int, err error) {
	var row struct {
		Title string `db:"title"`
		Num   int    `db:"episode_number"`
	}
	err = pgxscan.Get(ctx, r.pool, &row,
		`SELECT a.title, e.episode_number FROM episodes e JOIN anime a ON a.id = e.anime_id WHERE e.id = $1`,
		episodeID)
	if pgxscan.NotFound(err) {
		return "", 0, ErrNotFound
	}
	return row.Title, row.Num, err
}

func (r *Repo) EpisodeByNumber(ctx context.Context, animeID, num int) (*model.Episode, error) {
	var ep model.Episode
	err := pgxscan.Get(ctx, r.pool, &ep,
		`SELECT `+episodeCols+` FROM episodes WHERE anime_id = $1 AND episode_number = $2`, animeID, num)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.attachEpisodeLinks(ctx, &ep); err != nil {
		return nil, err
	}
	return &ep, nil
}

type EpisodeInput struct {
	Title    *string
	AirDate  *string // "YYYY-MM-DD"
	Duration *int
	Synopsis *string
	// Pointers, unlike model.Episode's plain bools: nil means "leave alone",
	// which is what lets a PATCH clear a note without also un-marking filler.
	IsFiller *bool
	IsRecap  *bool
}

func (r *Repo) CreateEpisode(ctx context.Context, animeID, num int, in EpisodeInput) (*model.Episode, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM anime WHERE id = $1)`, animeID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("anime with ID %d: %w", animeID, ErrNotFound)
	}

	var ep model.Episode
	err := pgxscan.Get(ctx, r.pool, &ep, `
		INSERT INTO episodes (anime_id, episode_number, title, air_date, duration,
			synopsis, is_filler, is_recap)
		VALUES ($1, $2, $3, $4, $5, $6, coalesce($7, false), coalesce($8, false))
		RETURNING `+episodeCols,
		animeID, num, in.Title, in.AirDate, in.Duration, in.Synopsis, in.IsFiller, in.IsRecap)
	if err != nil {
		if IsUniqueViolation(err) {
			return nil, ErrExists
		}
		return nil, err
	}
	ep.Links = []model.ContentLink{}
	return &ep, nil
}

// UpdateEpisode patches only the non-nil fields.
//
// Synopsis is the exception to coalesce: an empty string clears it, because
// "delete this description" has to be expressible. Every other field keeps the
// leave-alone-on-nil behaviour.
func (r *Repo) UpdateEpisode(ctx context.Context, animeID, num int, in EpisodeInput) (*model.Episode, error) {
	var ep model.Episode
	err := pgxscan.Get(ctx, r.pool, &ep, `
		UPDATE episodes SET
			title     = coalesce($3, title),
			air_date  = coalesce($4::date, air_date),
			duration  = coalesce($5, duration),
			synopsis  = CASE
			              WHEN $6::text IS NULL       THEN synopsis
			              WHEN btrim($6::text) = ''   THEN NULL
			              ELSE btrim($6::text)
			            END,
			is_filler = coalesce($7, is_filler),
			is_recap  = coalesce($8, is_recap)
		WHERE anime_id = $1 AND episode_number = $2
		RETURNING `+episodeCols,
		animeID, num, in.Title, in.AirDate, in.Duration, in.Synopsis, in.IsFiller, in.IsRecap)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.attachEpisodeLinks(ctx, &ep); err != nil {
		return nil, err
	}
	return &ep, nil
}

// episodeTitleIsPlaceholder marks the titles that are really the *absence* of a
// title written out as text: "Episodul 5", "Episode 5", "Ep. 5". Two sources
// produced them — the nightly sync used to insert `fmt.Sprintf("Episode %d")`
// when MAL had no title, and the admin form invites someone to type the same
// thing by hand.
//
// A sync must be free to replace these. Treating them as real editorial content
// is what left 91 Days episode 1 called "Episodul 1" while episodes 2–12 got
// their actual names.
const episodeTitleIsPlaceholder = `
	(coalesce(btrim(title), '') = '' OR title ~* '^(episodul|episode|ep\.?)\s*[0-9]+\s*$')`

// FillEpisodeMeta writes an upstream source's facts onto an episode that
// already exists.
//
// Titles and air dates are filled only where ours are empty (or a placeholder,
// see above) — an editor who typed a real Romanian episode title must not lose
// it to a sync.
//
// filler/recap are pointers so they can be *skipped*, which matters: they are
// overwritten when MAL supplied them (taking MAL's answer is the point of a
// sync), but the AniList fallback has no such flags, and passing its zero values
// through would silently un-mark every filler episode on a series that fell back.
//
// Returns whether the row actually changed, so the caller can report a useful
// count instead of "touched N rows" where N is every episode.
func (r *Repo) FillEpisodeMeta(ctx context.Context, animeID, num int, title, aired *string, filler, recap *bool) (bool, error) {
	// Every placeholder carries an explicit cast. Without them Postgres cannot
	// infer a type for $3/$4 — they appear only inside coalesce and IS NULL,
	// neither of which pins one down — and the statement fails to prepare
	// (SQLSTATE 42P08).
	tag, err := r.pool.Exec(ctx, `
		UPDATE episodes SET
			title     = CASE WHEN `+episodeTitleIsPlaceholder+`
			                 THEN coalesce($3::text, title) ELSE title END,
			air_date  = coalesce(air_date, $4::date),
			is_filler = coalesce($5::boolean, is_filler),
			is_recap  = coalesce($6::boolean, is_recap)
		WHERE anime_id = $1 AND episode_number = $2
		  AND (
		    (`+episodeTitleIsPlaceholder+` AND $3::text IS NOT NULL)
		    OR (air_date IS NULL AND $4::date IS NOT NULL)
		    OR ($5::boolean IS NOT NULL AND is_filler <> $5::boolean)
		    OR ($6::boolean IS NOT NULL AND is_recap <> $6::boolean)
		  )`, animeID, num, title, aired, filler, recap)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) DeleteEpisode(ctx context.Context, animeID, num int) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM episodes WHERE anime_id = $1 AND episode_number = $2`, animeID, num)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) attachEpisodeLinks(ctx context.Context, ep *model.Episode) error {
	ep.Links = []model.ContentLink{}
	return pgxscan.Select(ctx, r.pool, &ep.Links,
		`SELECT `+linkCols+` FROM content_links
		 WHERE episode_id = $1 AND is_active = true ORDER BY priority DESC, id`, ep.ID)
}

// ── Chapters ──────────────────────────────────────────────────────────────────

// ChaptersByManga returns bare chapter rows (the old backend's list endpoint
// did not attach links; the detail endpoint does).
func (r *Repo) ChaptersByManga(ctx context.Context, mangaID int) ([]model.Chapter, error) {
	chs := []model.Chapter{}
	err := pgxscan.Select(ctx, r.pool, &chs,
		// chapters.chapter_number, qualified. chapterCols selects
		// `chapter_number::text AS chapter_number`, and an unqualified ORDER BY
		// binds to that output alias rather than the numeric column — so the
		// list came back sorted as text: 1, 10, 100, 101, … 11, 110, 2.
		// Invisible while the catalog held no chapters; a bulk import made it
		// every manga's chapter list.
		`SELECT `+chapterCols+` FROM chapters WHERE manga_id = $1
		 ORDER BY chapters.chapter_number ASC`, mangaID)
	for i := range chs {
		chs[i].Links = []model.ContentLink{}
	}
	return chs, err
}

func (r *Repo) ChapterByNumber(ctx context.Context, mangaID int, num float64) (*model.Chapter, error) {
	var ch model.Chapter
	err := pgxscan.Get(ctx, r.pool, &ch,
		`SELECT `+chapterCols+` FROM chapters WHERE manga_id = $1 AND chapter_number = $2`, mangaID, num)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ch.Links = []model.ContentLink{}
	err = pgxscan.Select(ctx, r.pool, &ch.Links,
		`SELECT `+linkCols+` FROM content_links
		 WHERE chapter_id = $1 AND is_active = true ORDER BY priority DESC, id`, ch.ID)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

type ChapterInput struct {
	Title       *string
	ReleaseDate *string // "YYYY-MM-DD"
	Pages       *int
}

func (r *Repo) CreateChapter(ctx context.Context, mangaID int, num float64, in ChapterInput) (*model.Chapter, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM manga WHERE id = $1)`, mangaID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("manga with ID %d: %w", mangaID, ErrNotFound)
	}

	var ch model.Chapter
	err := pgxscan.Get(ctx, r.pool, &ch, `
		INSERT INTO chapters (manga_id, chapter_number, title, release_date, pages)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+chapterCols, mangaID, num, in.Title, in.ReleaseDate, in.Pages)
	if err != nil {
		if IsUniqueViolation(err) {
			return nil, ErrExists
		}
		return nil, err
	}
	ch.Links = []model.ContentLink{}
	return &ch, nil
}

func (r *Repo) UpdateChapter(ctx context.Context, mangaID int, num float64, in ChapterInput) (*model.Chapter, error) {
	var ch model.Chapter
	err := pgxscan.Get(ctx, r.pool, &ch, `
		UPDATE chapters SET
			title        = coalesce($3, title),
			release_date = coalesce($4::date, release_date),
			pages        = coalesce($5, pages)
		WHERE manga_id = $1 AND chapter_number = $2
		RETURNING `+chapterCols, mangaID, num, in.Title, in.ReleaseDate, in.Pages)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ch.Links = []model.ContentLink{}
	return &ch, nil
}

func (r *Repo) DeleteChapter(ctx context.Context, mangaID int, num float64) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM chapters WHERE manga_id = $1 AND chapter_number = $2`, mangaID, num)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ── Content links ─────────────────────────────────────────────────────────────

type LinkInput struct {
	EpisodeID   *int
	ChapterID   *int
	HostingURL  string
	Quality     *string
	Language    string
	Kind        string // 'embed' | 'extract'
	Provider    *string
	ProviderRef *string
	Priority    int
}

func (r *Repo) AddContentLink(ctx context.Context, in LinkInput) (*model.ContentLink, error) {
	var l model.ContentLink
	err := pgxscan.Get(ctx, r.pool, &l, `
		INSERT INTO content_links (episode_id, chapter_id, hosting_url, quality, language,
		                           is_active, kind, provider, provider_ref, priority)
		VALUES ($1, $2, $3, $4, $5, true, $6, $7, $8, $9)
		RETURNING `+linkCols,
		in.EpisodeID, in.ChapterID, in.HostingURL, in.Quality, in.Language,
		in.Kind, in.Provider, in.ProviderRef, in.Priority)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// ExtractSources returns an episode's resolvable sources, best first: healthy
// (last_ok true or never checked) before known-dead, then by priority.
func (r *Repo) ExtractSources(ctx context.Context, episodeID int) ([]model.ContentLink, error) {
	links := []model.ContentLink{}
	err := pgxscan.Select(ctx, r.pool, &links,
		`SELECT `+linkCols+` FROM content_links
		 WHERE episode_id = $1 AND is_active = true AND kind = 'extract'
		 ORDER BY (last_ok IS NOT FALSE) DESC, priority DESC, id`, episodeID)
	return links, err
}

// ChapterExtractSources returns a chapter's extractable page sources, best
// first — the manga twin of ExtractSources.
func (r *Repo) ChapterExtractSources(ctx context.Context, chapterID int) ([]model.ContentLink, error) {
	links := []model.ContentLink{}
	err := pgxscan.Select(ctx, r.pool, &links,
		`SELECT `+linkCols+` FROM content_links
		 WHERE chapter_id = $1 AND is_active = true AND kind = 'extract'
		 ORDER BY (last_ok IS NOT FALSE) DESC, priority DESC, id`, chapterID)
	return links, err
}

// AllActiveExtractSources lists every resolvable source across the catalog —
// the health checker's work queue.
func (r *Repo) AllActiveExtractSources(ctx context.Context) ([]model.ContentLink, error) {
	links := []model.ContentLink{}
	err := pgxscan.Select(ctx, r.pool, &links,
		`SELECT `+linkCols+` FROM content_links
		 WHERE is_active = true AND kind = 'extract'
		 ORDER BY last_checked_at ASC NULLS FIRST, id`)
	return links, err
}

// RecordSourceHealth writes a health-check outcome. The stream
// endpoint sorts known-dead sources last, so this is what keeps rot invisible
// to users.
func (r *Repo) RecordSourceHealth(ctx context.Context, id int, ok bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE content_links SET last_checked_at = now(), last_ok = $2 WHERE id = $1`, id, ok)
	return err
}

// AllEpisodeLinks returns every link on an episode, active or not — the admin
// source manager needs disabled ones so they can be re-enabled.
func (r *Repo) AllEpisodeLinks(ctx context.Context, episodeID int) ([]model.ContentLink, error) {
	links := []model.ContentLink{}
	err := pgxscan.Select(ctx, r.pool, &links,
		`SELECT `+linkCols+` FROM content_links
		 WHERE episode_id = $1 ORDER BY priority DESC, id`, episodeID)
	return links, err
}

type LinkPatch struct {
	Quality  *string
	IsActive *bool
	Priority *int
}

// UpdateContentLink patches only the non-nil fields (: reorder /
// disable sources from the admin panel without deleting them).
func (r *Repo) UpdateContentLink(ctx context.Context, id int, in LinkPatch) (*model.ContentLink, error) {
	var l model.ContentLink
	err := pgxscan.Get(ctx, r.pool, &l, `
		UPDATE content_links SET
			quality   = coalesce($2, quality),
			is_active = coalesce($3, is_active),
			priority  = coalesce($4, priority)
		WHERE id = $1
		RETURNING `+linkCols, id, in.Quality, in.IsActive, in.Priority)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *Repo) DeleteContentLink(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM content_links WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
