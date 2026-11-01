# Batch 4.3 — Repository Audit

## Existing architecture preserved
- Go backend (Chi, pgx, PostgreSQL) untouched in structure; additive handlers only.
- Next.js frontend untouched.
- Existing migrations 0001-0014 preserved; new 0015 added.
- Existing provenance/audit tables reused (data_sources, raw_archives, ingest_runs, collection_jobs, disclosures).

## Discovery findings
- 14 existing migrations; migration pairing test updated to expect >=15.
- Existing financial/corporate-action tables (financial_periods, financial_statement_lines, financial_ratios, corporate_actions) reused conceptually; new canonical tables added for report-level facts.
- Go handlers: financials, ratios, corporate-actions already existed; new endpoints added for profile/board/ownership/mutual-funds/coverage/freshness.

## New components
- services/data-ingestion/ Python package.
- services/api/migrations/0015_fundamental_data_foundation.{up,down}.sql.
- services/api/internal/repositories/fundamentals.go
- services/api/internal/handlers/fundamental_handlers.go

## No destructive changes
- No existing tables dropped or replaced.
- No production database modified.
