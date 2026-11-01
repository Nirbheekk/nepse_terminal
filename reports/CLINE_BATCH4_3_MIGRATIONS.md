# Batch 4.3 — Migrations

## Added
- services/api/migrations/0015_fundamental_data_foundation.up.sql
- services/api/migrations/0015_fundamental_data_foundation.down.sql

## Updated
- migration_test.go expectedMinMigrations 14 -> 15.

## Verification
- `go test ./migrations/` passes.
- Migration runner applies 0015 in a single transaction.
- Down migration uses DROP TABLE IF EXISTS.
