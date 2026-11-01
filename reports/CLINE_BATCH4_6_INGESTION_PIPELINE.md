# Batch 4.6 — Ingestion Pipeline

## Flow
FinancialReportAdapter -> Document Registration -> PDF Inspection -> Statement Classification -> Table Extraction -> Fact Normalization -> Financial Validation -> Entity Resolution -> Transactional Persistence -> Ratio Calculation -> Provenance Recording.

## Idempotency
Verified (SQLite + PostgreSQL).

## Retry/checksum/parser versioning/review queues/structured logging/failure recovery supported.
