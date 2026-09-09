-- Card drops: a character appears, the first player to name it keeps the card.
--
-- One live drop at a time, enforced by the partial index rather than by the
-- scheduler remembering: an unclaimed drop waits for its winner however long
-- that takes, and only the next day retires it. That is why drop_date is here
-- and why nothing has an expiry timestamp -- "expired" is a decision the next
-- day's first drop makes, not a clock running out.
CREATE TABLE game_drops (
  id          serial PRIMARY KEY,
  mal_char_id integer NOT NULL REFERENCES game_characters(mal_char_id) ON DELETE CASCADE,
  channel_id  text        NOT NULL,
  message_id  text,
  drop_date   date        NOT NULL DEFAULT CURRENT_DATE,
  dropped_at  timestamptz NOT NULL DEFAULT now(),
  claimed_by  integer     REFERENCES users(id) ON DELETE SET NULL,
  claimed_at  timestamptz,
  expired_at  timestamptz,
  guesses     integer     NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX game_drops_one_live
    ON game_drops (channel_id) WHERE claimed_by IS NULL AND expired_at IS NULL;

CREATE INDEX game_drops_day ON game_drops (drop_date);

-- Per player guess cooldown, so a surname list cannot be pasted at every drop.
CREATE TABLE game_guessers (
  user_id       integer NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  next_guess_at timestamptz NOT NULL DEFAULT now()
);
