-- Discord games: the account link, and the gacha collection.
--
-- The link already existed implicitly: every account on the platform was
-- created from an invite, and invites record who asked for the code on Discord.
-- Making it explicit matters anyway -- deriving it from invites on every
-- command breaks the day someone joins another way, and it would tie a live
-- feature to a table that exists for a different purpose.
ALTER TABLE users ADD COLUMN discord_id text UNIQUE;

-- Backfill from the invite that created each account. used_by_user_id is the
-- account that redeemed the code; discord_user_id is who asked for it.
--
-- DISTINCT ON because the column is UNIQUE and some people have two site
-- accounts from two invites: four of them on production, where one account is
-- the one they actually use and the other was abandoned (one had never logged
-- in at all). A plain UPDATE hit the unique constraint and aborted the whole
-- migration.
--
-- Recency of use decides it, not which invite came first: the point of the link
-- is that a Discord command reaches the account the person is playing on. A
-- second account simply stays unlinked, which is the right outcome -- one
-- Discord identity, one set of cards.
UPDATE users u
   SET discord_id = pick.discord_user_id
  FROM (
    SELECT DISTINCT ON (i.discord_user_id)
           i.discord_user_id, i.used_by_user_id
      FROM invites i
      JOIN users us ON us.id = i.used_by_user_id
     WHERE i.discord_user_id IS NOT NULL
     ORDER BY i.discord_user_id, us.last_seen_at DESC NULLS LAST, i.used_at DESC
  ) pick
 WHERE pick.used_by_user_id = u.id
   AND u.discord_id IS NULL;

-- One row per character a player owns, with the count of duplicates folded in
-- rather than a row per copy: the collection is read far more often than it is
-- written, and "how many spares do I have" is the question trading asks.
CREATE TABLE game_cards (
  user_id    integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  mal_char_id integer NOT NULL REFERENCES game_characters(mal_char_id) ON DELETE CASCADE,
  copies     integer NOT NULL DEFAULT 1,
  first_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, mal_char_id),
  CONSTRAINT game_cards_copies_positive CHECK (copies > 0)
);
CREATE INDEX game_cards_user_idx ON game_cards (user_id);

-- The daily draw, one per player per day, on the same UTC calendar boundary as
-- the site's chest so the two never disagree about what "today" means.
CREATE TABLE game_draws (
  user_id   integer NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  last_draw date    NOT NULL,
  streak    integer NOT NULL DEFAULT 1,
  CONSTRAINT game_draws_streak_positive CHECK (streak >= 1)
);
