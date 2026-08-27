package repo

// Chapter page sets for the native manga reader.

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"
)

// ChapterPageSet returns a chapter's page URLs for one language, in order.
func (r *Repo) ChapterPageSet(ctx context.Context, chapterID int, language string) ([]string, error) {
	urls := []string{}
	err := pgxscan.Select(ctx, r.pool, &urls,
		`SELECT url FROM chapter_pages
		 WHERE chapter_id = $1 AND language = $2
		 ORDER BY idx`, chapterID, language)
	return urls, err
}

// ChapterPageLanguages lists the editions a chapter has, RO first (the
// reader's default), then alphabetical.
func (r *Repo) ChapterPageLanguages(ctx context.Context, chapterID int) ([]string, error) {
	langs := []string{}
	err := pgxscan.Select(ctx, r.pool, &langs,
		`SELECT language FROM chapter_pages
		 WHERE chapter_id = $1
		 GROUP BY language
		 ORDER BY (language = 'ro') DESC, language`, chapterID)
	return langs, err
}

// ReplaceChapterPages swaps a language's whole page set atomically and keeps
// chapters.pages (the metadata count shown in lists) in sync with the RO
// edition.
func (r *Repo) ReplaceChapterPages(ctx context.Context, chapterID int, language string, urls []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM chapter_pages WHERE chapter_id = $1 AND language = $2`,
		chapterID, language); err != nil {
		return err
	}
	for i, u := range urls {
		if _, err := tx.Exec(ctx,
			`INSERT INTO chapter_pages (chapter_id, language, idx, url) VALUES ($1, $2, $3, $4)`,
			chapterID, language, i, u); err != nil {
			return err
		}
	}
	if language == "ro" && len(urls) > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE chapters SET pages = $2 WHERE id = $1`, chapterID, len(urls)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repo) DeleteChapterPages(ctx context.Context, chapterID int, language string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM chapter_pages WHERE chapter_id = $1 AND language = $2`,
		chapterID, language)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ChapterExists guards the pages endpoints' 404s.
func (r *Repo) ChapterExists(ctx context.Context, id int) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM chapters WHERE id = $1)`, id).Scan(&ok)
	return ok, err
}

// ChapterRef identifies a chapter for storage keys (manga/{id}/{number}/…).
type ChapterRef struct {
	MangaID       int
	ChapterNumber float64
}

func (r *Repo) ChapterRef(ctx context.Context, id int) (*ChapterRef, error) {
	var ref ChapterRef
	err := pgxscan.Get(ctx, r.pool, &ref,
		`SELECT manga_id, chapter_number FROM chapters WHERE id = $1`, id)
	if pgxscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ref, nil
}
