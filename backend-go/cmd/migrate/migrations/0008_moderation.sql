--: moderation queue. Reporting has existed since the baseline
-- (comments.is_reported) but nothing consumed it; bans are new.
--
-- A ban blocks login and posting (checked at those endpoints — JWTs stay
-- stateless). Admins cannot be banned (guarded in the handler).

ALTER TABLE public.users
  ADD COLUMN is_banned boolean NOT NULL DEFAULT false;

-- the queue reads exactly this slice
CREATE INDEX comments_reported_idx ON public.comments (created_at DESC)
  WHERE is_reported = true AND is_deleted = false;
