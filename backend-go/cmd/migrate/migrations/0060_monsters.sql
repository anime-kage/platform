-- Channel monsters: one live spawn at a time, damage tracked per player.
--
-- The fight is shared, so the row is the fight rather than one row per player:
-- everybody attacks the same HP pool and the rewards are split by how much of
-- it each of them took off. That is deliberately kinder to a hundred person
-- server than first-to-hit, which rewards being awake at the right minute.
CREATE TABLE game_monsters (
  id         serial PRIMARY KEY,
  code       text        NOT NULL,
  name       text        NOT NULL,
  channel_id text        NOT NULL,
  message_id text,
  hp_total   integer     NOT NULL,
  hp_left    integer     NOT NULL,
  xp_pot     integer     NOT NULL,
  gold_pot   integer     NOT NULL,
  spawned_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  killed_at  timestamptz,
  CONSTRAINT game_monsters_hp_sane CHECK (hp_left >= 0 AND hp_left <= hp_total)
);

-- Only one monster may be alive in a channel at a time. A partial index rather
-- than application logic: two /caut commands landing in the same instant would
-- otherwise both spawn, and the channel would be fighting two health bars.
CREATE UNIQUE INDEX game_monsters_one_alive
    ON game_monsters (channel_id) WHERE killed_at IS NULL;

CREATE TABLE game_monster_hits (
  monster_id integer NOT NULL REFERENCES game_monsters(id) ON DELETE CASCADE,
  user_id    integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  damage     integer NOT NULL DEFAULT 0,
  hits       integer NOT NULL DEFAULT 0,
  PRIMARY KEY (monster_id, user_id)
);
