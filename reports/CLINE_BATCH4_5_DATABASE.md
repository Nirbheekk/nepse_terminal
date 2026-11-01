# Batch 4.5 — Database

## Migration
0017_corporate_actions_provenance (adds source_document_id to corporate_actions).

## Tables reused
corporate_actions, public_issues, company_meetings, merger_events, merger_participants, source_documents, ingestion_runs, data_quality_findings.

## Idempotency
Verified for all pipelines (SQLite + PostgreSQL).

## Provenance
source_document_id on all event rows.
