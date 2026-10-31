-- 0003_ingestion: identifier, archive, run, checkpoint, SLA and reconciliation
-- tables. This migration is additive: it does not touch existing market rows.

-- ---------------------------------------------------------------- identifiers
-- ISIN is the only globally unique identifier an exchange security has. Symbols
-- are reused after delisting, so a symbol alone is not a stable key over time.
ALTER TABLE securities ADD COLUMN isin text;
ALTER TABLE securities ADD COLUMN isin_checksum_ok boolean;
ALTER TABLE securities ADD COLUMN isin_source_id uuid REFERENCES data_sources(id);

CREATE UNIQUE INDEX securities_isin_key ON securities (isin) WHERE isin IS NOT NULL;

-- ---------------------------------------------------------------- provenance
-- How the platform is allowed to obtain a source, stated explicitly rather than
-- inferred from a licence string.
ALTER TABLE data_sources ADD COLUMN access_mode text NOT NULL DEFAULT 'operator_captured';
ALTER TABLE data_sources ADD COLUMN attribution text;

-- ---------------------------------------------------------------- raw archives
-- Every ingested batch is written to an immutable gzip JSONL file before it is
-- normalised, and the digest is recorded here. This is what makes a load
-- reproducible: the exact bytes that produced a row are recoverable.
CREATE TABLE raw_archives (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_slug   text NOT NULL,
    stream        text NOT NULL,
    path          text NOT NULL,
    sha256        text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    bytes         bigint NOT NULL CHECK (bytes >= 0),
    rows          bigint NOT NULL CHECK (rows >= 0),
    captured_at   timestamptz,
    archived_at   timestamptz NOT NULL DEFAULT now(),
    licence_class text NOT NULL,
    meta          jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE UNIQUE INDEX raw_archives_digest_key ON raw_archives (stream, sha256);
CREATE INDEX raw_archives_stream_idx ON raw_archives (stream, archived_at DESC);

-- ---------------------------------------------------------------- jobs and runs
CREATE TABLE collection_jobs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job         text NOT NULL,
    status      text NOT NULL DEFAULT 'running'
                CHECK (status IN ('running', 'success', 'partial', 'failed', 'skipped')),
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    detail      jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX collection_jobs_recent_idx ON collection_jobs (started_at DESC);

CREATE TABLE ingest_runs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          uuid REFERENCES collection_jobs(id) ON DELETE SET NULL,
    source_slug     text NOT NULL,
    stream          text NOT NULL,
    status          text NOT NULL
                    CHECK (status IN ('success', 'partial', 'failed', 'skipped')),
    archive_id      uuid REFERENCES raw_archives(id) ON DELETE SET NULL,
    rows_read       integer NOT NULL DEFAULT 0,
    rows_written    integer NOT NULL DEFAULT 0,
    rows_unchanged  integer NOT NULL DEFAULT 0,
    rows_rejected   integer NOT NULL DEFAULT 0,
    cursor          text,
    rejected_sample jsonb NOT NULL DEFAULT '[]'::jsonb,
    error           text,
    started_at      timestamptz NOT NULL DEFAULT now(),
    finished_at     timestamptz
);
CREATE INDEX ingest_runs_stream_idx ON ingest_runs (stream, started_at DESC);

-- A checkpoint records the last archive successfully folded into the database.
-- Re-running with identical bytes is a no-op instead of a full re-upsert.
CREATE TABLE ingest_checkpoints (
    source_slug     text NOT NULL,
    stream          text NOT NULL,
    archive_sha256  text,
    cursor          text,
    rows_seen       bigint NOT NULL DEFAULT 0,
    last_run_at     timestamptz,
    last_success_at timestamptz,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source_slug, stream)
);

-- ---------------------------------------------------------------- freshness
CREATE TABLE dataset_slas (
    stream          text PRIMARY KEY,
    description     text NOT NULL,
    owner           text NOT NULL DEFAULT 'platform',
    max_age_seconds integer NOT NULL CHECK (max_age_seconds > 0),
    severity        text NOT NULL DEFAULT 'warning'
                    CHECK (severity IN ('info', 'warning', 'critical'))
);

INSERT INTO dataset_slas (stream, description, owner, max_age_seconds, severity) VALUES
    ('floorsheet',    'Trade-level contracts captured during the session', 'collector', 21600, 'critical'),
    ('quotes',        'Daily OHLCV per security',                          'collector', 21600, 'critical'),
    ('summary',       'Session summary block',                             'collector', 21600, 'warning'),
    ('indices',       'Headline and sector index observations',            'collector', 21600, 'warning'),
    ('market_cap',    'Capitalisation snapshot',                           'collector', 21600, 'warning'),
    ('companies',     'Company reference data',                            'platform',  2592000, 'info'),
    ('securities',    'Security reference data and identifiers',           'platform',  2592000, 'info')
ON CONFLICT (stream) DO NOTHING;

-- ---------------------------------------------------------------- reconciliation
CREATE TABLE reconciliation_runs (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    status         text NOT NULL CHECK (status IN ('match', 'mismatch', 'partial')),
    legacy_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
    platform_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
    discrepancies  jsonb NOT NULL DEFAULT '[]'::jsonb,
    checked_at     timestamptz NOT NULL DEFAULT now()
);
