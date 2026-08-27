-- Per-episode metadata: a description, and the filler/recap flags MAL carries.
--
-- `synopsis` mirrors the series-level pattern: one editable text, written by the
-- team, shown on the episode page. Deliberately NOT split into
-- synopsis/synopsis_romanian the way `anime` is — MAL's per-episode synopses are
-- mostly empty or one-line spoilers, so there is no English original worth
-- keeping alongside a Romanian one. This field is ours.
--
-- `is_filler` / `is_recap` come from Jikan's /anime/{id}/episodes, which exposes
-- both as booleans. They are stored rather than derived because the source is
-- unreliable — MAL blocks Jikan for days at a time — and because an editor must
-- be able to set them by hand when it is down, which is most of the time.
ALTER TABLE episodes
    ADD COLUMN IF NOT EXISTS synopsis  text,
    ADD COLUMN IF NOT EXISTS is_filler boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS is_recap  boolean NOT NULL DEFAULT false;

-- Nothing indexes these: they are read as part of an episode row that is always
-- already filtered by anime_id, never queried on their own.
