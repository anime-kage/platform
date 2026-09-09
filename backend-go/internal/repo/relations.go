package repo

// Series relations (migration 0034): the season chain and the franchise graph.
//
// The table stores MAL ids only; the local row is resolved by joining
// anime.mal_id here, so a relation to a series we have not imported costs
// nothing and starts working by itself the moment that series arrives.

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"

	"animekage/backend/internal/model"
)

// AnimeMalIDs lists every catalog anime that has a MAL id — the input to a
// relations sync.
func (r *Repo) AnimeMalIDs(ctx context.Context) ([]int, error) {
	ids := []int{}
	err := pgxscan.Select(ctx, r.pool, &ids,
		`SELECT mal_id FROM anime WHERE mal_id IS NOT NULL AND mal_id > 0 ORDER BY mal_id`)
	return ids, err
}

// RelationRow is one edge to store: the local anime, the kind, the other MAL id.
type RelationRow struct {
	AnimeID      int
	Relation     string
	RelatedMalID int
}

// ReplaceAnimeRelations rewrites one anime's edges in a transaction — delete
// then insert, so an edge AniList has dropped (a mistagged sequel, a merged
// entry) disappears here too instead of lingering for ever.
func (r *Repo) ReplaceAnimeRelations(ctx context.Context, animeID int, rows []RelationRow) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// NOT manual: a hand-pinned correlation describes a shape our catalog has
	// and AniList does not (merged seasons, titles we deliberately skip), so the
	// sync must not undo it. Without this guard every manual fix lasted until
	// 04:15 the next morning.
	if _, err := tx.Exec(ctx, `
		DELETE FROM anime_relations WHERE anime_id = $1 AND NOT manual`, animeID); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := tx.Exec(ctx, `
			INSERT INTO anime_relations (anime_id, relation, related_mal_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (anime_id, related_mal_id) DO UPDATE
			SET relation = EXCLUDED.relation, synced_at = now()`,
			row.AnimeID, row.Relation, row.RelatedMalID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// relatedCols is the slice of a title a relation card needs. Deliberately not
// the whole Anime row: these render as small cards, and the synopsis alone
// would multiply the payload of a detail page carrying a dozen of them.
const relatedCols = `
	rel.id, rel.title, rel.title_english, rel.title_romanian,
	rel.image_url, rel.year, rel.type, rel.status, rel.episodes, rel.slug,
	(SELECT count(*)::int FROM episodes e WHERE e.anime_id = rel.id) AS episode_count`

// AnimeRelations returns one anime's relations that resolve to a title we
// actually have, newest-story-order irrelevant — the caller buckets them.
//
// The INNER JOIN is what implements "don't show what we don't have": an edge
// pointing at an unimported series simply produces no row.
func (r *Repo) AnimeRelations(ctx context.Context, animeID int) ([]model.RelatedAnime, error) {
	rows := []model.RelatedAnime{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT ar.relation, `+relatedCols+`
		FROM anime_relations ar
		JOIN anime rel ON rel.mal_id = ar.related_mal_id
		WHERE ar.anime_id = $1 AND rel.id <> $1
		ORDER BY rel.year NULLS LAST, rel.title`, animeID)
	return rows, err
}

// SeasonChain walks PREQUEL/SEQUEL edges out from one anime and returns the
// whole run in watch order, the anime itself included.
//
// Why a walk and not a single query: the chain is a linked list, and a third
// season is only reachable from the first by going through the second. Fate is
// the case that proves it matters — its PREQUEL points at "Fate/Zero 2nd
// Season", not at Fate/Zero, so even a two-hop chain is normal.
//
// Bounded by `maxChain` and a seen-set: MAL data contains cycles (two entries
// each listing the other as sequel), and an unbounded walk over one would not
// return.
const maxChain = 24

func (r *Repo) SeasonChain(ctx context.Context, animeID int) ([]model.RelatedAnime, error) {
	type step struct {
		ID  int
		Row model.RelatedAnime
	}

	// walk follows one direction, returning the titles found in the order met.
	// `opposite` is the relation that points back the other way, used only by
	// the bridge query below.
	walk := func(from int, kind, opposite string) ([]step, error) {
		out := []step{}
		seen := map[int]bool{from: true}
		cur := from
		for len(out) < maxChain {
			var next model.RelatedAnime
			err := pgxscan.Get(ctx, r.pool, &next, `
				SELECT ar.relation, `+relatedCols+`
				FROM anime_relations ar
				JOIN anime rel ON rel.mal_id = ar.related_mal_id
				WHERE ar.anime_id = $1 AND ar.relation = $2
				-- A television entry wins whenever there is one, and only then
				-- does the year decide. A season chain is a run of seasons: when
				-- MAL lists both a side film and the next series as SEQUEL, the
				-- earliest-year rule quietly picked the film, and the chain
				-- stopped at it. Naruto: Shippuden led to a Boruto special
				-- rather than to Boruto itself for exactly that reason, and
				-- Dragon Ball, Laid-Back Camp and Shakugan no Shana all broke
				-- the same way the night new edges arrived.
				--
				-- Still a preference and not a filter: some series genuinely
				-- continue as a film, and those should keep their chain.
				ORDER BY (rel.type <> 'tv'), rel.year NULLS LAST, rel.id
				LIMIT 1`, cur, kind)

			// Nothing directly adjacent. MAL splits seasons we carry as one
			// entry, so the edge out of a merged season often points at a part
			// we deliberately do not stock, and the walk used to stop dead
			// there. Attack on Titan was the case that surfaced it: season 3
			// leads to MAL 38524 (season 3 part 2), which we do not carry, and
			// The Final Season points back at the same missing entry. Two
			// titles reaching for the same absent one from opposite directions
			// are adjacent in our catalogue even though MAL has a step between
			// them, so cross that gap rather than ending the chain.
			//
			// Only ever a fallback: a real edge always wins, so this cannot
			// reroute a chain that already resolves.
			if err != nil && (pgxscan.NotFound(err) || err == pgx.ErrNoRows) {
				err = pgxscan.Get(ctx, r.pool, &next, `
					SELECT $2::text AS relation, `+relatedCols+`
					FROM anime_relations mine
					JOIN anime_relations theirs
					  ON theirs.related_mal_id = mine.related_mal_id
					 AND theirs.relation = $3
					 AND theirs.anime_id <> mine.anime_id
					JOIN anime rel ON rel.id = theirs.anime_id
					WHERE mine.anime_id = $1 AND mine.relation = $2
					  AND NOT EXISTS (
					        SELECT 1 FROM anime x WHERE x.mal_id = mine.related_mal_id)
					ORDER BY (rel.type <> 'tv'), rel.year NULLS LAST, rel.id
					LIMIT 1`, cur, kind, opposite)
			}
			if err != nil {
				if pgxscan.NotFound(err) || err == pgx.ErrNoRows {
					break
				}
				return nil, err
			}
			if seen[next.ID] {
				break // cycle in the source data
			}
			seen[next.ID] = true
			out = append(out, step{ID: next.ID, Row: next})
			cur = next.ID
		}
		return out, nil
	}

	back, err := walk(animeID, "PREQUEL", "SEQUEL")
	if err != nil {
		return nil, err
	}
	fwd, err := walk(animeID, "SEQUEL", "PREQUEL")
	if err != nil {
		return nil, err
	}

	// A chain of one is not a chain — the caller renders nothing.
	if len(back) == 0 && len(fwd) == 0 {
		return []model.RelatedAnime{}, nil
	}

	var self model.RelatedAnime
	if err := pgxscan.Get(ctx, r.pool, &self, `
		SELECT ''::text AS relation, `+relatedCols+`
		FROM anime rel WHERE rel.id = $1`, animeID); err != nil {
		return nil, err
	}

	out := make([]model.RelatedAnime, 0, len(back)+len(fwd)+1)
	for i := len(back) - 1; i >= 0; i-- { // prequels came out nearest-first
		out = append(out, back[i].Row)
	}
	out = append(out, self)
	for _, s := range fwd {
		out = append(out, s.Row)
	}
	return out, nil
}
