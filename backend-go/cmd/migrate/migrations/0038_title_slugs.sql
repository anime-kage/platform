-- URL slugs, so /anime/36/episode/3 can read /anime/91-days/episode/3.
--
-- Nullable, and filled by `cmd/autoupdate slugs` rather than here. The rule the
-- Go side applies (fold diacritics, split digit→letter but not ordinals, do not
-- split camelCase) is a dozen lines of readable Go and would be an unreadable
-- nest of regexp_replace in SQL — and it has unit tests there, which SQL in a
-- migration cannot have. A NULL slug simply means "no pretty URL yet"; the
-- numeric id keeps working either way, so nothing breaks in between.
--
-- Manga gets the same treatment: /manga/12 has the identical problem.
ALTER TABLE anime ADD COLUMN IF NOT EXISTS slug text;
ALTER TABLE manga ADD COLUMN IF NOT EXISTS slug text;

-- Unique, but only over the rows that have one — a partial index, so the many
-- NULLs before the backfill runs do not collide with each other.
CREATE UNIQUE INDEX IF NOT EXISTS anime_slug_uniq ON anime (slug) WHERE slug IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS manga_slug_uniq ON manga (slug) WHERE slug IS NOT NULL;
