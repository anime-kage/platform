package anilist

// Series relations — what links a season to the one before and after it, and a
// series to its alternative retellings and spin-offs.
//
// AniList rather than Jikan for the usual reason (MAL blocks Jikan's servers
// for days at a time, and the nightly autoupdate log is full of 504s from
// exactly that), plus one specific to this data: AniList returns the whole
// relation graph for a title in the same page query we already use for
// banners, so a full-catalog sync is a handful of requests rather than one per
// title.

import (
	"fmt"
	"time"
)

// relationPageSize is AniList's per-page ceiling, same as banners.
const relationPageSize = 50

// Relation is one edge of the graph: from the title we asked about, to a MAL
// id, with the kind of link between them.
//
// Only the MAL id travels — no title, no cover. The catalog is keyed on
// mal_id, so that integer is the whole join, and storing anything else would
// mean caching a second copy of metadata that goes stale.
type Relation struct {
	Kind         string // SEQUEL | PREQUEL | ALTERNATIVE | SIDE_STORY | …
	RelatedMalID int
}

// relationKinds is the allowlist. AniList also emits CHARACTER (a music video
// that happens to feature the cast), ADAPTATION (the source manga) and OTHER,
// none of which answer "what do I watch next" — they would just add noise to
// the two strips that consume this.
var relationKinds = map[string]bool{
	"SEQUEL": true, "PREQUEL": true,
	"ALTERNATIVE": true, "ALTERNATIVE_VERSION": true,
	"SIDE_STORY": true, "PARENT": true, "SPIN_OFF": true,
	"SUMMARY": true,
}

// RelationsByMal maps MAL id → its relations, for the ids given. Titles with
// no usable relations are absent from the map.
//
// Non-anime edges are dropped: Frieren's ADAPTATION points at the manga, which
// shares neither our anime table nor its id space.
const relationPauseBetweenPages = 1500 * time.Millisecond

func (c *Client) RelationsByMal(malIDs []int) (map[int][]Relation, error) {
	out := make(map[int][]Relation, len(malIDs))

	for start := 0; start < len(malIDs); start += relationPageSize {
		// Paced deliberately. The whole catalog is dozens of pages and AniList
		// counts by the minute; firing them back to back is what earns a 429
		// (and, on a bad day, a longer cool-off). A bulk sync has no deadline,
		// so spending a minute on it is free.
		if start > 0 {
			time.Sleep(relationPauseBetweenPages)
		}
		end := min(start+relationPageSize, len(malIDs))

		var res struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
			Data struct {
				Page struct {
					Media []struct {
						IDMal     *int `json:"idMal"`
						Relations struct {
							Edges []struct {
								RelationType string `json:"relationType"`
								Node         struct {
									IDMal *int   `json:"idMal"`
									Type  string `json:"type"`
								} `json:"node"`
							} `json:"edges"`
						} `json:"relations"`
					} `json:"media"`
				} `json:"Page"`
			} `json:"data"`
		}

		gql := `query ($ids: [Int], $n: Int) {
			Page(perPage: $n) {
				media(idMal_in: $ids, type: ANIME) {
					idMal
					relations { edges { relationType node { idMal type } } }
				}
			}
		}`
		vars := map[string]any{"ids": malIDs[start:end], "n": relationPageSize}
		if err := c.query(gql, vars, &res); err != nil {
			return out, err
		}
		if len(res.Errors) > 0 {
			return out, fmt.Errorf("anilist: %s", res.Errors[0].Message)
		}

		for _, m := range res.Data.Page.Media {
			if m.IDMal == nil {
				continue
			}
			// Same guard as banners: idMal_in can return several AniList
			// entries sharing one MAL id, so the first wins.
			if _, seen := out[*m.IDMal]; seen {
				continue
			}
			rels := make([]Relation, 0, len(m.Relations.Edges))
			for _, e := range m.Relations.Edges {
				if e.Node.Type != "ANIME" || e.Node.IDMal == nil {
					continue
				}
				if !relationKinds[e.RelationType] {
					continue
				}
				// A title related to itself would render as its own sequel.
				if *e.Node.IDMal == *m.IDMal {
					continue
				}
				rels = append(rels, Relation{Kind: e.RelationType, RelatedMalID: *e.Node.IDMal})
			}
			if len(rels) > 0 {
				out[*m.IDMal] = rels
			}
		}
	}
	return out, nil
}
