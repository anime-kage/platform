package repo

// Banner storage: the per-series art, and which series a member
// picked as their profile backdrop.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// TitlesMissingBanner returns MAL ids we have not resolved a banner for yet.
// Driven by a partial index, so a repeat run costs nothing once the catalog
// is covered.
func (r *Repo) TitlesMissingBanner(ctx context.Context, manga bool, limit int) ([]int, error) {
	table := "anime"
	if manga {
		table = "manga"
	}
	rows, err := r.pool.Query(ctx,
		`SELECT mal_id FROM `+table+`
		 WHERE banner_url IS NULL AND mal_id IS NOT NULL
		 ORDER BY score DESC NULLS LAST LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetBanners writes banner URLs keyed by MAL id, in one statement.
//
// `asked` is every id we looked up. Ones AniList had no art for are stored as
// an empty string rather than left NULL — the "missing" index is
// `banner_url IS NULL`, so without this the same artless titles would be
// re-requested on every cron run for ever. Empty means "asked, none exists";
// NULL means "never asked".
func (r *Repo) SetBanners(ctx context.Context, banners map[int]string, manga bool, asked ...int) (int, error) {
	if len(banners) == 0 && len(asked) == 0 {
		return 0, nil
	}
	table := "anime"
	if manga {
		table = "manga"
	}

	malIDs := make([]int, 0, len(asked)+len(banners))
	urls := make([]string, 0, len(asked)+len(banners))
	for id, url := range banners {
		malIDs = append(malIDs, id)
		urls = append(urls, url)
	}
	for _, id := range asked {
		if _, found := banners[id]; !found {
			malIDs = append(malIDs, id)
			urls = append(urls, "")
		}
	}

	tag, err := r.pool.Exec(ctx, `
		UPDATE `+table+` AS t SET banner_url = v.url
		FROM (SELECT unnest($1::int[]) AS mal_id, unnest($2::text[]) AS url) v
		WHERE t.mal_id = v.mal_id`, malIDs, urls)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// ProfileBanner is what the profile header renders.
type ProfileBanner struct {
	MediaType string `json:"mediaType"` // "anime" | "manga"
	ID        int    `json:"id"`
	Title     string `json:"title"`
	BannerURL string `json:"bannerUrl"`
}

// UserBanner returns a member's chosen backdrop, or nil when they have none.
// A title whose banner_url has since gone missing is reported as no banner:
// the header falls back to its plain style rather than rendering a broken
// image behind somebody's name.
func (r *Repo) UserBanner(ctx context.Context, userID int) (*ProfileBanner, error) {
	var (
		animeID, mangaID *int
		aTitle, aBan     *string
		mTitle, mBan     *string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT u.banner_anime_id, u.banner_manga_id,
		       a.title, a.banner_url, m.title, m.banner_url
		FROM users u
		LEFT JOIN anime a ON a.id = u.banner_anime_id
		LEFT JOIN manga m ON m.id = u.banner_manga_id
		WHERE u.id = $1`, userID).
		Scan(&animeID, &mangaID, &aTitle, &aBan, &mTitle, &mBan)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	switch {
	case animeID != nil && aBan != nil && *aBan != "":
		return &ProfileBanner{MediaType: "anime", ID: *animeID, Title: deref(aTitle), BannerURL: *aBan}, nil
	case mangaID != nil && mBan != nil && *mBan != "":
		return &ProfileBanner{MediaType: "manga", ID: *mangaID, Title: deref(mTitle), BannerURL: *mBan}, nil
	default:
		return nil, nil
	}
}

// SetUserBanner points a profile at a title, or clears it when id is 0.
// The two columns are mutually exclusive (CHECK), so setting one always
// clears the other.
func (r *Repo) SetUserBanner(ctx context.Context, userID int, mediaType string, id int) error {
	if id == 0 {
		_, err := r.pool.Exec(ctx,
			`UPDATE users SET banner_anime_id = NULL, banner_manga_id = NULL, updated_at = now()
			 WHERE id = $1`, userID)
		return err
	}

	var col, other string
	switch mediaType {
	case "anime":
		col, other = "banner_anime_id", "banner_manga_id"
	case "manga":
		col, other = "banner_manga_id", "banner_anime_id"
	default:
		return fmt.Errorf("unknown media type %q", mediaType)
	}

	_, err := r.pool.Exec(ctx,
		`UPDATE users SET `+col+` = $2, `+other+` = NULL, updated_at = now() WHERE id = $1`,
		userID, id)
	return err
}

// BannerChoice is one option in the profile's banner picker.
type BannerChoice struct {
	MediaType string `json:"mediaType"`
	ID        int    `json:"id"`
	Title     string `json:"title"`
	BannerURL string `json:"bannerUrl"`
}

// BannerCandidates lists titles from the member's own lists that actually
// have banner art. Scoped to their lists on purpose: a profile backdrop is
// meant to say something about the person, and picking from 250 catalog
// entries they may never have watched says nothing.
func (r *Repo) BannerCandidates(ctx context.Context, userID int) ([]BannerChoice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT 'anime' AS kind, a.id, a.title, a.banner_url
		FROM watchlist w JOIN anime a ON a.id = w.anime_id
		WHERE w.user_id = $1 AND a.banner_url <> ''
		UNION ALL
		SELECT 'manga', m.id, m.title, m.banner_url
		FROM readlist rl JOIN manga m ON m.id = rl.manga_id
		WHERE rl.user_id = $1 AND m.banner_url <> ''
		ORDER BY 3`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BannerChoice{}
	for rows.Next() {
		var c BannerChoice
		if err := rows.Scan(&c.MediaType, &c.ID, &c.Title, &c.BannerURL); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
