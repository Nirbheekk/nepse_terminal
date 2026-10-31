# MASTER REPOSITORY INVENTORY

## Top-level
- apps/web: Next.js frontend
- services/api: Go API + migrations
- contracts/openapi.yaml
- docs: architecture, data catalog, licensing, runbooks
- infrastructure: docker, deployment, monitoring
- legacy: historical assets
- scripts: operational scripts

## Frontend
- App Router under src/app
- Route groups: (auth), (terminal)
- Admin route
- Components, hooks, lib
- Tests: chart_interactions.test.mjs, e2e_workflows.test.mjs

## Backend
- cmd/* entrypoints
- internal/* domain packages
- migrations: 14 up/down pairs
- OpenAPI contract
- Go tests present

Status: Verified by filesystem inspection and successful builds/tests.
