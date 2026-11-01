# Batch 4.6 — Ratio Engine

## Ratios
EPS, P/E, book value per share, P/B, ROE, ROA, net profit margin, debt-to-equity, current ratio, dividend payout ratio.

## Precision
Decimal-based; PostgreSQL NUMERIC. Division by zero / missing denominators handled (returns None).

## P/E
Returns None for zero/negative EPS (not meaningful). Uses compatible EPS.

## Every ratio includes formula inputs via input_lineage jsonb.
