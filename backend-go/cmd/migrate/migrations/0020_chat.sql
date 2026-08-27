-- Live chat. Persisted so the panel isn't empty when someone opens
-- it, and so moderation has something to act on — an in-memory ring buffer
-- would lose both across a restart.
--
-- Rows are small (a body caps at 500 chars) and a retention job trims them, so
-- this table stays in the low megabytes even for a busy day.
CREATE TABLE chat_messages (
  id          bigserial PRIMARY KEY,
  user_id     integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  body        text    NOT NULL CHECK (length(body) BETWEEN 1 AND 500),
  -- Twitch-style reply: the quoted context is denormalised, not a foreign key.
  -- The quote is a snapshot of what was answered; it must not change or vanish
  -- when the original is edited or deleted.
  reply_to_user    text,
  reply_to_excerpt text,
  -- moderator delete is a tombstone: the row stays so the id sequence and any
  -- audit trail hold, but nothing renders
  deleted_at  timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- the only read pattern: the newest N messages, and the retention sweep
CREATE INDEX chat_messages_created_idx ON chat_messages (created_at DESC);
