-- Site announcements ("Știri & anunțuri"): the one strip on /home that had no
-- table behind it and was rendering a hardcoded demo array.
--
-- Written by the team, not derived from events. A derived feed (newest release,
-- newest thread) was the tempting shortcut, but the two columns next to it on
-- the home page already show exactly those things; an announcement is the thing
-- nothing else can say — a rule change, a new feature, a planned downtime.
--
-- `is_published` rather than deleting a draft: an announcement is written before
-- it goes out, and pulling one back must not lose the text.
CREATE TABLE IF NOT EXISTS announcements (
    id           serial PRIMARY KEY,
    -- short label rendered as the accent kicker ("Adăugat", "Comunitate", …).
    -- Free text on purpose: the set of things worth announcing changes faster
    -- than a CHECK constraint can be migrated. Length is capped in the handler.
    tag          text        NOT NULL,
    title        text        NOT NULL,
    -- optional second line; the home card clamps it, longer text can live in a
    -- forum thread the `url` points at
    body         text,
    -- optional destination: internal path ("/anime/12") or absolute https URL
    url          text,
    is_published boolean     NOT NULL DEFAULT true,
    author_id    integer     REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- the read path is always "published ones, newest first"
CREATE INDEX IF NOT EXISTS announcements_feed_idx
    ON announcements (created_at DESC) WHERE is_published;
