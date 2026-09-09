-- The treasure hunt's cooldown.
--
-- A timestamp, not a date: the hunt runs on a rolling four-hour cooldown, so
-- unlike the daily draw and the site's chest it must survive midnight rather
-- than reset on it.
CREATE TABLE game_hunts (
  user_id   integer NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  last_hunt timestamptz NOT NULL DEFAULT now(),
  finds     integer NOT NULL DEFAULT 0
);
