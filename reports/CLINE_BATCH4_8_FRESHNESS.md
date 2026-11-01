# Batch 4.8 — Freshness

## Implementation
freshness_status(last_observation, now, expected_minutes) -> CURRENT/DELAYED/STALE/UNAVAILABLE. Successful no-new-observation jobs do not refresh timestamp.

## Table
data_freshness_snapshots (migration 0020).

## Go API
GET /api/v1/health/freshness (existing) + /data/freshness.
