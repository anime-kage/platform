--: playback resume positions, written by our own player.
-- One row per user per episode; the ~90% "watched" threshold is applied by
-- the progress endpoint (which then bumps the watchlist and watch_history
-- through the existing upsert semantics — not duplicated here).

CREATE TABLE public.playback_positions (
  user_id    integer NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
  episode_id integer NOT NULL REFERENCES public.episodes(id) ON DELETE CASCADE,
  position_s double precision NOT NULL,
  duration_s double precision,
  updated_at timestamp with time zone NOT NULL DEFAULT now(),

  PRIMARY KEY (user_id, episode_id),
  CONSTRAINT playback_positions_position_check CHECK (position_s >= 0)
);
