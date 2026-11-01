# Batch 4.4 — Entity Resolution

## Implementation
normalizers/entity_resolution.py: deterministic resolution using symbol, aliases, historical symbols. Fuzzy name similarity never auto-merges. Ambiguous matches flagged needs_review.

## Tests
test_fundamentals.py includes entity resolution cases.
