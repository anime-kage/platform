-- Grupe: a Connections-style daily. Sixteen titles, four hidden groups of four.
--
-- The groups live in one jsonb column rather than a side table because a puzzle
-- is written once by the generator and then only ever read whole. The shape is
--   [{"label": "Studio: MAPPA", "anime_ids": [12, 34, 56, 78]}, ...]
-- and the API never sends `label` or the grouping to the browser until a group
-- has been found, because the whole puzzle is the grouping.
ALTER TABLE game_puzzles ADD COLUMN IF NOT EXISTS groups jsonb;

-- Both check constraints named the four original modes explicitly, so a fifth
-- mode has to be admitted to each.
ALTER TABLE game_puzzles DROP CONSTRAINT IF EXISTS game_puzzles_mode;
ALTER TABLE game_puzzles ADD CONSTRAINT game_puzzles_mode
  CHECK (mode = ANY (ARRAY['title','poster','theme','character','groups']));

ALTER TABLE game_puzzles DROP CONSTRAINT IF EXISTS game_puzzles_target;
ALTER TABLE game_puzzles ADD CONSTRAINT game_puzzles_target CHECK (
      (mode = 'character' AND char_ids IS NOT NULL AND array_length(char_ids, 1) > 0)
   OR (mode = 'groups'    AND groups   IS NOT NULL)
   OR (mode NOT IN ('character','groups') AND anime_id IS NOT NULL)
);
