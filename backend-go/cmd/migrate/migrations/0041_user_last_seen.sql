-- "Membri online" on the community page (replacing the published-subtitle
-- count, which reads as noise on a catalog that is mostly imported).
--
-- There was no way to answer "who is here right now": the chat hub tracks open
-- SSE streams, but that only ever sees members with the chat panel open and
-- counts two tabs as two people. A column on users, touched by the auth
-- middleware, covers everyone making authenticated requests.
--
-- Nullable with no backfill: NULL means "not seen since this shipped", which
-- is the truth. Inventing a timestamp would put every existing member online
-- at once.
ALTER TABLE public.users ADD COLUMN last_seen_at timestamptz;

-- The online count scans by recency and nothing else.
CREATE INDEX users_last_seen_idx ON public.users (last_seen_at)
  WHERE last_seen_at IS NOT NULL;
