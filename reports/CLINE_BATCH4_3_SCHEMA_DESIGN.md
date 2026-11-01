# Batch 4.3 — Schema Design

## New tables (migration 0015)
Company fundamentals: company_profiles, company_identifiers, company_directors, company_director_appointments, ownership_snapshots.
Corporate actions: public_issues, public_issue_updates, company_meetings, merger_events, merger_participants.
Financial: financial_reports, financial_statement_facts, financial_metric_definitions, financial_ratio_observations, financial_report_documents.
Mutual funds: mutual_funds, mutual_fund_schemes, mutual_fund_nav_history, mutual_fund_portfolio_disclosures.
Ingestion ops: source_documents, ingestion_jobs, ingestion_runs, ingestion_errors, data_quality_findings, entity_resolution_reviews.

## Reused tables
data_sources, raw_archives, collection_jobs, ingest_runs, ingest_checkpoints, disclosures, financial_periods, financial_statement_lines, corporate_actions, companies, securities.

## Rules
- NUMERIC for financial values; UTC timestamps; FKs; unique constraints; check constraints; indexes.
- Source identifiers stored separately from canonical IDs.
- Ownership snapshots allow unknown values; no forced 100%.
- Financial facts preserve original labels, scope, audit status, units, restatements.
- Ratios distinguish reported vs calculated.
- NAV records valuation/publication dates, scheme, currency, source document.
