# Batch 4.4 — Company Profiles

## Adapter
CompanyProfileAdapter (fixture-backed JSON). Parses company profiles, normalizes symbol/sector/website/capital/dates, deterministic entity resolution.

## Pipeline
run_company_profile_pipeline: transactional upsert by symbol, idempotent (source_document_id excluded from change detection).

## Go API
GET /api/v1/companies/{symbol}/profile returns profile + source/verification_status/missing_data.

## Tests
tests/unit/test_fundamentals.py, tests/integration/test_fundamentals_pipeline.py.
