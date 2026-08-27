-- Password reset tokens.
--
-- token_hash, never the token itself: the raw token goes out in one email and
-- is never stored, so a leaked database dump cannot be used to reset anyone's
-- password. Same reasoning as storing bcrypt hashes rather than passwords.
CREATE TABLE public.password_resets (
  id         serial PRIMARY KEY,
  user_id    integer NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  -- what the requester looked like, for abuse investigation. Not used to
  -- restrict redemption: people request on a phone and click on a laptop.
  requested_ip text
);

-- the redemption lookup: hash → row
CREATE INDEX password_resets_token_idx ON public.password_resets (token_hash);
-- invalidating a user's outstanding tokens when one is spent or the password
-- changes; partial because spent rows are never touched again
CREATE INDEX password_resets_live_idx ON public.password_resets (user_id) WHERE used_at IS NULL;
