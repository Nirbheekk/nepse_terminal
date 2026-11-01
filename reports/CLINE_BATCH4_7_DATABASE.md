# Batch 4.7 — Database

## Migration
0019_mutual_fund_revisions: revision_number, supersedes_id on mutual_fund_nav_history.

## Tables reused
mutual_funds, mutual_fund_schemes, mutual_fund_nav_history, mutual_fund_portfolio_disclosures.

## Idempotency
Verified for profile, NAV, holdings pipelines.

## Provenance
source_document_id on all rows.
