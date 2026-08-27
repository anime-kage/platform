--: our own subtitle tracks.
-- A .vtt per episode per language, served as <track> in our player only —
-- iframes can't carry them, which is the whole reason the own player exists.
--   status 'machine'   raw auto-translate output, not shown to users
--          'edited'    human-edited draft (release pipeline)
--          'published' live — the stream endpoint serves these
--   translator_id/source_sub are provenance for the release pipeline.

CREATE TABLE public.subtitles (
  id            serial PRIMARY KEY,
  episode_id    integer NOT NULL REFERENCES public.episodes(id) ON DELETE CASCADE,
  language      text NOT NULL,
  label         text,
  format        text NOT NULL DEFAULT 'vtt',
  url           text NOT NULL,
  status        text NOT NULL DEFAULT 'published',
  translator_id integer REFERENCES public.users(id) ON DELETE SET NULL,
  source_sub    text,
  created_at    timestamp with time zone NOT NULL DEFAULT now(),

  CONSTRAINT subtitles_format_check CHECK (format IN ('vtt', 'srt', 'ass')),
  CONSTRAINT subtitles_status_check CHECK (status IN ('machine', 'edited', 'published')),
  -- one live track per language per episode; drafts can coexist
  CONSTRAINT subtitles_published_unique UNIQUE (episode_id, language, status)
);

CREATE INDEX subtitles_episode_idx ON public.subtitles (episode_id, status);
