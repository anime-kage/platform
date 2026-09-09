-- Card trades between players.
--
-- An offer is a row rather than something held in the bot's memory: a restart
-- must not silently void trades people are in the middle of agreeing, and the
-- accept has to be checked against the database anyway because both sides'
-- spares can change between the offer and the answer.
CREATE TABLE game_trades (
  id           serial PRIMARY KEY,
  from_user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  to_user_id   integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  give_char_id integer NOT NULL REFERENCES game_characters(mal_char_id) ON DELETE CASCADE,
  want_char_id integer NOT NULL REFERENCES game_characters(mal_char_id) ON DELETE CASCADE,
  gold         integer NOT NULL DEFAULT 0,
  -- who owes the gold: true when the offering side does
  from_pays    boolean NOT NULL DEFAULT false,
  created_at   timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL,
  accepted_at  timestamptz,
  declined_at  timestamptz,
  CONSTRAINT game_trades_not_self CHECK (from_user_id <> to_user_id)
);

CREATE INDEX game_trades_open ON game_trades (to_user_id)
    WHERE accepted_at IS NULL AND declined_at IS NULL;
