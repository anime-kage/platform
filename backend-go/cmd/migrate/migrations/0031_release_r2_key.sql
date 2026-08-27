-- Keep the uploaded video in R2 instead of copying it to local disk.
--
-- Until now a direct-to-R2 upload was pulled into staging and the object deleted,
-- so staging still had to hold every in-flight release: translator count × the
-- release cap × ~1.5 GB. That is what made the cap a *disk* limit rather than a
-- workflow one, and what made a nearly-full disk a real operational worry.
--
-- ffmpeg reads https with range requests, so nothing actually requires the file
-- to be local: subtitle extraction, the preview and the burn can all work from a
-- short-lived presigned URL. The video stays in the bucket for the life of the
-- release and is deleted with the same purge that drops staging after publish.
--
-- staging_path is kept, not replaced. Releases uploaded through the older paths
-- still have their video on disk, and the burn writes its output there — so a
-- release now has a video in exactly one of two places, and r2_key IS NOT NULL is
-- how you tell which.
ALTER TABLE public.releases ADD COLUMN r2_key text;

-- The purge and the janitor both ask "which releases still hold bytes somewhere".
CREATE INDEX releases_r2_key_idx ON public.releases (id) WHERE r2_key IS NOT NULL;
