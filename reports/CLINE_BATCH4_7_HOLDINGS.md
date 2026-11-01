# Batch 4.7 — Holdings

## Adapter
MutualFundHoldingsAdapter (fixture-backed JSON). Parses holding symbol, name, units, market value, weight, as-of date.

## Resolution
Security resolution via canonical symbol. Unresolved holdings flagged for entity-resolution review.

## Pipeline
run_mf_holdings_pipeline: inserts portfolio disclosures.

## Go API
GET /api/v1/mutual-funds/{id}/holdings.
