# Batch 5.0 — P0 Fixes

## Navigation
- apps/web uses /stocks (canonical). /companies route exists in platform/web (parallel app). No fix needed in canonical apps/web.
- Heatmap: embedded in market page in apps/web; /heatmap route only in platform/web.

## Charts
- Fixed relative fetch calls in financial-chart.tsx to use API_BASE (2 calls).
- performance-analysis.tsx already uses centralized API? It uses fetch("/api/v1/market/breadth") relative. Let me fix too.
