-- Guess-the-character: five portraits, two answers each, one submission.
--
-- This mode does not fit the shape the others share. Title, poster and theme
-- all ask about ONE series, so game_puzzles.anime_id identifies the question.
-- A character round shows five people from five different shows, so there is
-- no single anime behind it -- hence char_ids, and anime_id becoming nullable
-- with a CHECK that keeps it mandatory everywhere it still means something.
--
-- favorites comes straight from Jikan and is the only cheap signal for whether
-- a character is recognisable at all. Sorting by it is what keeps a round made
-- of people players have actually seen, rather than five background extras.

CREATE TABLE game_characters (
  mal_char_id integer PRIMARY KEY,
  anime_id    integer NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
  name        text    NOT NULL,
  image_url   text    NOT NULL,
  favorites   integer NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX game_characters_anime_idx ON game_characters (anime_id);
CREATE INDEX game_characters_fav_idx   ON game_characters (favorites DESC);

ALTER TABLE game_puzzles
  ADD COLUMN char_ids integer[],
  ALTER COLUMN anime_id DROP NOT NULL,
  ADD CONSTRAINT game_puzzles_target CHECK (
    (mode = 'character' AND char_ids IS NOT NULL AND array_length(char_ids, 1) > 0)
    OR (mode <> 'character' AND anime_id IS NOT NULL)
  );

-- One row per attempt holds all five slots: the mode is scored as a single
-- submission, so partial state has no meaning and there is nothing to merge.
ALTER TABLE game_attempts
  ADD COLUMN char_answers jsonb;
