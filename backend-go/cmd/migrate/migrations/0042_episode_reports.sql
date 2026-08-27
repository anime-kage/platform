-- Episode reports (member-facing "something is wrong with this episode").
--
-- Comment reporting already existed, but as a bare `comments.is_reported`
-- boolean: a moderator learned that someone objected and nothing about what.
-- An episode report is useless in that shape — "the third source is dead" and
-- "the skip markers are ten seconds late" need different fixes — so this one
-- carries the member's text.
--
-- user_id is nullable and ON DELETE SET NULL: a report stays actionable after
-- the reporter deletes their account. The episode reference is ON DELETE
-- CASCADE instead, because a report about an episode that no longer exists has
-- nothing left to point at.
CREATE TABLE public.episode_reports (
  id          serial PRIMARY KEY,
  episode_id  integer NOT NULL REFERENCES public.episodes(id) ON DELETE CASCADE,
  user_id     integer REFERENCES public.users(id) ON DELETE SET NULL,
  body        text NOT NULL,
  status      text NOT NULL DEFAULT 'open',
  created_at  timestamptz NOT NULL DEFAULT now(),
  resolved_at timestamptz,
  resolved_by integer REFERENCES public.users(id) ON DELETE SET NULL,

  CONSTRAINT episode_reports_status_check CHECK (status IN ('open', 'resolved')),
  -- Bounded in the database as well as the handler: this is member-submitted
  -- free text and the queue has to stay readable.
  CONSTRAINT episode_reports_body_len
    CHECK (char_length(btrim(body)) BETWEEN 1 AND 2000)
);

-- the moderation queue reads exactly this slice
CREATE INDEX episode_reports_open_idx
  ON public.episode_reports (created_at DESC) WHERE status = 'open';

-- "does this episode already have an open report" on the episode page
CREATE INDEX episode_reports_episode_idx ON public.episode_reports (episode_id);
