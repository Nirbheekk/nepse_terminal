# Batch 4.9 — Frontend Integration

## Approach
Verified existing Next.js App Router integration against Go APIs. No duplicate API layers introduced. Existing typed clients and Zod validation preserved. Empty states accurate.

## Startup
- Backend: Go API on :8081 (docker) / local build.
- Frontend: Next.js on :3100.
- Database: PostgreSQL on :5433 (nepse_terminal_test for destructive tests).
