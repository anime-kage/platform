-- Hardsub job state.
--
-- Burning the RO track into the picture is how a subtitle reaches viewers on a
-- host we can only embed: a soft track needs a player we control, and a
-- third-party <iframe> is not one. It is opt-in — the existing publish flow
-- (soft .vtt track + source link) is unchanged and remains the default. This
-- only records the state of an optional extra artefact.
--
-- On the release rather than in its own table because a release has exactly one
-- burned output: re-burning replaces it. A job table would buy history nobody
-- has asked for and an extra join on every publish-page render.
--
-- Progress deliberately does NOT live here. It moves a few times a second for
-- ten-plus minutes, and writing that to a row would be a lot of WAL for a number
-- nobody needs after the fact. It is kept in memory by the worker (the same
-- shape as the auto-translate status) and merged into the status response;
-- losing it to a restart costs nothing, because a restart requeues the job.
ALTER TABLE public.releases
  ADD COLUMN hardsub_state       text,
  ADD COLUMN hardsub_path        text,
  ADD COLUMN hardsub_error       text,
  ADD COLUMN hardsub_queued_at   timestamptz,
  ADD COLUMN hardsub_finished_at timestamptz,
  ADD CONSTRAINT releases_hardsub_state_check CHECK (
    hardsub_state IS NULL OR hardsub_state IN ('queued', 'running', 'done', 'failed')
  );

-- The worker's claim query: oldest queued first, so the queue is FIFO and a
-- position can be counted cheaply for the UI.
CREATE INDEX releases_hardsub_queue_idx
  ON public.releases (hardsub_queued_at)
  WHERE hardsub_state IN ('queued', 'running');
