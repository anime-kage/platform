package anilist

// Series banner art. AniList is the only source we have for it:
// MAL — and therefore Jikan — carries portrait covers only, while AniList's
// bannerImage is already a wide crop designed to sit behind text. Coverage on
// our catalog is close to total (30/30 on a sample of top titles).

import "fmt"

// bannerPageSize is AniList's own per-page ceiling. Fetching 50 ids per call
// is what keeps a full-catalog backfill to a handful of requests instead of
// one per title.
const bannerPageSize = 50

// BannersByMal maps MAL id → banner URL for the ids given. kind is "ANIME"
// or "MANGA". Titles with no banner are simply absent from the map; the
// caller decides whether that is worth recording.
func (c *Client) BannersByMal(malIDs []int, kind string) (map[int]string, error) {
	out := make(map[int]string, len(malIDs))

	for start := 0; start < len(malIDs); start += bannerPageSize {
		end := min(start+bannerPageSize, len(malIDs))

		var res struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
			Data struct {
				Page struct {
					Media []struct {
						IDMal       *int    `json:"idMal"`
						BannerImage *string `json:"bannerImage"`
					} `json:"media"`
				} `json:"Page"`
			} `json:"data"`
		}

		gql := `query ($ids: [Int], $n: Int, $type: MediaType) {
			Page(perPage: $n) { media(idMal_in: $ids, type: $type) { idMal bannerImage } } }`
		vars := map[string]any{"ids": malIDs[start:end], "n": bannerPageSize, "type": kind}
		if err := c.query(gql, vars, &res); err != nil {
			return out, err
		}
		if len(res.Errors) > 0 {
			return out, fmt.Errorf("anilist: %s", res.Errors[0].Message)
		}

		for _, m := range res.Data.Page.Media {
			// idMal_in can return more rows than ids asked for (several
			// AniList entries can share a MAL id), so first match wins
			// rather than last.
			if m.IDMal == nil || m.BannerImage == nil || *m.BannerImage == "" {
				continue
			}
			if _, seen := out[*m.IDMal]; !seen {
				out[*m.IDMal] = *m.BannerImage
			}
		}
	}
	return out, nil
}
