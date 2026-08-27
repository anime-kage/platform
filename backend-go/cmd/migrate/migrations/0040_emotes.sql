-- Custom chat emotes, uploaded by the team (7TV/Twitch style).
--
-- Replaces the hardcoded emoji map in the frontend, which could only ever map a
-- code to a single unicode character. `image_url` is a local upload, so an emote
-- is a real picture — which is the whole point.
--
-- `width`/`height` are the image's natural size, stored at upload time. They are
-- not what the chat renders at (a fixed height does that, see below) — they are
-- kept so the admin list can flag an emote whose proportions will look wrong
-- before anyone has to notice it in chat.
CREATE TABLE IF NOT EXISTS emotes (
    id         serial      PRIMARY KEY,
    -- what people type, e.g. "Kagege". Case-sensitive on purpose: Twitch codes
    -- are, and "PogChamp" vs "pogchamp" reading as one emote surprises people.
    code       text        NOT NULL,
    image_url  text        NOT NULL,
    width      integer     NOT NULL DEFAULT 0,
    height     integer     NOT NULL DEFAULT 0,
    -- soft disable: an emote pulled from the picker still renders in the old
    -- messages that already used it, rather than turning them into raw text
    is_active  boolean     NOT NULL DEFAULT true,
    created_by integer     REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- one emote per code
CREATE UNIQUE INDEX IF NOT EXISTS emotes_code_uniq ON emotes (code);
