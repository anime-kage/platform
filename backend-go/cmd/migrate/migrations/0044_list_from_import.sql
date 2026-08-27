-- Keep bulk list imports out of the community activity feed.
--
-- The importer already avoids watch_history, so /istoric stays clean, but the
-- feed on /home is built from watchlist.updated_at / readlist.updated_at — and
-- an import stamps every row it writes with the same instant. One member
-- importing a ten-year AniList backlog therefore becomes the entire feed:
-- 162 identical "a adăugat" events, all theirs, all at once.
--
-- Marked rather than filtered by timestamp, because "many rows at the same
-- second" is a guess and this is a fact. The flag is cleared the moment the
-- member touches the entry by hand (see UpsertWatchlist), so an imported
-- series that is later actually watched rejoins the feed.
ALTER TABLE public.watchlist ADD COLUMN from_import boolean NOT NULL DEFAULT false;
ALTER TABLE public.readlist  ADD COLUMN from_import boolean NOT NULL DEFAULT false;

-- The feed scans by recency and now also filters on this.
CREATE INDEX watchlist_activity_idx ON public.watchlist (updated_at DESC) WHERE NOT from_import;
CREATE INDEX readlist_activity_idx  ON public.readlist  (updated_at DESC) WHERE NOT from_import;
