-- Reverse 0003. Market data rows are untouched; only the ingestion bookkeeping
-- and the identifier columns are removed.

DROP TABLE IF EXISTS reconciliation_runs;
DROP TABLE IF EXISTS dataset_slas;
DROP TABLE IF EXISTS ingest_checkpoints;
DROP TABLE IF EXISTS ingest_runs;
DROP TABLE IF EXISTS collection_jobs;
DROP TABLE IF EXISTS raw_archives;

DROP INDEX IF EXISTS securities_isin_key;
ALTER TABLE securities DROP COLUMN IF EXISTS isin_source_id;
ALTER TABLE securities DROP COLUMN IF EXISTS isin_checksum_ok;
ALTER TABLE securities DROP COLUMN IF EXISTS isin;

ALTER TABLE data_sources DROP COLUMN IF EXISTS attribution;
ALTER TABLE data_sources DROP COLUMN IF EXISTS access_mode;
