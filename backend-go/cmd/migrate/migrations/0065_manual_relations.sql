-- Let a relation be pinned by hand and survive the nightly sync.
--
-- syncRelations deletes every row for an anime and re-inserts what AniList
-- reports, so anything added by hand vanished at the next 04:15 cron. That is
-- fine while our catalog mirrors MAL's shape, and wrong as soon as it does not.
--
-- Attack on Titan is the case that forced this. MAL splits the story into
-- Season 3, Season 3 Part 2, The Final Season and The Final Season Part 2. Our
-- catalog merges them the way IMDb does: Season 3 holds 22 episodes and The
-- Final Season holds 30. The sequel edge from Season 3 therefore points at MAL
-- 38524, an entry we deliberately do not carry, and the chain dead-ends: the
-- walk uses an inner join, so an edge to a title we do not have is dropped and
-- everything past it is stranded. 521 series are in that position.
--
-- A manual row states the correlation our catalog actually has, and the sync
-- now leaves those alone.
ALTER TABLE anime_relations
  ADD COLUMN manual boolean NOT NULL DEFAULT false;

-- The sync only ever wrote automatic rows, so a partial index keeps the
-- hand-made ones cheap to find when someone asks what has been overridden.
CREATE INDEX anime_relations_manual_idx ON anime_relations (anime_id) WHERE manual;
