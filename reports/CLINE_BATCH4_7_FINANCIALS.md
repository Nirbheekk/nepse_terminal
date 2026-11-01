# Batch 4.7 — Financials

## Reuse
Batch 4.6 financial parsing architecture. Fund-specific facts: investment/dividend/interest income, realized/unrealized gains, management/custodian fees, total assets/liabilities, net assets, units outstanding.

## Go API
GET /api/v1/mutual-funds/{id}/financials (returns empty/missing_data until real data).
