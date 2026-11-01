# Batch 4.5 — Corporate Actions

## Adapter
DividendCapitalActionAdapter: cash dividends, bonus shares, rights, splits.

## Pipeline
run_dividend_capital_pipeline: upsert by (symbol, action_type, fiscal_year, book_closure_date). Idempotent.

## Go API
GET /api/v1/corporate-actions, /companies/{symbol}/corporate-actions.
