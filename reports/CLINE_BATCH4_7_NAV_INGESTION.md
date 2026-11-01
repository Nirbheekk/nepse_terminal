# Batch 4.7 — NAV Ingestion

## Adapter
MutualFundNavAdapter (fixture-backed JSON). Parses NAV per unit, total NAV, units outstanding, valuation/publication dates.

## Validation
NAV nonnegative, units positive, total NAV consistency with NAV*units (1% tolerance), duplicate detection.

## Pipeline
run_mf_nav_pipeline: upsert by (fund, valuation_date). Idempotent. Revision tracking.

## Go API
GET /api/v1/mutual-funds/{id}/nav, /nav/history.
