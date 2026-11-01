# Batch 4.3 — Test Results

## Python
- Location: services/data-ingestion
- Command: `PYTHONPATH=src python -m pytest -q`
- Result: 12 passed (numbers, html tables, cdsc parser, pipeline idempotency).

## PostgreSQL migrations
- `go test ./migrations/` passes (file pairing, non-empty, sequential 0001-0015).

## Go
- `go build ./...` passes.
- `go test ./...` passes (all packages cached/ok).

## Next.js
- `npm run typecheck` passes.
- `npm test` passes (18 tests).

## Security
- No credentials committed.
- Live fetch disabled by default.
- No production DB modified.
