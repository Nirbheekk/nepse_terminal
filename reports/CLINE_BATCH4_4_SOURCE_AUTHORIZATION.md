# Batch 4.4 — Source Authorization

## Statuses
All official sources remain PERMISSION_UNKNOWN. Live fetch disabled by default (NEPSE_LIVE_FETCH_ENABLED=false).

## Migration 0016
Extended data_sources with: permission_status, license_evidence, permission_reviewed_at, attribution_requirements, commercial_use_allowed, allowed_request_frequency, historical_coverage, last_successful_retrieval.

## Rules
- No CAPTCHA/login/rate-limit bypass.
- No automatic approval of public websites.
- Fixture-backed parsers only for unapproved sources.
- Reference repos (yonepse, NepseAPI-Unofficial) remain REFERENCE_ONLY.
