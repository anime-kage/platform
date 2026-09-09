-- Progress becomes per-faction, and a faction becomes switchable.
--
-- 0050 put a single xp total on game_profiles and made the faction a one-way
-- choice. Playing it back, that is the wrong shape: it forces a permanent
-- decision before a player has seen what any of the ladders look like, and the
-- ladders are the whole appeal of picking a side.
--
-- So: switch freely, but each faction keeps its own experience. Moving to a new
-- one starts that ladder at level 1; moving back finds the old rank exactly
-- where it was left. Nothing is ever lost, and no choice is punished.
--
-- Gold deliberately does NOT split. It is the shared economy the whole platform
-- prices against (docs/GAMES-ARCHITECTURE.md §3), and a currency that forks per
-- faction would make every price mean something different depending on who you
-- were standing next to. Identity is per faction; money is per player.

CREATE TABLE game_faction_xp (
  user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  faction text    NOT NULL,
  xp      bigint  NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, faction),
  CONSTRAINT game_faction_xp_sane CHECK (xp >= 0)
);

-- Carry across whatever the single-track model had recorded, so nobody who
-- played the staging build loses the level they already reached.
INSERT INTO game_faction_xp (user_id, faction, xp)
SELECT user_id, faction, xp
  FROM game_profiles
 WHERE faction IS NOT NULL AND xp > 0
ON CONFLICT (user_id, faction) DO NOTHING;

-- One source of truth: the per-faction rows. A leftover total on the profile
-- would drift the first time someone switched.
ALTER TABLE game_profiles DROP COLUMN xp;
