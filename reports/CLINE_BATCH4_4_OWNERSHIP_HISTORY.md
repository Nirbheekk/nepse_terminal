# Batch 4.4 — Ownership History

## Adapter
OwnershipDisclosureAdapter (fixture-backed JSON). Parses promoter/public/other percentages, effective date.

## Validation
Percentages 0-100; no forced promoter+public=100. Missing values stay None (never zero).

## Pipeline
run_ownership_pipeline: upsert by (symbol, effective_date).

## Go API
GET /api/v1/companies/{symbol}/ownership returns snapshots + source/verification_status/missing_data.
