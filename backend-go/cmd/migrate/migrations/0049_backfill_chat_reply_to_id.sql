-- Make replies written before 0048 clickable too.
--
-- Those rows carry only reply_to_user and reply_to_excerpt, so the target has
-- to be inferred. That inference is exactly the heuristic rejected for the LIVE
-- path, and for good reason — two identical messages from one person are normal
-- in chat, and jumping to the wrong one is worse than not jumping. The
-- difference here is that this runs once over a fixed set of rows and can
-- afford to be strict:
--
--   * the candidate must be by the user the reply names
--   * it must be OLDER than the reply
--   * its body must START WITH the stored excerpt
--   * and there must be EXACTLY ONE such message
--
-- Anything ambiguous keeps reply_to_id NULL and simply stays un-clickable,
-- which is the same behaviour as before this migration. Nothing is guessed.
--
-- The excerpt is truncated at 60 characters with a trailing ellipsis, so that
-- has to come off before comparing.
WITH resolved AS (
  SELECT r.id AS reply_id,
         (SELECT t.id
            FROM chat_messages t
            JOIN users tu ON tu.id = t.user_id
           WHERE tu.username = r.reply_to_user
             AND t.id < r.id
             AND t.deleted_at IS NULL
             AND left(t.body, length(rtrim(r.reply_to_excerpt, '…')))
                 = rtrim(r.reply_to_excerpt, '…')
           ORDER BY t.id DESC
           LIMIT 1) AS target_id,
         (SELECT count(*)
            FROM chat_messages t
            JOIN users tu ON tu.id = t.user_id
           WHERE tu.username = r.reply_to_user
             AND t.id < r.id
             AND t.deleted_at IS NULL
             AND left(t.body, length(rtrim(r.reply_to_excerpt, '…')))
                 = rtrim(r.reply_to_excerpt, '…')) AS matches
    FROM chat_messages r
   WHERE r.reply_to_user IS NOT NULL
     AND r.reply_to_excerpt IS NOT NULL
     AND r.reply_to_id IS NULL
     AND r.deleted_at IS NULL
)
UPDATE chat_messages m
   SET reply_to_id = resolved.target_id
  FROM resolved
 WHERE m.id = resolved.reply_id
   AND resolved.matches = 1
   AND resolved.target_id IS NOT NULL;
