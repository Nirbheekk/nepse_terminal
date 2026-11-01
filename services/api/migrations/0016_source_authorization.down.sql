-- 0016_source_authorization down
ALTER TABLE data_sources DROP COLUMN IF EXISTS last_successful_retrieval;
ALTER TABLE data_sources DROP COLUMN IF EXISTS historical_coverage;
ALTER TABLE data_sources DROP COLUMN IF EXISTS allowed_request_frequency;
ALTER TABLE data_sources DROP COLUMN IF EXISTS commercial_use_allowed;
ALTER TABLE data_sources DROP COLUMN IF EXISTS attribution_requirements;
ALTER TABLE data_sources DROP COLUMN IF EXISTS permission_reviewed_at;
ALTER TABLE data_sources DROP COLUMN IF EXISTS license_evidence;
ALTER TABLE data_sources DROP COLUMN IF EXISTS permission_status;
