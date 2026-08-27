-- Per-user override for the in-flight release cap.
--
-- Staging is bounded by translators × cap × file size, so the cap is what keeps
-- disk predictable when a team translates faster than it verifies. The global
-- default lives in TRANSLATOR_RELEASE_CAP; this column raises or lowers it for
-- one person. NULL means "use the default" — the common case, and the reason
-- this is nullable rather than defaulted.
ALTER TABLE users ADD COLUMN release_cap integer;

ALTER TABLE users ADD CONSTRAINT users_release_cap_nonneg
  CHECK (release_cap IS NULL OR release_cap >= 0);
