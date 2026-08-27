--: the release pipeline.
-- A release is one translator's episode in flight: video + EN sub in OUR
-- staging storage, RO draft as rows (not a file — autosave, diff view, and
-- Claude translation windows all want events). Staging is transient: deleted
-- at publish/reject, 30-day auto-expiry for abandoned drafts.
--
-- State machine: draft → in_review → approved → published
--                      ↘ changes_requested (back to the translator, with notes)

CREATE TABLE public.releases (
  id             serial PRIMARY KEY,
  anime_id       integer NOT NULL REFERENCES public.anime(id) ON DELETE CASCADE,
  episode_number integer NOT NULL,
  uploader_id    integer NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
  reviewer_id    integer REFERENCES public.users(id) ON DELETE SET NULL,
  state          text NOT NULL DEFAULT 'draft',
  -- video file path relative to STAGING_DIR ("<id>/video.mp4"); NULL once cleaned
  staging_path   text,
  review_notes   text,
  created_at     timestamp with time zone NOT NULL DEFAULT now(),
  updated_at     timestamp with time zone NOT NULL DEFAULT now(),

  CONSTRAINT releases_state_check CHECK
    (state IN ('draft', 'in_review', 'changes_requested', 'approved', 'published')),
  CONSTRAINT releases_episode_positive CHECK (episode_number > 0)
);

CREATE INDEX releases_state_idx ON public.releases (state, updated_at);
CREATE INDEX releases_uploader_idx ON public.releases (uploader_id);

-- One subtitle line. en_text is the source, ro_text the draft; `edited` marks
-- human lines — auto-translate (4.3) only ever fills rows where ro_text = ''.
CREATE TABLE public.subtitle_events (
  id         serial PRIMARY KEY,
  release_id integer NOT NULL REFERENCES public.releases(id) ON DELETE CASCADE,
  idx        integer NOT NULL,
  start_ms   integer NOT NULL,
  end_ms     integer NOT NULL,
  en_text    text NOT NULL DEFAULT '',
  ro_text    text NOT NULL DEFAULT '',
  edited     boolean NOT NULL DEFAULT false,

  CONSTRAINT subtitle_events_unique UNIQUE (release_id, idx),
  CONSTRAINT subtitle_events_range_check CHECK (start_ms >= 0 AND end_ms >= start_ms)
);
