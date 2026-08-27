-- Clear episode titles that are really the absence of a title.
--
-- The nightly sync used to insert `fmt.Sprintf("Episode %d", n)` whenever MAL had
-- no title for an episode, so the catalog carries rows literally titled
-- "Episode 3". Two problems with that: it is English on a Romanian site, and it
-- is indistinguishable from a real title, so nothing would ever replace it once
-- MAL published the actual one.
--
-- NULL is the honest value. The UI renders "Episodul 3" from a NULL, which is
-- the same information in the right language, and `FillEpisodeMeta` will fill
-- the real title in on the next sync.
--
-- The insert site is fixed (cmd/autoupdate leaves the title nil now) and
-- FillEpisodeMeta treats this shape as empty, so this is a one-off cleanup of
-- rows the old code left behind.
UPDATE episodes
SET title = NULL
WHERE title ~* '^(episodul|episode|ep\.?)\s*[0-9]+\s*$';
