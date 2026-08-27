-- Invite-only registration. The landing page already sells this
-- flow: join Discord → `/invitație` → a single-use code → register with it.
-- The Discord bot (cmd/discordbot) mints rows here; POST /api/auth/register
-- redeems one when INVITE_ONLY is on.
--
-- The code is the secret, so it is the natural primary key from the outside;
-- id stays for FK-friendliness. discord_user_id is a snowflake, hence text —
-- it exceeds int4 and is not ours to renumber.

CREATE TABLE public.invites (
  id serial PRIMARY KEY,
  -- stored upper-case; the register endpoint upper-cases before lookup so the
  -- code is effectively case-insensitive for anyone typing it by hand
  code text NOT NULL UNIQUE,

  discord_user_id  text NOT NULL,
  discord_username text,

  created_at timestamptz NOT NULL DEFAULT now(),
  -- NULL = never expires; the bot sets this from INVITE_TTL
  expires_at timestamptz,

  -- claimed exactly once: both columns are set together, in one statement
  used_by_user_id integer REFERENCES public.users(id) ON DELETE SET NULL,
  used_at         timestamptz,

  CONSTRAINT invites_used_together CHECK (
    (used_by_user_id IS NULL AND used_at IS NULL)
    OR (used_by_user_id IS NOT NULL AND used_at IS NOT NULL)
  )
);

-- the per-user daily quota lookup ("did you already mint one in the last 24h?")
CREATE INDEX invites_issuer_idx ON public.invites (discord_user_id, created_at DESC);

-- finding a member's still-unclaimed code, so re-running the command re-shows
-- it instead of burning quota and littering the table with dead codes
CREATE INDEX invites_outstanding_idx ON public.invites (discord_user_id)
  WHERE used_at IS NULL;
