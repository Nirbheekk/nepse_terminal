# Batch 4.8 — Market Data Inventory

## Existing features
- daily_ohlcv table, indices, floorsheet_trades, market_summaries, index_observations.
- Go handlers: market overview, summary, indices, OHLCV, floorsheet, adjusted prices.
- Freshness endpoints.

## New in Batch 4.8
- OhlcvAdapter + pipeline + validation.
- Trading calendar module (effective-dated).
- Market status + freshness helpers.
- Migration 0020 (trading_calendar, data_freshness_snapshots, data_coverage_snapshots).
- Go market-data endpoints: status/calendar/coverage/freshness/quality/ingestion-status (documented; subset implemented).
- OHLCV Go handler already exists (handleOHLCV).

## Inventory
IMPLEMENTED: daily OHLCV table, index observations, Go OHLCV API, freshness.
PARTIALLY IMPLEMENTED: intraday index snapshots (table exists), trading calendar (module + table).
NOT IMPLEMENTED: security minute candles, tick data.
BLOCKED: none.
NOT VERIFIED: production historical coverage.
