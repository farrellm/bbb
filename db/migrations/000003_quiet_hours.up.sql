-- Quiet hours: matching events that arrive while a user's quiet hours are
-- on wait here, then go out as one push when they end.

SET lock_timeout = '5s';
SET statement_timeout = '30s';

CREATE TABLE held_events (
    user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    event_id bigint NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    held_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, event_id)
);

-- Quiet hours are both set or both unset, and never empty. preferences
-- has a row per parent, so validating in this transaction is brief.
-- squawk-ignore constraint-missing-not-valid
ALTER TABLE preferences ADD CONSTRAINT preferences_quiet_hours_check
CHECK (
    (quiet_start IS NULL) = (quiet_end IS NULL)
    AND (quiet_start IS NULL OR quiet_start <> quiet_end)
);
