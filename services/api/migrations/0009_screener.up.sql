-- 0009_screener: saved screens, watchlists and the alert pipeline.
--
-- Every table here is owned by a user and every query filters on user_id, so a
-- tenant boundary is a property of the schema rather than a convention handlers
-- might forget. The composite uniqueness constraints are scoped the same way:
-- two tenants may both keep a list called "Banks".
--
-- Alert delivery is split into an event (what was observed, deduplicated) and a
-- delivery (the attempt to tell someone). Keeping them apart is what makes a
-- retry idempotent: retrying a delivery must not create a second event, and a
-- suppressed event must still be counted even though nothing was sent.

CREATE TABLE screening_rules (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
    -- The validated definition, stored as given so a screen can be re-run and
    -- diffed later even after the metric registry grows.
    definition  jsonb NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Case-insensitive per-owner uniqueness: "Banks" and "banks" are one screen.
CREATE UNIQUE INDEX screening_rules_owner_name_key
    ON screening_rules (user_id, lower(btrim(name)));
CREATE INDEX screening_rules_owner_idx ON screening_rules (user_id, updated_at DESC);

CREATE TABLE watchlists (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX watchlists_owner_name_key
    ON watchlists (user_id, lower(btrim(name)));
CREATE INDEX watchlists_owner_idx ON watchlists (user_id, updated_at DESC);

CREATE TABLE watchlist_items (
    watchlist_id uuid NOT NULL REFERENCES watchlists(id) ON DELETE CASCADE,
    symbol       text NOT NULL,
    note         text NOT NULL DEFAULT '' CHECK (length(note) <= 200),
    added_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (watchlist_id, symbol)
);

-- A watchlist has at most this many members. The cap is enforced in the store so
-- a runaway client cannot grow one list without bound.
COMMENT ON TABLE watchlist_items IS 'Members of a watchlist; at most 500 per list (enforced in store).';

CREATE TABLE alert_rules (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name             text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    -- symbol NULL means "any security this screen matches".
    symbol           text,
    screen           jsonb,
    metric           text NOT NULL,
    comparator       text NOT NULL CHECK (comparator IN ('gt','gte','lt','lte','eq')),
    threshold        numeric(24,6) NOT NULL,
    cooldown_seconds integer NOT NULL DEFAULT 3600
                     CHECK (cooldown_seconds BETWEEN 0 AND 604800),
    -- Attempts per delivery before it is left failed for an operator to see.
    max_attempts     integer NOT NULL DEFAULT 3 CHECK (max_attempts BETWEEN 1 AND 10),
    enabled          boolean NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    -- A rule must name a subject: a symbol, or a screen, or both.
    CHECK (symbol IS NOT NULL OR screen IS NOT NULL)
);

CREATE INDEX alert_rules_owner_idx ON alert_rules (user_id, created_at DESC);
CREATE INDEX alert_rules_enabled_idx ON alert_rules (enabled) WHERE enabled;

CREATE TABLE alert_events (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id          uuid NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol           text NOT NULL,
    session_date     date NOT NULL,
    metric           text NOT NULL,
    metric_value     numeric(24,6),
    threshold        numeric(24,6) NOT NULL,
    comparator       text NOT NULL,
    -- sha256 of rule|symbol|session|metric|value. The unique constraint below is
    -- the deduplication guarantee: the same observation cannot be stored twice,
    -- even if two workers race.
    fingerprint      text NOT NULL,
    first_seen_at    timestamptz NOT NULL DEFAULT now(),
    last_seen_at     timestamptz NOT NULL DEFAULT now(),
    -- Observations of the same condition that were deliberately not re-sent.
    suppressed_count integer NOT NULL DEFAULT 0,
    UNIQUE (rule_id, fingerprint)
);

CREATE INDEX alert_events_owner_idx ON alert_events (user_id, first_seen_at DESC);
CREATE INDEX alert_events_rule_symbol_idx ON alert_events (rule_id, symbol, first_seen_at DESC);

CREATE TABLE alert_deliveries (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        uuid NOT NULL REFERENCES alert_events(id) ON DELETE CASCADE,
    rule_id         uuid NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel         text NOT NULL DEFAULT 'in_app'
                    CHECK (channel IN ('in_app')),
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','delivered','failed')),
    attempts        integer NOT NULL DEFAULT 0,
    max_attempts    integer NOT NULL DEFAULT 3,
    last_error      text,
    next_attempt_at timestamptz,
    payload         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    delivered_at    timestamptz
);

CREATE INDEX alert_deliveries_owner_idx ON alert_deliveries (user_id, created_at DESC);
-- The retry scan needs only the rows still owed an attempt.
CREATE INDEX alert_deliveries_retry_idx
    ON alert_deliveries (next_attempt_at)
    WHERE status = 'pending';
