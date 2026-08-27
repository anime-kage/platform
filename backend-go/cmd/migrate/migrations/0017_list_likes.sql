-- List likes: members can like public custom lists; the browse tab ranks
-- "most popular" by these counts. One like per (list, user).
CREATE TABLE IF NOT EXISTS list_likes (
    id         serial PRIMARY KEY,
    list_id    integer NOT NULL REFERENCES user_lists(id) ON DELETE CASCADE,
    user_id    integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamp NOT NULL DEFAULT now(),
    UNIQUE (list_id, user_id)
);

CREATE INDEX IF NOT EXISTS list_likes_list_idx ON list_likes (list_id);
