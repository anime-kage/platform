-- Episode views: the signal the home leaderboards should have been ranking by.
--
-- Until now "views" were read out of watch_history, which is not a view ledger
-- at all — it is a *progress* ledger, written only by UpsertWatchlist when a
-- member's episode count goes up, with amount = the delta. Marking a 24-episode
-- series as watched therefore wrote one row worth 24, and the leaderboard's
-- sum(amount) read that as 24 views in an instant. The all-time tab was worse
-- still: it counted watchlist rows, i.e. trackers, so the three tabs were not
-- even in the same unit.
--
-- A view here is one member opening one episode. The primary key is what makes
-- it "once per user": a re-watch, a refresh, or a second source on the same
-- episode all collide and are dropped, so the number cannot be inflated by one
-- enthusiastic member. watch_history keeps its own job (the profile history
-- graph) and is no longer consulted for rankings.
CREATE TABLE public.episode_views (
  user_id    integer NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
  episode_id integer NOT NULL REFERENCES public.episodes(id) ON DELETE CASCADE,
  -- Denormalised from episodes. The leaderboard groups by title over a date
  -- window, and carrying anime_id here turns that into one index scan instead
  -- of a join through episodes on every home page render. episodes.anime_id
  -- never changes, so there is nothing for it to drift out of sync with.
  anime_id   integer NOT NULL REFERENCES public.anime(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (user_id, episode_id)
);

-- The leaderboard's access path: newest-first within a title, so the today and
-- 30-day windows scan only the range they need.
CREATE INDEX episode_views_anime_created_idx
  ON public.episode_views (anime_id, created_at DESC);
