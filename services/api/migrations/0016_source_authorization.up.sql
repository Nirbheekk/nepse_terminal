-- 0016_source_authorization: extend data_sources with explicit authorization metadata.
ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS permission_status text NOT NULL DEFAULT 'PERMISSION_UNKNOWN'
    CHECK (permission_status IN ('APPROVED','PERMISSION_UNKNOWN','RESTRICTED','UNAVAILABLE','REFERENCE_ONLY'));
ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS license_evidence text NOT NULL DEFAULT '';
ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS permission_reviewed_at timestamptz;
ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS attribution_requirements text NOT NULL DEFAULT '';
ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS commercial_use_allowed boolean NOT NULL DEFAULT false;
ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS allowed_request_frequency text NOT NULL DEFAULT '';
ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS historical_coverage text NOT NULL DEFAULT '';
ALTER TABLE data_sources ADD COLUMN IF NOT EXISTS last_successful_retrieval timestamptz;
