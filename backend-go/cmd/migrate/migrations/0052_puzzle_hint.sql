-- An opt-in clue for the title puzzle, once a player is nearly out of guesses.
--
-- Six wrong guesses with nothing new to go on is a dead end, and a player who
-- gives up there learns nothing. Offering the synopsis late turns the last two
-- attempts into a real chance -- at a price, so it stays a choice rather than
-- something everyone takes by default.
--
-- Recorded on the attempt, not inferred: the reduction has to be applied when
-- the puzzle is finally solved, which can be several requests later, and the
-- ledger should be able to explain why an award was smaller than the headline
-- rate.
ALTER TABLE game_attempts
  ADD COLUMN used_hint boolean NOT NULL DEFAULT false;
