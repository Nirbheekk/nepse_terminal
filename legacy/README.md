# NEPSE Terminal - Legacy System & Retirement Path

## 1. Overview

This directory documents the historical reference implementation, scripts, and initial SQLite schema of the legacy Python collector system.

> **Where the code actually lives.** The monorepo was built beside the original
> checkout rather than on top of it, so the legacy tree was never moved or
> rewritten. It sits one level up, as `../platform/` (the first Go service) plus
> the original Python entry points at `../server.py`, `../collector.py`,
> `../db.py`, `../market_data.py`, `../nepse_utils.py`, and `../nepse.db`. Keeping
> them in place means the old `python -m pytest` suite and the old dashboard still
> run unchanged, and every count in the reconciliation reports can be re-checked
> against the same files. Nothing under this directory is imported by the Go or
> Next.js services.
- **`collector.py`**: Legacy Selenium/Requests scraper for NEPSE endpoints.
- **`db.py`**: Legacy SQLite storage driver.
- **`nepse.db`**: Local SQLite historical database (reconciled and backfilled into PostgreSQL).
- **`market_data.py` & `nepse_utils.py`**: Legacy helper utilities and mathematical functions.

## 2. Retirement & Transition Status

All legacy functionality has been completely rewritten and verified in Go and Next.js:
1. **Reconciliation Complete**: 79/79 verification checks matched; 48,273 trade contracts migrated and reconciled with zero field divergence.
2. **Precision Upgrade**: Migrated from standard floating-point to exact arbitrary-precision rational arithmetic (`numeric.Decimal` using `big.Rat` and PostgreSQL `NUMERIC`).
3. **Decoupling**: Portfolio accounting is completely decoupled from trade execution.
4. **Data Provider Licensing**: Raw exchange feeds are classified as `public_unverified` with external redistribution prohibited, while official corporate disclosures are licensed under `permitted_with_attribution`.

The legacy Python components are preserved strictly for audit provenance and historical validation against archive snapshots.
