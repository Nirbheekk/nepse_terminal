# Batch 4.7 — Fund Profiles

## Adapter
MutualFundProfileAdapter (fixture-backed JSON). Parses fund code, name, manager, type, launch/maturity, outstanding units, benchmark.

## Pipeline
run_mf_profile_pipeline: upsert by fund_code.

## Go API
GET /api/v1/mutual-funds, /mutual-funds/{id}.
