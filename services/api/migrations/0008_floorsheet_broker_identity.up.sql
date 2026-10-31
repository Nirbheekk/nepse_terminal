-- 0008_floorsheet_broker_identity: keep the member id beside the broker name.
--
-- A capture taken while the session was open carries null member ids and null
-- names; the same contracts carry both once the exchange has published them.
-- Storing only the name made the identifier unreachable from SQL and left
-- summaries keyed on a display string, which can be renamed between sessions.
--
-- broker_code in the summary table is therefore the member id when the exchange
-- supplied one, with the name kept beside it for display. Rows with no id fall
-- back to the name, and the coverage figures say how many did.

ALTER TABLE floorsheet_trades ADD COLUMN buyer_broker_id text;
ALTER TABLE floorsheet_trades ADD COLUMN seller_broker_id text;

CREATE INDEX floorsheet_buyer_idx ON floorsheet_trades (trading_date, buyer_broker_id);
CREATE INDEX floorsheet_seller_idx ON floorsheet_trades (trading_date, seller_broker_id);

ALTER TABLE floorsheet_broker_summaries ADD COLUMN broker_name text;
