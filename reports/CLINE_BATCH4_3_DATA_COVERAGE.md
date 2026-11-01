# Batch 4.3 — Data Coverage

## Real records imported
- 0 (no production DB; no authorized live ingestion).

## Fixture records
- 3 public_issues (NABIL IPO, NICA FPO, SCB RIGHTS) imported into SQLite test DB during pipeline test.

## Missing categories
- Company profiles/board/ownership: tables exist, no data.
- Financial statements/ratios: existing tables, no new ingestion.
- Mutual funds/NAV: tables exist, no data.
- Mergers/meetings: tables exist, no data.

## Licensing limitations
- All official sources PERMISSION_UNKNOWN; live fetch disabled.
- Redistribution prohibited (public_unverified).
- GitHub repos REFERENCE_ONLY.
