-- A pinned chat message: one at a time, held at the top of the room.
--
-- Columns on chat_messages rather than a side table, because a pin is a
-- property of a message and dies with it: the ON DELETE CASCADE that already
-- removes a user's messages takes the pin with it, and nothing can end up
-- pointing at a message that is gone.
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS pinned_at timestamptz;
ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS pinned_by integer
  REFERENCES users(id) ON DELETE SET NULL;

-- Exactly one pin, enforced by the database rather than by remembering to
-- unpin first. The index is on a constant, so at most one row may satisfy the
-- WHERE clause at any time; a second pin fails loudly instead of quietly
-- leaving the room with two messages claiming the top slot.
CREATE UNIQUE INDEX IF NOT EXISTS chat_messages_single_pin
  ON chat_messages ((true)) WHERE pinned_at IS NOT NULL;
