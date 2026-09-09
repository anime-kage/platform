-- Per player attack cooldown, and the penalty when a monster hits back.
--
-- Keyed by player rather than by (player, monster): the rest is a rest, and
-- being able to dodge it by running to a different monster would make the
-- cooldown decorative. One row per player, rewritten in place.
CREATE TABLE game_attackers (
  user_id        integer NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  next_attack_at timestamptz NOT NULL DEFAULT now(),
  counters       integer NOT NULL DEFAULT 0
);
