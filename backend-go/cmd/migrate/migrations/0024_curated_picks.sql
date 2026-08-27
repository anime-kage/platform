-- Editorially curated placements: the landing collage, the home
-- spotlight, and the two catalog recommendations stop being "whatever scored
-- highest" and become someone's choice.
--
-- Polymorphic by two nullable FKs plus a CHECK rather than a (media_type, id)
-- pair, same as `comments` does for episode/chapter: this way the database
-- still enforces that the referenced title exists, and ON DELETE CASCADE
-- removes a pick automatically when its title leaves the catalog. A bare
-- integer column could not do either, and a slot pointing at a deleted anime
-- would 500 the home page.
CREATE TABLE public.curated_picks (
  id       serial PRIMARY KEY,
  slot     text NOT NULL,
  -- display order within the slot, 0-based
  position integer NOT NULL,
  anime_id integer REFERENCES public.anime(id) ON DELETE CASCADE,
  manga_id integer REFERENCES public.manga(id) ON DELETE CASCADE,
  created_by integer REFERENCES public.users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  -- exactly one target
  CONSTRAINT curated_picks_one_target CHECK (
    (anime_id IS NOT NULL AND manga_id IS NULL)
    OR (anime_id IS NULL AND manga_id IS NOT NULL)
  ),
  -- one title per position per slot; replacing a slot rewrites these rows
  CONSTRAINT curated_picks_slot_position UNIQUE (slot, position)
);

-- the read path: every lookup is "give me this slot, in order"
CREATE INDEX curated_picks_slot_idx ON public.curated_picks (slot, position);
