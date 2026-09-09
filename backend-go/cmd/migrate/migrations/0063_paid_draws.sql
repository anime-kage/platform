-- Paid extra draws: after the free daily one, more may be bought with gold.
--
-- Counted per day and reset by comparing last_draw, the same way the streak is,
-- so no scheduled job is needed to zero it at midnight.
ALTER TABLE game_draws ADD COLUMN extra_today integer NOT NULL DEFAULT 0;
