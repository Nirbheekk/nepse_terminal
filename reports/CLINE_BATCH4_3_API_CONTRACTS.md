# Batch 4.3 — API Contracts

## Endpoints implemented (Go)
- GET /api/v1/companies/{symbol}/profile
- GET /api/v1/companies/{symbol}/board
- GET /api/v1/companies/{symbol}/ownership
- GET /api/v1/mutual-funds
- GET /api/v1/mutual-funds/{id}/nav
- GET /api/v1/data/coverage
- GET /api/v1/data/freshness (alias to existing freshness handler)

## Already existing
- GET /api/v1/companies/{symbol}/financials
- GET /api/v1/companies/{symbol}/ratios
- GET /api/v1/corporate-actions
- GET /api/v1/companies/{symbol}/corporate-actions

## Planned (documented, not yet backed by data)
- GET /api/v1/companies/{symbol}/financials (exists)
- GET /api/v1/public-issues (use /corporate-actions? or future)
- GET /api/v1/mergers
- GET /api/v1/company-meetings
- GET /api/v1/data/freshness (done)

## OpenAPI
contracts/openapi.yaml and services/api/internal/contracts/openapi.yaml synchronized (identical hash).
