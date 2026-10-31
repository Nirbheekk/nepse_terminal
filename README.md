# NEPSE Terminal

Market terminal for the Nepal Stock Exchange: a Go API over PostgreSQL, a Next.js
App Router front end, and the ingestion, screening and research jobs that feed
them. It is built from the legacy Python collector in the parent directory, not
as a replacement for it — see [`legacy/README.md`](legacy/README.md).

## Layout

```
apps/web                       Next.js 15 App Router, TypeScript, Tailwind, Zod
  src/app/(auth)               login, register — no market chrome
  src/app/(terminal)           market, stocks, charts, indices, sectors, screener,
                               floorsheet, watchlists, alerts, portfolio, news,
                               research
  src/app/admin                data operations: freshness, ingest runs, archives
  src/components/{charts,terminal,tables,ui}
  src/lib/{api,schemas,utils}
  src/hooks
services/api                   Go 1.26 module (nepse/terminal/api)
  cmd/{server,worker,migrate}
  cmd/{ingest,reconcile,floorsheet,seed}   operational data tools
  internal/handlers            routing, middleware, HTTP handlers
  internal/repositories        pgx pool, migrations, all SQL
  internal/services            analytics, corporate actions, financials, ratios,
                               disclosures, institutional RBAC, developer API
  internal/marketdata          decimal arithmetic, ISINs, normalization, source
                               adapters (JSON snapshots, SQLite, exports)
  internal/ingestion           stream specs, validation, idempotent writes,
                               raw archives
  internal/reconciliation      old-vs-new coverage and discrepancy reports
  internal/screener            rule validation and the metric registry
  internal/alerts              alert evaluation, cooldowns, delivery
  internal/portfolios          positions, lots, NEPSE fee schedule
  internal/indicators          technical series
  internal/research            evidence-grounded query assembly
  internal/jobs                freshness monitoring and seeding
  migrations                   embedded, checksum-guarded
contracts/openapi.yaml         canonical API contract (served at /openapi.yaml)
infrastructure/{docker,monitoring,deployment}
scripts                        dev bring-up, contract sync
docs                           architecture, data catalog, licensing, runbooks
```

## Running it

```bash
./scripts/dev.sh db      # PostgreSQL on :5433
./scripts/dev.sh api     # API on :8081
./scripts/dev.sh web     # Next.js on :3100
```

Or the whole stack: `docker compose up --build`.

`./scripts/dev.sh` and `docker-compose.yml` use the same ports and credentials, so
only one of them should own the database at a time.

### Configuration

The API reads everything from the environment and refuses to start with a missing
or weak `JWT_SECRET`:

| Variable | Purpose |
|---|---|
| `DATABASE_URL` | PostgreSQL DSN, required |
| `JWT_SECRET` | signing key for access tokens, required, minimum length enforced |
| `HTTP_ADDR` | listen address, default `:8081` |
| `ALLOWED_ORIGINS` | comma-separated browser origins allowed to send credentialed requests |
| `COOKIE_SECURE` | set `true` behind TLS |

Tokens are HttpOnly cookies (`np_at`, `np_rt`) plus a readable `np_csrf`, and every
mutating request must echo the CSRF value in `X-CSRF-Token`.

## Checks

```bash
cd services/api && go build ./... && go vet ./... && go test ./...
cd services/api && TEST_DATABASE_URL=postgres://nepse:nepse_local_dev@localhost:5433/nepse?sslmode=disable \
  go test -tags=integration ./...
cd apps/web && npx tsc --noEmit && npm run build && npm test
./scripts/sync-contracts.sh --check
```

## Data honesty

Prices, volumes and capitalisation come from stored sessions. Every response
carries `meta.as_of`, `meta.source` and `meta.license_class`, and an empty result
is reported as a gap in coverage rather than filled with an estimate. Where a
metric needs more history than the database holds — a 20-period average with two
sessions on record — the API says so and the UI repeats it.

Nothing in this repository is licensed for redistribution of exchange data.
