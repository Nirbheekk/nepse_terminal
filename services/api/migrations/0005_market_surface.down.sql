DROP INDEX IF EXISTS companies_name_lower_idx;
DROP INDEX IF EXISTS companies_sector_symbol_idx;
DROP TABLE IF EXISTS market_status_snapshots;
DELETE FROM dataset_slas WHERE stream = 'market_status';
ALTER TABLE daily_ohlcv DROP COLUMN IF EXISTS market_cap;
