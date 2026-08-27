-- Per-placement poster override.
--
-- The banner and spotlight are wide, cinematic blocks; a portrait cover often
-- reads badly there. This lets an editor upload artwork for one placement
-- without touching `anime.image_url` / `manga.image_url` — the catalog, the
-- cards and every list keep the real cover. NULL means "use the title's own".
ALTER TABLE public.curated_picks ADD COLUMN image_url text;
