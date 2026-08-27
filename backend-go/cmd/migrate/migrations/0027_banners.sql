-- Series banners + the Letterboxd-style profile backdrop.
--
-- Banners come from AniList: it is the only one of our two metadata sources
-- that has them (MAL/Jikan carry portrait covers only), they are already
-- wide crops meant to sit behind text, and our catalog stores mal_id while
-- AniList exposes idMal — so the two join without any new identity.
ALTER TABLE public.anime ADD COLUMN banner_url text;
ALTER TABLE public.manga ADD COLUMN banner_url text;

-- The series a member chose as their profile backdrop. Two nullable FKs plus
-- a CHECK, like curated_picks: the database keeps the reference honest and
-- ON DELETE SET NULL quietly clears the banner if the title ever leaves the
-- catalog, rather than leaving the profile pointing at a missing row.
ALTER TABLE public.users
  ADD COLUMN banner_anime_id integer REFERENCES public.anime(id) ON DELETE SET NULL,
  ADD COLUMN banner_manga_id integer REFERENCES public.manga(id) ON DELETE SET NULL,
  ADD CONSTRAINT users_banner_one_target CHECK (
    banner_anime_id IS NULL OR banner_manga_id IS NULL
  );

-- the backfill job's working set: titles we have not asked AniList about yet
CREATE INDEX anime_missing_banner_idx ON public.anime (mal_id) WHERE banner_url IS NULL AND mal_id IS NOT NULL;
CREATE INDEX manga_missing_banner_idx ON public.manga (mal_id) WHERE banner_url IS NULL AND mal_id IS NOT NULL;
