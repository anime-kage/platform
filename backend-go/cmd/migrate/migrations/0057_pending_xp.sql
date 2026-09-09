-- Experience earned before choosing a faction.
--
-- Until now that XP was simply dropped: the award writes into game_faction_xp
-- keyed by the profile's faction, and with none set the statement matched no
-- row. Gold still landed, because gold lives on the profile itself, so a new
-- player could win five rounds, watch their gold climb, and have nothing to
-- show for it in levels -- with no way to get it back once they finally picked
-- a side.
--
-- It is held here instead, and moved into the first faction the player
-- chooses. A column rather than a sentinel row in game_faction_xp: "unassigned"
-- is not a faction, and putting it there would leak into every query that
-- lists, ranks or sums factions.
ALTER TABLE game_profiles
  ADD COLUMN pending_xp integer NOT NULL DEFAULT 0
  CONSTRAINT game_profiles_pending_xp_sane CHECK (pending_xp >= 0);

-- The ledger recorded every award all along, including the ones that never
-- reached a faction, so the XP these players lost is recoverable. Only for
-- profiles still without a faction: once one is set we cannot tell which
-- ledger rows predate the choice, and the rest are already banked.
UPDATE game_profiles p
   SET pending_xp = GREATEST(
         COALESCE((SELECT sum(l.xp_delta) FROM game_ledger l WHERE l.user_id = p.user_id), 0), 0)
 WHERE p.faction IS NULL;
