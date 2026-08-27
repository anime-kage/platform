package repo

// Curated placements. Four surfaces — the landing collage, the home
// spotlight, and the two catalog recommendations — used to pick whatever
// scored highest. These let a coordinator choose instead.

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

// CuratedPick is one chosen title. Exactly one of Anime/Manga is set, matching
// the CHECK on the table.
type CuratedPick struct {
	Position int          `json:"position"`
	Anime    *model.Anime `json:"anime,omitempty"`
	Manga    *model.Manga `json:"manga,omitempty"`
	// List is set for slots that feature a member's list rather than a title
	// ("Listă remarcată" on /liste).
	List *model.UserList `json:"list,omitempty"`
	// ImageURL overrides the title's own cover *for this placement only*.
	// nil means use the title's. The anime/manga rows are never touched.
	ImageURL *string `json:"imageUrl,omitempty"`
}

// CuratedRef is what an editor submits: a slot entry before it is resolved.
type CuratedRef struct {
	MediaType string `json:"mediaType"` // "anime" | "manga"
	ID        int    `json:"id"`
	// Carried on every write because a slot save is a full replace — the
	// admin page sends the override back with each reorder, so moving a
	// poster up does not silently drop the artwork chosen for it.
	ImageURL *string `json:"imageUrl,omitempty"`
}

// curatedRow is the raw join result — ids only, resolved separately so the
// slot read reuses the existing single-title queries rather than growing a
// second copy of the (large) anime/manga column lists.
type curatedRow struct {
	Position int     `db:"position"`
	AnimeID  *int    `db:"anime_id"`
	MangaID  *int    `db:"manga_id"`
	ListID   *int    `db:"list_id"`
	ImageURL *string `db:"image_url"`
}

// CuratedSlot returns a slot's picks in display order, each resolved to the
// full title. A pick whose title has since been deleted simply is not there —
// the FK cascade removes the row, so this cannot return a dangling entry.
func (r *Repo) CuratedSlot(ctx context.Context, slot string) ([]CuratedPick, error) {
	var rows []curatedRow
	if err := pgxscan.Select(ctx, r.pool, &rows,
		`SELECT position, anime_id, manga_id, list_id, image_url FROM curated_picks
		 WHERE slot = $1 ORDER BY position`, slot); err != nil {
		return nil, err
	}

	picks := make([]CuratedPick, 0, len(rows))
	for _, row := range rows {
		p := CuratedPick{Position: row.Position, ImageURL: row.ImageURL}
		switch {
		case row.AnimeID != nil:
			a, err := r.AnimeByID(ctx, *row.AnimeID)
			if err != nil {
				return nil, fmt.Errorf("resolve curated anime %d: %w", *row.AnimeID, err)
			}
			p.Anime = a
		case row.MangaID != nil:
			m, err := r.MangaByID(ctx, *row.MangaID)
			if err != nil {
				return nil, fmt.Errorf("resolve curated manga %d: %w", *row.MangaID, err)
			}
			p.Manga = m
		case row.ListID != nil:
			// viewer 0: a featured list is rendered for everyone, so the
			// per-viewer "liked" flag is not meaningful here.
			l, err := r.UserListByID(ctx, *row.ListID, 0)
			if err != nil {
				return nil, fmt.Errorf("resolve curated list %d: %w", *row.ListID, err)
			}
			if l == nil || !l.IsPublic {
				continue // unpublished or gone: show nothing rather than a stub
			}
			p.List = l
		default:
			continue // impossible while the CHECK holds
		}
		picks = append(picks, p)
	}
	return picks, nil
}

// ReplaceCuratedSlot swaps a slot's contents wholesale, in one transaction.
// Replace rather than patch: the editor UI hands over the whole ordered list,
// and a half-applied reorder on a public page is worse than none.
func (r *Repo) ReplaceCuratedSlot(ctx context.Context, slot string, refs []CuratedRef, editorID int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM curated_picks WHERE slot = $1`, slot); err != nil {
		return err
	}
	for i, ref := range refs {
		var animeID, mangaID, listID *int
		id := ref.ID
		switch ref.MediaType {
		case "manga":
			mangaID = &id
		case "list":
			listID = &id
		default:
			animeID = &id
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO curated_picks (slot, position, anime_id, manga_id, list_id, image_url, created_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`, slot, i, animeID, mangaID, listID, ref.ImageURL, editorID); err != nil {
			// A bad id trips the FK here rather than silently storing a
			// pointer to nothing — the handler turns it into a 400.
			return err
		}
	}
	return tx.Commit(ctx)
}
