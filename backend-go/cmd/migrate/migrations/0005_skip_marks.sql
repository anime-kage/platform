--: skip intro/outro marks.
-- Resolution order at play time: our marks → AniSkip (keyed on anime.mal_id +
-- episode number; hits are cached back into this table) → nothing.
-- source 'manual' (coordinator set it, wins forever) | 'aniskip' (cached).

CREATE TABLE public.skip_marks (
  id         serial PRIMARY KEY,
  episode_id integer NOT NULL REFERENCES public.episodes(id) ON DELETE CASCADE,
  kind       text NOT NULL,
  start_s    double precision NOT NULL,
  end_s      double precision NOT NULL,
  source     text NOT NULL DEFAULT 'manual',
  created_at timestamp with time zone NOT NULL DEFAULT now(),

  CONSTRAINT skip_marks_kind_check CHECK (kind IN ('intro', 'outro')),
  CONSTRAINT skip_marks_source_check CHECK (source IN ('manual', 'aniskip')),
  CONSTRAINT skip_marks_range_check CHECK (start_s >= 0 AND end_s > start_s),
  CONSTRAINT skip_marks_unique UNIQUE (episode_id, kind)
);
