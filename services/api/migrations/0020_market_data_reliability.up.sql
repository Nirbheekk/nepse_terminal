-- 0020_market_data_reliability: trading calendar, freshness, coverage.
CREATE TABLE IF NOT EXISTS trading_calendar (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trading_date date NOT NULL,
    is_trading_day boolean NOT NULL DEFAULT true,
    session_type text NOT NULL DEFAULT 'regular' CHECK (session_type IN ('regular','half_day','special')),
    open_time time,
    close_time time,
    timezone text NOT NULL DEFAULT 'Asia/Kathmandu',
    reason text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT '',
    effective_from date NOT NULL DEFAULT CURRENT_DATE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (trading_date)
);
CREATE INDEX IF NOT EXISTS trading_calendar_date_idx ON trading_calendar (trading_date);

CREATE TABLE IF NOT EXISTS data_freshness_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    stream text NOT NULL,
    last_observation_at timestamptz,
    last_ingestion_at timestamptz,
    status text NOT NULL DEFAULT 'UNKNOWN' CHECK (status IN ('CURRENT','DELAYED','STALE','UNAVAILABLE','NOT_EXPECTED','UNVERIFIED')),
    detail text NOT NULL DEFAULT '',
    computed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS dfs_stream_idx ON data_freshness_snapshots (stream, computed_at DESC);

CREATE TABLE IF NOT EXISTS data_coverage_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type text NOT NULL,
    entity_key text NOT NULL,
    data_type text NOT NULL,
    earliest_date date,
    latest_date date,
    observation_count bigint NOT NULL DEFAULT 0,
    missing_count bigint NOT NULL DEFAULT 0,
    computed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS dcs_entity_idx ON data_coverage_snapshots (entity_type, entity_key);
