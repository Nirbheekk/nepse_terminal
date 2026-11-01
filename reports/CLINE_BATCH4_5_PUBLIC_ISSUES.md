# Batch 4.5 — Public Issues

## Adapter
PublicIssueAdapter (fixture-backed JSON). Parses IPO/FPO/rights notices.

## Pipeline
run_public_issue_pipeline: upsert by (symbol, issue_type, open_date). Idempotent.

## Go API
GET /api/v1/public-issues (optional ?symbol).
