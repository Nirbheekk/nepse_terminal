-- 0014_developer_keys_exports: developer API keys, usage quotas, entitlements,
-- dataset licensing matrix, and export job tracking.

CREATE TABLE developer_keys (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id              uuid REFERENCES organizations(id) ON DELETE SET NULL,
    name                text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    prefix              text NOT NULL,
    key_hash            bytea NOT NULL UNIQUE,
    tier                text NOT NULL DEFAULT 'free' CHECK (tier IN ('free', 'pro', 'institutional', 'enterprise')),
    rate_limit_rpm      int NOT NULL DEFAULT 60,
    monthly_quota       int NOT NULL DEFAULT 10000,
    requests_this_month int NOT NULL DEFAULT 0,
    quota_reset_at      timestamptz NOT NULL DEFAULT now() + interval '30 days',
    allowed_ips         text[] NOT NULL DEFAULT '{}',
    expires_at          timestamptz,
    revoked_at          timestamptz,
    last_used_at        timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX dev_keys_user_idx ON developer_keys (user_id, revoked_at);
CREATE INDEX dev_keys_hash_idx ON developer_keys (key_hash) WHERE revoked_at IS NULL;

CREATE TABLE entitlements (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_type     text NOT NULL CHECK (subject_type IN ('user', 'org', 'developer_key')),
    subject_id       uuid NOT NULL,
    dataset          text NOT NULL CHECK (dataset IN ('market_live', 'daily_ohlcv', 'floorsheet', 'financials', 'disclosures', 'analytics', 'exports')),
    license_class    text NOT NULL DEFAULT 'public_unverified' CHECK (license_class IN (
                         'official_verified', 'third_party_licensed', 'public_unverified', 'derived', 'restricted')),
    can_redistribute boolean NOT NULL DEFAULT false,
    max_export_rows  int NOT NULL DEFAULT 10000,
    granted_at       timestamptz NOT NULL DEFAULT now(),
    expires_at       timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (subject_type, subject_id, dataset)
);
CREATE INDEX entitlements_subject_idx ON entitlements (subject_type, subject_id);

CREATE TABLE export_jobs (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id           uuid REFERENCES organizations(id) ON DELETE SET NULL,
    developer_key_id uuid REFERENCES developer_keys(id) ON DELETE SET NULL,
    dataset          text NOT NULL CHECK (dataset IN ('companies', 'daily_ohlcv', 'floorsheet', 'financials', 'disclosures', 'portfolio')),
    format           text NOT NULL CHECK (format IN ('csv', 'json', 'parquet')),
    status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled')),
    parameters       jsonb NOT NULL DEFAULT '{}',
    row_count        int NOT NULL DEFAULT 0,
    file_size_bytes  bigint NOT NULL DEFAULT 0,
    storage_path     text NOT NULL DEFAULT '',
    file_name        text NOT NULL DEFAULT '',
    sha256           text NOT NULL DEFAULT '',
    license_class    text NOT NULL DEFAULT 'public_unverified',
    attribution      text NOT NULL DEFAULT '',
    error_message    text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    completed_at     timestamptz,
    expires_at       timestamptz NOT NULL DEFAULT now() + interval '7 days'
);
CREATE INDEX export_jobs_user_idx ON export_jobs (user_id, created_at DESC);
CREATE INDEX export_jobs_status_idx ON export_jobs (status, created_at);
