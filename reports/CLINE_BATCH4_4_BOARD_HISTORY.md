# Batch 4.4 — Board History

## Adapter
BoardDisclosureAdapter (fixture-backed JSON). Parses director name, role, designation, appointment/cessation dates, independent flag.

## Pipeline
run_board_pipeline: upsert by (symbol, full_name, role); historical appointments preserved (no overwrite of distinct rows).

## Go API
GET /api/v1/companies/{symbol}/board returns members + source/verification_status/missing_data.

## Tests
Included in fundamentals tests.
