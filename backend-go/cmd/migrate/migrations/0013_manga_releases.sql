-- 4.6: the manga release variant. A release is now either an anime episode
-- (video + subtitle events, the editor flow) or a manga chapter (finished
-- RO page images + optional EN originals, the "bring your own pages" flow —
-- verify shows a page flipper, publish writes chapter_pages).

ALTER TABLE releases
  ADD COLUMN medium text NOT NULL DEFAULT 'anime',
  ADD COLUMN manga_id integer REFERENCES manga(id) ON DELETE CASCADE,
  ADD COLUMN chapter_number numeric(5,2);

ALTER TABLE releases ALTER COLUMN episode_number DROP NOT NULL;

ALTER TABLE releases
  ADD CONSTRAINT releases_medium_check CHECK (medium IN ('anime', 'manga'));

ALTER TABLE releases DROP CONSTRAINT releases_episode_positive;
ALTER TABLE releases
  ADD CONSTRAINT releases_episode_positive
    CHECK (episode_number IS NULL OR episode_number > 0),
  ADD CONSTRAINT releases_chapter_positive
    CHECK (chapter_number IS NULL OR chapter_number > 0);

-- a release always names its series: catalog id (per medium) or proposed title
ALTER TABLE releases DROP CONSTRAINT releases_series_named;
ALTER TABLE releases
  ADD CONSTRAINT releases_series_named
    CHECK (anime_id IS NOT NULL OR manga_id IS NOT NULL OR proposed_title IS NOT NULL),
  ADD CONSTRAINT releases_medium_target CHECK (
    (medium = 'anime' AND manga_id IS NULL AND chapter_number IS NULL
                      AND episode_number IS NOT NULL)
    OR
    (medium = 'manga' AND anime_id IS NULL AND episode_number IS NULL
                      AND chapter_number IS NOT NULL)
  );
