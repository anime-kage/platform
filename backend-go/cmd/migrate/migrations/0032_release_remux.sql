-- Rewrap uploaded Matroska as MP4 so the preview plays.
--
-- Translators upload .mkv, browsers do not play .mkv, and the preview on the
-- translator page is how they confirm they sent the right episode. So an MKV
-- upload now queues a rewrap: video copied bit for bit, audio transcoded only
-- when the container cannot legally carry it, and r2_key swapped to the MP4 once
-- it lands. The MKV is deleted after the swap.
--
-- Modelled on hardsub (0030) rather than sharing its columns: the two jobs have
-- different lifetimes and a release can legitimately want both — the rewrap runs
-- once at ingest, the burn is opt-in at publish and may be re-run. Folding them
-- into one state machine would make "queued" ambiguous.
--
-- Like hardsub, progress lives in the worker's memory, not here: it moves several
-- times a second and nothing needs it to survive a restart. A restart requeues
-- the job instead, which is cheaper than the writes would have been.
ALTER TABLE public.releases
	ADD COLUMN remux_state       text,       -- NULL | queued | running | done | failed
	ADD COLUMN remux_error       text,
	ADD COLUMN remux_queued_at   timestamptz,
	ADD COLUMN remux_finished_at timestamptz;

-- The worker's claim query orders queued jobs by age; this is the index behind it.
CREATE INDEX releases_remux_queue_idx ON public.releases (remux_queued_at)
	WHERE remux_state = 'queued';
