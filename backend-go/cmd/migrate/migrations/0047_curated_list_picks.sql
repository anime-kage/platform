-- The Vitrină could only feature a title. "Listă remarcată" on /liste needs to
-- point at a LIST, which curated_picks had no way to express: its CHECK
-- required exactly one of anime_id/manga_id, so a list reference could not be
-- stored at all.
--
-- Until now that block showed whichever seeded editorial list had the highest
-- invented like count. The seeds are gone, so without this the slot has nothing
-- real to point at except a computed platform list with no author.
--
-- A third nullable FK rather than a separate table: every other property of a
-- pick (slot, position, who set it, when) is identical, and CuratedSlot already
-- switches on which target is non-null.
ALTER TABLE public.curated_picks
  ADD COLUMN IF NOT EXISTS list_id integer REFERENCES public.user_lists(id) ON DELETE CASCADE;

-- Widen "exactly one target" from two to three. Dropped and recreated because a
-- CHECK cannot be altered in place.
ALTER TABLE public.curated_picks
  DROP CONSTRAINT IF EXISTS curated_picks_one_target;

ALTER TABLE public.curated_picks
  ADD CONSTRAINT curated_picks_one_target CHECK (
    (anime_id IS NOT NULL)::int
  + (manga_id IS NOT NULL)::int
  + (list_id  IS NOT NULL)::int = 1
  );

-- A featured list that gets deleted should vacate the slot, not leave a row
-- pointing at nothing. ON DELETE CASCADE above does that; this index keeps the
-- cascade lookup cheap and is the only access path by list.
CREATE INDEX IF NOT EXISTS curated_picks_list_idx
  ON public.curated_picks (list_id) WHERE list_id IS NOT NULL;
