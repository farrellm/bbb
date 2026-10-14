DROP INDEX IF EXISTS events_unnotified_idx;
ALTER TABLE events DROP COLUMN IF EXISTS notified_at;
