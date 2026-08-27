--: the LibreTranslate machine-translation experiment is dead.
-- These tables were never read by the Go backend, the frontend, or the shared
-- types, and held zero rows. Romanian titles live on anime.title_romanian /
-- manga.title_romanian; episode subtitles get their own table in Phase 4.

DROP TABLE IF EXISTS public.anime_translations;
DROP TABLE IF EXISTS public.manga_translations;
