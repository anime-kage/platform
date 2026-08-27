-- In-app notifications. One row per (recipient, event): a follow, a reply to
-- your comment, or a lifecycle change on a release you own (approved, changes
-- requested, published). The body text is rendered in Romanian at creation
-- time and stored verbatim so the list renders with no extra joins; actor_id
-- is kept for the avatar and survives the actor being deleted (SET NULL).
-- read_at NULL = unread; the header badge counts those.

CREATE TABLE notifications (
  id serial PRIMARY KEY,
  user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type text NOT NULL,
  actor_id integer REFERENCES users(id) ON DELETE SET NULL,
  body text NOT NULL,
  link text,
  read_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
-- the badge query hits only unread rows, so a partial index keeps it cheap
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;
