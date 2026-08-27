-- Series relations: which season comes before/after, and which titles are
-- alternative retellings or spin-offs of the same story.
--
-- Two decisions worth stating, because both were deliberate:
--
-- 1. No `related_anime_id` column. Only the MAL id is stored, and the local
--    row is resolved by joining `anime.mal_id` at read time. The catalog is
--    largely seasonal imports right now, so most sequels here have prequels we
--    have never imported — Mushoku Tensei III, Grand Blue Season 3, Youjo
--    Senki II and eight others. A stored FK would be NULL for all of them and
--    would stay NULL until someone re-ran the sync after every import. Joining
--    on mal_id instead means the day a title enters the catalog, every
--    relation pointing at it lights up on its own, with nothing to re-run.
--
-- 2. Nothing here creates an anime row. A relation to a series we do not have
--    is three integers; it imports no metadata, no cover, and renders nothing
--    until that series exists on its own terms.
CREATE TABLE IF NOT EXISTS anime_relations (
    anime_id       integer NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    -- SEQUEL | PREQUEL | ALTERNATIVE | SIDE_STORY | PARENT | SPIN_OFF | SUMMARY
    relation       text    NOT NULL,
    -- the MAL id of the other series; may or may not be in `anime` today
    related_mal_id integer NOT NULL,
    synced_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (anime_id, related_mal_id)
);

-- the read path: "everything related to this anime"
CREATE INDEX IF NOT EXISTS anime_relations_anime_idx ON anime_relations (anime_id);
-- the resolve path: the join back onto anime.mal_id
CREATE INDEX IF NOT EXISTS anime_relations_related_idx ON anime_relations (related_mal_id);
