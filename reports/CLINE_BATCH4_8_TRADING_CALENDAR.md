# Batch 4.8 — Trading Calendar

## Module
normalizers/calendar.py: is_weekend (Saturday), market_status (PRE_OPEN/MARKET_OPEN/MARKET_CLOSED/HOLIDAY), freshness_status.

## Table
trading_calendar (migration 0020): effective-dated, session_type, open/close times, timezone Asia/Kathmandu, reason, source.

## Rules
No hardcoded permanent schedule; special closures representable; UNKNOWN for unverified holidays.
