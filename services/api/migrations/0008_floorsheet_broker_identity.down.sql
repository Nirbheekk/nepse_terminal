ALTER TABLE floorsheet_broker_summaries DROP COLUMN IF EXISTS broker_name;

DROP INDEX IF EXISTS floorsheet_buyer_idx;
DROP INDEX IF EXISTS floorsheet_seller_idx;

ALTER TABLE floorsheet_trades DROP COLUMN IF EXISTS seller_broker_id;
ALTER TABLE floorsheet_trades DROP COLUMN IF EXISTS buyer_broker_id;
