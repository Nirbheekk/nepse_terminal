-- 0005_market_surface: per-security reported capitalisation and exchange status.
--
-- Both additions carry a value the source actually publishes, so a company
-- screen can show a market capitalisation that is measured rather than inferred.
-- Where the source has no figure the column stays NULL and the API says which
-- method produced the number it serves.

-- The exchange publishes marketCapitalization alongside each security's daily
-- trade block. It was being read by the snapshot adapter and then dropped by the
-- normaliser, so every company-level capitalisation had to be guessed as
-- close x shares. Storing it makes the reported figure the primary one.
ALTER TABLE daily_ohlcv ADD COLUMN market_cap NUMERIC(28,4);
COMMENT ON COLUMN daily_ohlcv.market_cap IS
    'Issuer-reported capitalisation for the session; NULL when the source omitted it.';

-- The exchange's own market state (isOpen) with the timestamp it applies to.
-- This is an observation, not a derived guess: "closed" at 15:00 is a fact from
-- the feed, whereas inferring it from the clock would break on holidays.
CREATE TABLE market_status_snapshots (
    status          text NOT NULL
                    CHECK (status IN ('open', 'preopen', 'closed', 'unknown')),
    reported_status text NOT NULL,
    as_of           timestamptz NOT NULL,
    observed_at     timestamptz NOT NULL DEFAULT now(),
    data_source_id  uuid REFERENCES data_sources(id),
    ingested_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (status, as_of)
);
CREATE INDEX market_status_recent_idx
    ON market_status_snapshots (as_of DESC, observed_at DESC);

INSERT INTO dataset_slas (stream, description, owner, max_age_seconds, severity) VALUES
    ('market_status', 'Exchange open/closed state', 'collector', 3600, 'warning')
ON CONFLICT (stream) DO NOTHING;

-- Filters and sorts on the company screen read these columns.
CREATE INDEX companies_sector_symbol_idx ON companies (sector_id, symbol);
CREATE INDEX companies_name_lower_idx ON companies (lower(name));
