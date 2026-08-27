-- The weekly programme ("Programul săptămânii"), decided by the team.
--
-- Replaces a schedule derived from MAL's broadcast_day/broadcast_time, which
-- answered the wrong question: that is when a series airs in Japan, not when a
-- Romanian subtitle lands here. With a hand-curated catalog those two have
-- nothing to do with each other — a coordinator knows when episode 5 will be
-- published because they know who is translating it.
--
-- `scheduled_at` is a timestamptz, i.e. a real instant, not a weekday name and
-- a wall-clock string. The old pair could only be rendered as "23:30 JST" and
-- left every member to do the conversion; an instant renders in each viewer's
-- own timezone with no arithmetic and no ambiguity about which day it falls on.
CREATE TABLE IF NOT EXISTS schedule_slots (
    id             serial      PRIMARY KEY,
    anime_id       integer     NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    episode_number integer     NOT NULL CHECK (episode_number > 0),
    scheduled_at   timestamptz NOT NULL,
    -- optional one-liner shown under the title ("parte 1", "întârziat o zi")
    note           text,
    created_by     integer     REFERENCES users(id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    -- one slot per episode: scheduling the same episode twice is a mistake,
    -- and this turns "schedule it again" into an update instead of a duplicate
    UNIQUE (anime_id, episode_number)
);

-- the read path is always a date window, newest-first or soonest-first
CREATE INDEX IF NOT EXISTS schedule_slots_when_idx ON schedule_slots (scheduled_at);
