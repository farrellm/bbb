-- Push notifications: the notifier's progress through events, and a
-- preferences row for every account.

SET lock_timeout = '5s';
SET statement_timeout = '30s';

-- An event is pushed once, then stamped.
ALTER TABLE events ADD COLUMN notified_at timestamptz;

-- Events from before push existed are old news; don't announce them.
UPDATE events SET notified_at = created_at;

-- The notifier's work queue is the few unstamped rows. The table is small,
-- so building the index in the migration's transaction is brief.
-- squawk-ignore require-concurrent-index-creation
CREATE INDEX events_unnotified_idx ON events (id) WHERE notified_at IS NULL;

-- New accounts get their row when they are created; existing ones get the
-- defaults here.
INSERT INTO preferences (user_id)
SELECT id FROM users
ON CONFLICT DO NOTHING;
