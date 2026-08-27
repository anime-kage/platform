-- Publishing gate: after a verifier approves, a coordinator
-- publishes from /publish — that's where the series/episode mapping is
-- confirmed and the source is attached. A release may now arrive before its
-- series exists in the catalog: the translator proposes a title (free text),
-- the coordinator imports the real one from MAL and links it at publish time.
-- Translators deliberately cannot import titles themselves (catalog hygiene).

ALTER TABLE public.releases ALTER COLUMN anime_id DROP NOT NULL;
ALTER TABLE public.releases ADD COLUMN proposed_title text;

-- a release must name its series one way or the other
ALTER TABLE public.releases ADD CONSTRAINT releases_series_named
  CHECK (anime_id IS NOT NULL OR proposed_title IS NOT NULL);
