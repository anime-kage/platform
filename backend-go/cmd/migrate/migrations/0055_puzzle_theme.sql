-- Which opening or ending a theme puzzle is asking about.
--
-- A series has several (Fullmetal Alchemist: Brotherhood has ten), so the
-- anime_id alone does not identify the question. Stored rather than derived
-- from the puzzle id so the same day always plays the same track even if the
-- harvest later adds or reorders themes for that series.
ALTER TABLE game_puzzles
  ADD COLUMN theme_id bigint REFERENCES game_themes(id) ON DELETE SET NULL;
