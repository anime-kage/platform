-- The daily chest: one claim per day, worth more on a streak.
--
-- Two columns rather than a row per claim. A claims log would let us rebuild
-- any streak after the fact, but the only question ever asked is "can this
-- player claim now, and what is it worth" -- and that is answerable from the
-- last claim and the running count. A log would grow by one row per player per
-- day for information nothing reads.
--
-- last_claim is a DATE, not a timestamp: the chest resets on the calendar day,
-- so someone claiming at 23:59 can claim again at 00:01. Rolling 24 hours would
-- punish playing earlier each day, which is the opposite of the incentive.
CREATE TABLE game_chests (
  user_id    integer PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  last_claim date    NOT NULL,
  streak     integer NOT NULL DEFAULT 1,
  CONSTRAINT game_chests_streak_sane CHECK (streak >= 1)
);
