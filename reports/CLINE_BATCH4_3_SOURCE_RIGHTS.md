# Batch 4.3 — Source Rights

## Classification
Current market-data classification remains `public_unverified`; redistribution prohibited. Not changed.

## Statuses
- PERMISSION_UNKNOWN: all official sources listed above. Only fixture-backed parsers and discovery metadata implemented. Live fetch disabled by default (NEPSE_LIVE_FETCH_ENABLED=false).
- REFERENCE_ONLY: both GitHub repositories. Treated as untrusted reference; no code copied.

## Rules enforced
- No authentication/CAPTCHA/rate-limit bypass.
- No uncontrolled scraping loops.
- No restricted raw documents exposed via public APIs.
- Scanned PDFs marked OCR_REQUIRED.
- Missing values never converted to zero.
