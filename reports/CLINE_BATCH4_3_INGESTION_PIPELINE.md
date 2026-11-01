# Batch 4.3 — Ingestion Pipeline

## Architecture
Source Adapter -> Fetch -> Raw Document Metadata -> Parse -> Normalize -> Validate -> Stage -> Review if necessary -> Transactional Upsert -> Provenance Record.

## Implemented vertical slice
CDSC IPO list (fixture-backed, permission unknown, live fetch disabled).

Demonstrated:
1. Authorized file import (import-document).
2. Source metadata capture (source_documents).
3. Raw content checksum (sha256).
4. Parsing (cdsc_ipo parser).
5. Validation (validate_public_issue).
6. Canonical entity mapping (public_issues).
7. Transactional upsert (single session transaction).
8. Idempotent rerun (duplicate sha256 -> skipped; no duplicates).
9. Provenance (source_document_id, source_url on rows; ingestion_runs).
10. Data quality reporting (rows_read/written/rejected in ingestion_runs).

## CLI commands
list-sources, check-source, import-document, run-source, validate-records, show-ingestion-status, retry-failed, report-coverage.

## Reliability
Bounded retries, exponential backoff, request timeouts, per-source rate limiting, conditional HTTP (ETag/Last-Modified), checksums, duplicate detection, structured-ish logs, job locking (jobs), idempotency, failure recovery (rollback).
