-- Romanian metadata + verifier assignment.
--
-- synopsis_romanian: the description we actually show — auto-translated at
-- import when the translator is configured, hand-editable by coordinators.
-- assigned_verifier_id: who a release is routed to for verification (soft
-- rule: filters queues; coordinators/admins can always act and reassign).
-- last_verifier_id: the translator's last pick, used as the default.

ALTER TABLE anime ADD COLUMN synopsis_romanian text;
ALTER TABLE manga ADD COLUMN synopsis_romanian text;

ALTER TABLE releases ADD COLUMN assigned_verifier_id integer REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX releases_assigned_idx ON releases (assigned_verifier_id, state);

ALTER TABLE users ADD COLUMN last_verifier_id integer REFERENCES users(id) ON DELETE SET NULL;
