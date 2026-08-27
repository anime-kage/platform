-- Community forum: persisted threads and replies. Replaces the
-- demo THREADS array on the /comunitate Forum tab. Categories are validated in
-- the handler against a fixed allowlist; pinning/locking are moderator powers.
-- reply_count and last_activity_at are denormalised on the thread so the list
-- view sorts and renders without touching forum_replies.

CREATE TABLE forum_threads (
  id serial PRIMARY KEY,
  user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  category text NOT NULL,
  title text NOT NULL CHECK (char_length(title) BETWEEN 3 AND 160),
  body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 8000),
  is_pinned boolean NOT NULL DEFAULT false,
  is_locked boolean NOT NULL DEFAULT false,
  reply_count integer NOT NULL DEFAULT 0,
  last_activity_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now()
);

-- pinned first, then most-recently-active — the default list order
CREATE INDEX forum_threads_activity_idx ON forum_threads (is_pinned DESC, last_activity_at DESC);
CREATE INDEX forum_threads_cat_idx ON forum_threads (category, last_activity_at DESC);

CREATE TABLE forum_replies (
  id serial PRIMARY KEY,
  thread_id integer NOT NULL REFERENCES forum_threads(id) ON DELETE CASCADE,
  user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 8000),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX forum_replies_thread_idx ON forum_replies (thread_id, created_at);
