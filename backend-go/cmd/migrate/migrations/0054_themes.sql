-- Openings and endings, harvested from AnimeThemes.
--
-- Metadata only. The media itself stays on their CDN and is linked, never
-- mirrored: it keeps that bandwidth and that content off this origin, and a
-- replaced file stays current without a re-import. What is stored here is the
-- pointer plus enough to build a puzzle without calling their API at midnight.
--
-- audio_url and video_url are separate on purpose. The game plays AUDIO while
-- the player guesses -- a single video frame usually gives the series away --
-- and the video is the reveal at the end.
--
-- Keyed by (anime_id, slug) because a series has OP1, OP2, ED1 and so on, and
-- each is a separate puzzle candidate.
CREATE TABLE game_themes (
  id         bigserial PRIMARY KEY,
  anime_id   integer NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
  slug       text    NOT NULL,          -- OP1, ED2, …
  kind       text    NOT NULL,          -- 'OP' | 'ED', for filtering later
  song_title text,
  audio_url  text    NOT NULL,
  video_url  text,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT game_themes_unique UNIQUE (anime_id, slug)
);
CREATE INDEX game_themes_anime_idx ON game_themes (anime_id);
