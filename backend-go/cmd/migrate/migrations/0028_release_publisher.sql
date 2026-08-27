-- Record who published a release.
--
-- The pipeline already names the translator (`uploader_id`) and the verifier
-- (`reviewer_id`), but publishing — the coordinator's call that actually puts
-- a release in front of members — left no trace beyond `updated_at`. Those
-- are three distinct people doing three distinct jobs, and the episode page
-- credits only two of them.
--
-- Nullable with no backfill on purpose: releases published before this
-- migration have no recorded publisher, and inventing one (the uploader, the
-- reviewer, an admin) would be a plausible-looking lie in a credit line. They
-- simply show two credits instead of three.
ALTER TABLE public.releases
  ADD COLUMN published_by integer REFERENCES public.users(id) ON DELETE SET NULL,
  ADD COLUMN published_at timestamptz;
