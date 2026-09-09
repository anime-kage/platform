-- Arcade: daily puzzles, and the one progression track behind them.
--
-- Four things live here, and the split matters.
--
-- game_profiles is the player: faction, and the cached balances. Cached, not
-- authoritative -- game_ledger is authoritative. A balance column that six
-- features increment is a column nobody can audit, and the first exploit costs
-- a database restore to unpick. Every movement is a ledger row, and the profile
-- carries the running total so the common read is one lookup rather than a sum
-- over thousands of rows.
--
-- game_puzzles is one row per (mode, day). Generated ahead of time by a
-- scheduled job, never on first request: otherwise the first visitor after
-- midnight pays the generation latency, and a failure means no puzzle at all
-- rather than yesterday's still being there.
--
-- The answer is a plain anime_id and it never leaves the server. Guesses are
-- checked here. Shipping the answer to the client and comparing in the browser
-- would make the whole thing a view-source away from pointless.
--
-- game_attempts records awarded_xp/awarded_gold on the row rather than only in
-- the ledger, because the award rule depends on how old the puzzle was when it
-- was solved (full value today, half for the catch-up week) and we want to be
-- able to show a player what they earned without re-deriving that later.

CREATE TABLE game_profiles (
  user_id    integer PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  faction    text,
  xp         bigint  NOT NULL DEFAULT 0,
  gold       bigint  NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT game_profiles_xp_sane   CHECK (xp   >= 0),
  CONSTRAINT game_profiles_gold_sane CHECK (gold >= 0)
);

CREATE TABLE game_ledger (
  id         bigserial PRIMARY KEY,
  user_id    integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  xp_delta   integer NOT NULL DEFAULT 0,
  gold_delta integer NOT NULL DEFAULT 0,
  -- 'puzzle_solve', 'badge', later 'card_pull', 'shop_buy', 'gift'
  reason     text    NOT NULL,
  ref        text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX game_ledger_user_idx ON game_ledger (user_id, created_at DESC);

CREATE TABLE game_puzzles (
  id        bigserial PRIMARY KEY,
  mode      text NOT NULL,
  play_date date NOT NULL,
  anime_id  integer NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
  CONSTRAINT game_puzzles_mode CHECK (mode IN ('title', 'poster', 'theme', 'character')),
  CONSTRAINT game_puzzles_one_per_day UNIQUE (mode, play_date)
);
CREATE INDEX game_puzzles_date_idx ON game_puzzles (play_date DESC, mode);

CREATE TABLE game_attempts (
  puzzle_id    bigint  NOT NULL REFERENCES game_puzzles(id) ON DELETE CASCADE,
  user_id      integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  guesses      jsonb   NOT NULL DEFAULT '[]'::jsonb,
  solved_at    timestamptz,
  gave_up      boolean NOT NULL DEFAULT false,
  awarded_xp   integer NOT NULL DEFAULT 0,
  awarded_gold integer NOT NULL DEFAULT 0,
  updated_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (puzzle_id, user_id)
);
CREATE INDEX game_attempts_user_idx ON game_attempts (user_id);

-- Badges are recorded, not computed on read: "solved every title puzzle last
-- week" stops being answerable once the week rolls past the seven-day window
-- the arcade keeps, so the award has to be captured when it happens.
CREATE TABLE game_badges (
  user_id   integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code      text    NOT NULL,
  earned_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, code)
);
