--: the native manga reader's page lists. One row per page image per
-- language edition ('ro' default, 'en' the original when we have it — the
-- reader defaults to RO and offers a togglec).
-- URLs point at our own object storage for in-house scanlations (6.3) or at third-party
-- pages via the image proxy (6.2).

CREATE TABLE public.chapter_pages (
  id         serial PRIMARY KEY,
  chapter_id integer NOT NULL REFERENCES public.chapters(id) ON DELETE CASCADE,
  language   text NOT NULL DEFAULT 'ro',
  idx        integer NOT NULL,
  url        text NOT NULL,

  CONSTRAINT chapter_pages_idx_unique UNIQUE (chapter_id, language, idx),
  CONSTRAINT chapter_pages_idx_positive CHECK (idx >= 0)
);

CREATE INDEX chapter_pages_lookup_idx ON public.chapter_pages (chapter_id, language, idx);
