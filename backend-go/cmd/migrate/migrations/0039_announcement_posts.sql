-- Announcements become posts: a body worth reading on its own page, a cover
-- image, a shareable URL, and a comment thread.
--
-- The home strip is unchanged — it still shows tag, title and date. What changes
-- is that the card now leads somewhere.
ALTER TABLE announcements
    -- wide image at the top of the post; the home card stays text-only
    ADD COLUMN IF NOT EXISTS cover_url text,
    -- shareable URL segment, same contract as anime.slug: generated from the
    -- title once and never regenerated, because it is a link people paste
    ADD COLUMN IF NOT EXISTS slug text;

CREATE UNIQUE INDEX IF NOT EXISTS announcements_slug_uniq
    ON announcements (slug) WHERE slug IS NOT NULL;

-- Comments on a post. Same shape as every other comment target: one nullable
-- FK on the shared table rather than a parallel table, so replies, voting,
-- reporting and moderation all work with no new code.
ALTER TABLE comments
    ADD COLUMN IF NOT EXISTS announcement_id integer
        REFERENCES announcements(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS comments_announcement_idx ON comments (announcement_id);
