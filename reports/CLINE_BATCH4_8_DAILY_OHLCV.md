# Batch 4.8 — Daily OHLCV

## Adapter
OhlcvAdapter (fixture-backed JSON). Validates OHLC relationships, nonnegative volume.

## Pipeline
run_ohlcv_pipeline: upsert by (security_id, trading_date). Idempotent.

## Go API
GET /api/v1/charts/ohlcv (existing).
