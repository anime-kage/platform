-- content_links had no index on episode_id: only the primary key and
-- content_links_health_idx (is_active, last_checked_at), neither of which can
-- answer "does this episode have a link".
--
-- That made every `EXISTS (SELECT 1 FROM content_links WHERE episode_id = ...)`
-- a sequential scan. Invisible while the table held three rows; the bulk import
-- took it to 381k, and GET /api/users/me/watchlist went to ~2s because the
-- playable-episode subquery runs that EXISTS per episode, twice per row.
--
-- Partial on is_active because every caller wants playable links only, which
-- also keeps the index off the dead rows a source sweep will accumulate.
CREATE INDEX content_links_episode_active_idx
  ON public.content_links (episode_id) WHERE is_active;

-- The chapter side has the same shape and the same latent problem.
CREATE INDEX content_links_chapter_active_idx
  ON public.content_links (chapter_id) WHERE is_active;
