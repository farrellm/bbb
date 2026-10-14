-- Pingry BBB Watch schema. See docs/DESIGN.md, "Data model".

SET lock_timeout = '5s';
SET statement_timeout = '30s';

CREATE TYPE event_kind AS ENUM ('new', 'edit', 'moved', 'removed');

CREATE TABLE users (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    google_sub text NOT NULL UNIQUE,
    email text NOT NULL,
    name text NOT NULL DEFAULT '',
    picture_url text NOT NULL DEFAULT '',
    is_admin boolean NOT NULL DEFAULT FALSE,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz
);

CREATE TABLE invites (
    code text PRIMARY KEY,
    created_by bigint REFERENCES users (id) ON DELETE SET NULL,
    max_uses integer NOT NULL DEFAULT 1 CHECK (max_uses > 0),
    uses integer NOT NULL DEFAULT 0 CHECK (uses >= 0),
    expires_at timestamptz,
    revoked boolean NOT NULL DEFAULT FALSE,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- iOS PWA sign-in handoff; rows older than 10 minutes are dead.
CREATE TABLE login_handoffs (
    nonce text PRIMARY KEY,
    user_id bigint REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE TABLE push_subscriptions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    endpoint text NOT NULL UNIQUE,
    p256dh text NOT NULL,
    auth text NOT NULL,
    user_agent text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_ok_at timestamptz
);

CREATE INDEX push_subscriptions_user_id_idx ON push_subscriptions (user_id);

CREATE TABLE preferences (
    user_id bigint PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    divisions text[] NOT NULL DEFAULT '{all,lower,middle,upper}',
    notify_new boolean NOT NULL DEFAULT TRUE,
    notify_edit boolean NOT NULL DEFAULT TRUE,
    notify_removed boolean NOT NULL DEFAULT FALSE,
    -- only notify for is_signup items
    signups_only boolean NOT NULL DEFAULT FALSE,
    -- signups notify even outside the chosen divisions
    signups_any_division boolean NOT NULL DEFAULT FALSE,
    min_edit_words integer NOT NULL DEFAULT 1 CHECK (min_edit_words >= 0),
    -- pushes inside quiet hours are held, then sent as a summary
    quiet_start time,
    quiet_end time
);

CREATE TABLE scrape_runs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    http_status integer,
    body_hash text,
    -- body_hash matched the previous run, so parsing was skipped
    unchanged boolean NOT NULL DEFAULT FALSE,
    item_count integer,
    suspect boolean NOT NULL DEFAULT FALSE,
    error text
);

CREATE TABLE items (
    panel_id bigint PRIMARY KEY,
    division text NOT NULL CHECK (
        division IN ('all', 'lower', 'middle', 'upper')
    ),
    position integer NOT NULL,
    -- set right after the item's first version is inserted
    current_version_id bigint,
    first_seen_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    removed_at timestamptz,
    is_signup boolean NOT NULL DEFAULT FALSE,
    deadline date
);

CREATE TABLE item_versions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    panel_id bigint NOT NULL REFERENCES items (panel_id) ON DELETE CASCADE,
    seen_at timestamptz NOT NULL,
    title text NOT NULL,
    subheader text NOT NULL,
    body_html text NOT NULL,
    body_text text NOT NULL,
    content_hash text NOT NULL,
    signup_links text[] NOT NULL DEFAULT '{}'
);

CREATE INDEX item_versions_panel_id_idx ON item_versions (panel_id, id);

-- items and item_versions reference each other, so this key is added once
-- both exist. The tables are empty here, so it takes no meaningful lock.
-- squawk-ignore adding-foreign-key-constraint
ALTER TABLE items ADD CONSTRAINT items_current_version_id_fkey
FOREIGN KEY (current_version_id) REFERENCES item_versions (id);

CREATE TABLE events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id bigint NOT NULL REFERENCES scrape_runs (id),
    panel_id bigint NOT NULL REFERENCES items (panel_id) ON DELETE CASCADE,
    kind event_kind NOT NULL,
    from_version_id bigint REFERENCES item_versions (id),
    to_version_id bigint REFERENCES item_versions (id),
    -- panel id of a recently removed item with the same title
    repost_of bigint REFERENCES items (panel_id) ON DELETE SET NULL,
    became_signup boolean NOT NULL DEFAULT FALSE,
    words_added integer NOT NULL DEFAULT 0,
    words_removed integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX events_panel_id_idx ON events (panel_id);
CREATE INDEX events_run_id_idx ON events (run_id);

CREATE TABLE deliveries (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id bigint NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    subscription_id bigint REFERENCES push_subscriptions (id) ON DELETE SET NULL,
    status text NOT NULL,
    attempted_at timestamptz NOT NULL DEFAULT now(),
    error text
);

CREATE INDEX deliveries_event_id_idx ON deliveries (event_id);

-- unread badges
CREATE TABLE reads (
    user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    event_id bigint NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    read_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, event_id)
);
