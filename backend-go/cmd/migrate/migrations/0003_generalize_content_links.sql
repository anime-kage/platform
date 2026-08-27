--: generalize content_links into "sources" for the own-player
-- architecture.
--   kind='embed'   iframe fallback — every pre-existing row
--   kind='extract' resolvable to an HLS manifest by the Go resolver (3.2);
--                  provider + provider_ref identify it to the resolver registry
-- priority picks the preferred source; last_checked_at/last_ok are written by
-- the source health checker (3.8).

ALTER TABLE public.content_links
  ADD COLUMN kind text NOT NULL DEFAULT 'embed',
  ADD COLUMN provider text,
  ADD COLUMN provider_ref text,
  ADD COLUMN priority integer NOT NULL DEFAULT 0,
  ADD COLUMN last_checked_at timestamp with time zone,
  ADD COLUMN last_ok boolean;

ALTER TABLE public.content_links
  ADD CONSTRAINT content_links_kind_check
    CHECK (kind IN ('embed', 'extract')),
  ADD CONSTRAINT content_links_extract_ref_check
    CHECK (kind <> 'extract' OR (provider IS NOT NULL AND provider_ref IS NOT NULL));

-- the health checker scans active sources by staleness
CREATE INDEX content_links_health_idx
  ON public.content_links (is_active, last_checked_at);
