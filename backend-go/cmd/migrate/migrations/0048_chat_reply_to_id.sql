-- Let a reply point at the message it answers, so the client can jump to it.
--
-- Until now a reply carried only reply_to_user and reply_to_excerpt: enough to
-- render the quote, but not to find the original. Matching on the excerpt was
-- the alternative and it is worse than nothing here — two identical messages
-- from the same person are common in a chat, and jumping to the wrong one is
-- more confusing than not jumping at all.
--
-- ON DELETE SET NULL rather than CASCADE: a moderator removing a message must
-- not silently delete every reply to it. The quote text stays, so the reply
-- still reads correctly, it just stops being clickable.
ALTER TABLE chat_messages
  ADD COLUMN IF NOT EXISTS reply_to_id bigint
  REFERENCES chat_messages (id) ON DELETE SET NULL;

-- Replies are looked up by their target when a message is deleted.
CREATE INDEX IF NOT EXISTS chat_messages_reply_to_idx
  ON chat_messages (reply_to_id) WHERE reply_to_id IS NOT NULL;
