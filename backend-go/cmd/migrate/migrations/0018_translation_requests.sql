-- Translation requests ("Cereri"): members ask for a series to be subtitled and
-- vote on each other's asks. Deduped by canonical MAL id (resolved from
-- Jikan/AniList at submit time) so the same series can't be requested twice.
CREATE TABLE IF NOT EXISTS translation_requests (
    id         serial PRIMARY KEY,
    user_id    integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    medium     text    NOT NULL CHECK (medium IN ('anime', 'manga')),
    mal_id     integer,
    title      text    NOT NULL,
    image_url  text,
    note       text,
    status     text    NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'in_progress', 'approved', 'rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- one request per (medium, resolved series) — the dedup guarantee
CREATE UNIQUE INDEX IF NOT EXISTS translation_requests_medium_mal_uniq
    ON translation_requests (medium, mal_id) WHERE mal_id IS NOT NULL;
-- fallback dedup for unresolved titles: case-insensitive per medium
CREATE UNIQUE INDEX IF NOT EXISTS translation_requests_medium_title_uniq
    ON translation_requests (medium, lower(title)) WHERE mal_id IS NULL;

CREATE TABLE IF NOT EXISTS request_votes (
    id         serial PRIMARY KEY,
    request_id integer NOT NULL REFERENCES translation_requests(id) ON DELETE CASCADE,
    user_id    integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (request_id, user_id)
);

CREATE INDEX IF NOT EXISTS request_votes_request_idx ON request_votes (request_id);
