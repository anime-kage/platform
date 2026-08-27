-- Chat-only timeouts and bans.
--
-- Deliberately NOT users.banned_at: that suspends the whole account. This is
-- the live chat's own mute list — a timed-out user keeps reading, browsing,
-- rating and commenting, they just can't post in the room. Keeping the two
-- separate means a staff member can cool a room down without touching anyone's
-- account, and an account ban never has to be reasoned about here.
CREATE TABLE chat_restrictions (
  -- one live restriction per user: a new timeout replaces the old one rather
  -- than stacking, which is what "he did it again, mute him longer" means
  user_id    integer PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  -- NULL = permanent (a ban). A timestamp = a timeout that lapses on its own,
  -- so nothing has to run to un-mute someone.
  expires_at timestamptz,
  reason     text,
  -- who did it, kept for the audit trail; ON DELETE SET NULL so removing a
  -- staff account doesn't lift every restriction they ever handed out
  created_by integer REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- the sweep that clears lapsed timeouts (rows are only read by primary key,
-- so this index exists for the janitor, not for the send path)
CREATE INDEX chat_restrictions_expires_idx ON chat_restrictions (expires_at)
  WHERE expires_at IS NOT NULL;
