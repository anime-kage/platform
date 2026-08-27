-- Custom user lists ("Liste"): curated collections of anime/manga with
-- per-item notes. Public lists are browsable by anyone; private ones only by
-- their owner. Distinct from watchlist/readlist (status tracking) — these are
-- editorial ("Top isekai", "De plâns garantat").

CREATE TABLE user_lists (
  id serial PRIMARY KEY,
  user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
  description text,
  is_public boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX user_lists_user_idx ON user_lists (user_id, updated_at DESC);
CREATE INDEX user_lists_public_idx ON user_lists (is_public, updated_at DESC);

CREATE TABLE user_list_items (
  id serial PRIMARY KEY,
  list_id integer NOT NULL REFERENCES user_lists(id) ON DELETE CASCADE,
  anime_id integer REFERENCES anime(id) ON DELETE CASCADE,
  manga_id integer REFERENCES manga(id) ON DELETE CASCADE,
  note text,
  position integer NOT NULL DEFAULT 0,
  added_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((anime_id IS NOT NULL) <> (manga_id IS NOT NULL)),
  UNIQUE (list_id, anime_id),
  UNIQUE (list_id, manga_id)
);

CREATE INDEX user_list_items_list_idx ON user_list_items (list_id, position, id);
