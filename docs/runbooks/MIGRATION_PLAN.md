# MIGRATION PLAN

**Phase 0 deliverable.** A dependency-ordered, gate-controlled strangler migration
from the audited Python/FastAPI + SQLite + single-page-HTML system to
**Go (chi, pgx, PostgreSQL)** + **Next.js App Router**.

Baseline it must not regress: [AUDIT_REPORT.md](AUDIT_REPORT.md) §9 — **131 passing
tests**, verified live data, and a checksummed export
(`exports/legacy/20261029T084007Z`, 11/11 validation checks).

**Blocking prerequisites found during the audit** (both must be closed before
Phase 1 begins, and neither requires writing a feature):

| Prereq | Why | Action |
|---|---|---|
| **P1 — no version control** | `git status` → `fatal: not a git repository`. There is no way to revert anything. | `git init` + baseline commit + tag (AUDIT §12 R0) |
| **P2 — no off-machine backup** | `nepse.db` (31 MB) and `data/` exist only here; floorsheet history cannot be re-fetched (403) | Cold copy before the first schema change (AUDIT §12 R1) |

---

## 1. Principles

1. **The Python service keeps running** until a Go replacement passes its
   acceptance gate. Nothing is deleted first.
2. **One source of truth:** PostgreSQL. JSON files become an ingestion staging
   area, not a parallel store.
3. **Behaviour is ported, structure is replaced.** Data semantics that were hard
   won in this codebase are preserved verbatim: Asia/Kathmandu handling, one-time
   NPT→UTC conversion, `contractId` natural key, computed market cap with
   coverage disclosure, honest "unavailable" states.
4. **No feature is claimed until its gate passes** (brief §27).
5. **Licensing gates publication, not engineering** — see
   [LICENSING_REPORT.md](LICENSING_REPORT.md). Entitlement checks are built from
   Phase 1, not retrofitted.

## 2. Target architecture

```
                    ┌──────────────────────────┐
  Browser ──HTTPS──►│ Next.js (App Router, TS) │──► /api/v1 (server-to-server)
                    │  RSC + client charts     │
                    └──────────────────────────┘
                                 │
                    ┌────────────▼─────────────┐
                    │ Go API (chi)             │
                    │  auth │ market │ charts  │
                    │  screener │ floorsheet   │
                    │  portfolio │ admin       │
                    └────────────┬─────────────┘
                                 │ pgxpool
                    ┌────────────▼─────────────┐
                    │ PostgreSQL 16            │
                    │  NUMERIC money, TIMESTAMPTZ│
                    │  versioned migrations    │
                    └────────────▲─────────────┘
                                 │
                    ┌────────────┴─────────────┐
                    │ Go workers (separately   │
                    │ runnable, same binary    │
                    │ with -role=worker)       │
                    │  ingest │ indicators     │
                    │  alerts │ valuations     │
                    │  reconcile │ exports     │
                    └────────────▲─────────────┘
                                 │
                    ┌────────────┴─────────────┐
                    │ Source adapters          │
                    │  unofficial NEPSE client │
                    │  → licensed feed (later) │
                    └──────────────────────────┘
```

Deliberately **not** included: Kubernetes, Kafka, ClickHouse, microservices.
Redis only if a measured need appears (session revocation or worker coordination).

### Ports (avoiding the unrelated stack already running)

| Service | Port | Note |
|---|---|---|
| Go API | 8080 | |
| Next.js | 3000 | |
| PostgreSQL | **5433** | a `postgres:16-alpine` container from another project already uses 5432; it is untouched |
| Legacy FastAPI | 8000 | reduced to a read-only reference by Phase 4, then stopped |

## 3. Schema mapping (SQLite → PostgreSQL)

| Legacy | PostgreSQL | Change |
|---|---|---|
| `floorsheet.contract_id TEXT` | `floorsheet_trades.contract_id NUMERIC(30,0)` + `UNIQUE` | numeric compare, not string |
| `quantity/rate/amount FLOAT` | `NUMERIC(18,4)` / `NUMERIC(18,6)` | **no float money** |
| `trade_time DATETIME` (UTC naive) | `executed_at TIMESTAMPTZ` | zone-aware; the NPT→UTC conversion happens once, in the adapter |
| `business_date DATE` | `trading_date DATE` | unchanged |
| `raw TEXT` | `raw JSONB` + `raw_sha256` | queryable, verifiable |
| — | `ingestion_job_id`, `data_source_id`, `license_class` | provenance (brief §16) |
| `index_snapshots` | `index_observations` | + `index_definition_id` FK |
| `daily_prices` (0 rows) | `daily_ohlcv` | will actually be populated from D5 |
| `market_cap_snapshots` | `market_cap_history` | + `method_version`, `coverage_numerator/denominator` |

New tables (auth, watchlists, alerts, portfolios, research, admin) are created
from `docs/DATABASE_SCHEMA.md` in Phase 1 and are genuinely new work, not ports.

## 4. The one genuine data win — corrected during Phase 3

This section previously claimed that `getPriceVolumeHistory(date)` would allow a
≈230-session backfill (~78,000 daily bars). **Phase 3 disproved that claim and the
platform does not rely on it.** Re-testing the endpoint during implementation
resulted in a 403 for every historical date, and the legacy `daily_prices` table
remains empty, so no historical bars were ever collected. What the corpus actually
contains is a single session, 2026-10-29.

The available data win is therefore smaller but real: the platform reconstructs
daily bars from the 425 captured company snapshots (which the legacy system read
and discarded), plus 48,273 contract-level trades and 17 index observations.
Multi-session history remains **blocked** on an authorised source, and is stated
as blocked in [IMPLEMENTATION_PROGRESS.md](IMPLEMENTATION_PROGRESS.md) rather than
counted as pending work.

## 5. Phases and gates

Each phase ends with tests and a written gate. A phase is not "done" because code
exists; it is done when the gate passes.

| Phase | Deliverable | Depends on | Gate |
|---|---|---|---|
| **0** | This audit set + export/validation tool | — | 11/11 export checks PASS; 131 legacy tests green; full file inventory and defect register ✅ |
| **1** | Repo skeleton: `cmd/api`, `cmd/worker`, `internal/*`, compose (Postgres 5433), versioned migrations for the full schema, config, structured logging, `/healthz` `/readyz`, provenance columns, entitlements table | 0 | `docker compose up` → migrations apply → `/readyz` 200; `go test ./...` green; migration up/down/up idempotent |
| **2** | Auth + RBAC: users, sessions, refresh rotation, Argon2id, HttpOnly cookies, CSRF, roles, orgs, audit log; Next.js shell with login/settings/admin | 1 | Two-account isolation test; refresh rotation; CSRF rejection; token never in localStorage |
| **3** | Ingestion workers + adapters: market, daily OHLCV (**with the ~1-year backfill**), indices, floorsheet (1 s live + post-close), reference data; reconciliation and quality checks; legacy export loaded | 2 | Row-count and checksum parity with the latest `exports/legacy/<run>/manifest.json`; no duplicate `contract_id`; every row has provenance |
| **4** | Go market API parity: `/market/*`, `/companies*`, `/indices*`, `/sectors*`, `/floorsheet*`, `/charts/ohlcv` | 3 | Contract tests mirroring the 44 legacy API tests; legacy FastAPI switched to read-only reference |
| **5** | Next.js terminal core: Market Overview, Markets, Companies, Stock Analysis, Indices & Sectors, Market Depth, Charts (daily→yearly) + indicator engine | 4 | Playwright: every page renders real data; no console errors; indicator unit tests against reference series |
| **6** | Screener, Floorsheet workspace, Watchlists, Historical Explorer, exports (entitlement-gated) | 5 | Screener filters return correct subsets; floorsheet pagination/filters parity; export requires entitlement |
| **7** | Alerts engine, Portfolio + risk analytics (TWR, XIRR, Sharpe, drawdown), valuations | 6 | Recompute a known portfolio by hand; alert dedupe/cooldown tests |
| **8** | News, Research/AI (gated on licensing), Institutional workspace (portfolio half only), Developers portal (gated) | 7 | Citation-provenance test for AI; unsupported modules render an explicit unavailable state |
| **9** | Observability, CI/CD, backups + restore rehearsal, runbook, load test | 8 | Restore from backup verified; p95 latency budget met; dashboards live |
| **10** | Legacy retirement after acceptance | 9 | Every legacy feature either parity-tested or explicitly dropped with a written reason |

### Corrected facts the plan depends on (from the audit)

| Earlier assumption | Verified reality | Effect on plan |
|---|---|---|
| Scheduler runs 13 jobs | **5 jobs + 1 daemon thread** (`server.py:351-419`) | Phase 3 port is smaller than scoped |
| `getDaily*Graph` returns daily history | Returns **one session** (445 points / 3.8 h) | Phase 5 daily charts depend entirely on the Phase 3 backfill + forward accumulation |
| 650 companies have detail files | **425 do** (65.4%); only 6 have extras | Phase 3 reference backfill is real work, not a copy |
| API has 22 routes | **27 operations** | Phase 4 contract surface is 5 operations larger |

### Effort calibration (honest)

Phase 1–4 is the heavy structural work; 5–6 is the bulk of UI surface; 7–8 is
where new analytics live. Realistically **8–12 focused working sessions** for a
credible terminal through Phase 7, excluding waiting on NEPSE licensing and
excluding the deliberately-blocked features in
[FEATURE_CATALOG.md](FEATURE_CATALOG.md). Any claim of a complete 25-module
institutional terminal in one pass would be false.

## 6. Rollback strategy per phase

The legacy system has **no rollback mechanism today** (AUDIT §12). The migration
must not repeat that mistake, so every phase declares how it is undone *before* it
is started.

### 6.1 Common checkpoints

```bash
# code (after P1 above)
git switch -c phase-N-<topic>
# ... work, commit per logical change ...
git switch main && git revert --no-commit <sha>   # to undo a merged phase

# data (before any schema or collector change)
STAMP=$(date -u +%Y%m%dT%H%M%SZ); mkdir -p backups/$STAMP
cp nepse.db backups/$STAMP/ && sha256sum backups/$STAMP/nepse.db > backups/$STAMP/nepse.db.sha256
python tools/export_legacy.py                     # checksummed baseline
```

Stop the server by PID before replacing `nepse.db`:

```bash
PID=$(netstat -ano | grep ":8000" | grep LISTENING | awk '{print $5}' | head -1); taskkill //PID $PID //F
```

### 6.2 Per-phase

| Phase | Rollback | Trigger to roll back |
|---|---|---|
| 1 | `docker compose down -v` (new volumes only); delete `cmd/`, `internal/`, `migrations/`. Legacy untouched. | migrations fail to re-apply; `/readyz` not 200 |
| 2 | Revert branch; drop only the new auth tables (`0002.down.sql`). No legacy data touched. | any session/CSRF test fails; token appears in client storage |
| 3 | `TRUNCATE` new Postgres tables and re-run from the export; **legacy SQLite and `floorsheet/` never modified** | turnover reconciliation mismatch; row-count/checksum parity fails |
| 4 | Proxy rule back to FastAPI:8000; keep Go container stopped | any contract test diverges from legacy |
| 5 | Serve `dashboard.html` at `/` again (single route change) | console error, wrong numbers, or a page renders unavailable data as real |
| 6 | Feature flag per page; exports fall back to disabled | screener returns a subset that fails an independent recount |
| 7 | Disable worker roles `alerts`, `valuation`; keep stored data | duplicate alerts, or P&L disagrees with hand calculation |
| 8 | Disable AI/news/developer routes; they are additive | AI emits an uncited figure |
| 9 | Config-only (metrics/tracing can be switched off) | monitoring overhead degrades p95 |
| 10 | Keep the legacy directory archived and the tag `legacy-final`; restore by checkout | any feature found missing after retirement |

### 6.3 Hard rules

1. **`nepse.db`, `data/`, `historical/`, `floorsheet/` are append-only during the
   whole migration.** No phase may delete or rewrite them. Capture-only.
2. **Destructive SQL never ships in the same release as code.** Expand → migrate →
   contract, one deploy apart.
3. **The legacy service is stopped, never deleted,** until Phase 10 passes its gate.
4. A phase that cannot state its rollback does not start.

## 7. Risk register

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| R1 | **NEPSE licensing not obtained** → cannot publish API/exports/AI | High | Blocking for release | Entitlement layer from Phase 1; keep deployments private; start NEPSE application now |
| R2 | Exchange blocks or rate-limits automated polling | Medium | Data loss | Adapter interface + configurable cadence + backoff (already proven); licensed feed swap |
| R3 | Historical depth is inventable only for ~1 year | Certain | Chart/screener limits | Disclose window on every metric; accumulate forward |
| R4 | No historical intraday data | Certain | Advanced charting limited | Session-only intraday, labelled; never synthesize bars |
| R5 | Scope explosion across 25 modules | High | Never ships | Phase gates; ship modules 1–5, 9, 25 first |
| R6 | Rewrite drifts from legacy behaviour | Medium | Silent regressions | Contract tests ported from 131 legacy tests; parallel run in Phase 4 |
| R7 | Float→NUMERIC migration rounding | Low | Wrong totals | Assert exported totals equal Postgres aggregates in Phase 3 gate |
| R8 | Port collision with the unrelated `backend-*` Docker stack | Low | Local breakage | Dedicated compose project, Postgres on 5433, no shared volumes |
| R9 | Go/Next toolchain drift | Low | Build breakage | Pin Go 1.27 and Node 24 (both installed); commit `go.sum`, lockfile |
| R10 | Auth done late leaves portfolio data exposed | Medium | Security | Auth is Phase 2, before any user data exists |
| R11 | **No VCS during early migration** → an unrecoverable edit | High until P1 closed | High | `git init` is prerequisite P1, not a phase task |
| R12 | Concurrent writers corrupt SQLite / `data/` while the Go stack runs in parallel | Medium | High | Legacy keeps its single-writer rule; Go writes only to Postgres; no shared files |
| R13 | Legacy JSON staleness mistaken for fresh data during parallel run | Medium | Medium | Go reads only Postgres; comparison jobs assert freshness before diffing |

## 8. Explicit non-goals for the first pass

Listed so they are not quietly "delivered" as stubs:

- Order placement / broker integration (regulatory review required)
- Real-time streaming (source is snapshot-polled)
- Broker-level investor attribution
- Institutional ownership, fund flows, factor exposure
- Historical intraday OHLCV
- Full financial statements (only reported summary metrics exist)

## 9. Phase 1 deliverable (built)

Paths below are the current layout, after the restructure into `services/api`
(the earlier single-module `platform/` tree is preserved outside this repository
as the legacy reference — see `legacy/README.md`).

```
services/api/cmd/server     chi router, /healthz, /readyz, graceful shutdown
services/api/cmd/worker     worker supervisor (-role flags), graceful stop
services/api/cmd/migrate    applies the embedded migration set and exits
services/api/internal/config       env-driven, validated at boot
services/api/internal/handlers     middleware: request-id, logging, recover, CORS, rate limit
services/api/internal/repositories pgxpool, checksum-guarded migration runner
services/api/migrations/0001_init  core market + identity schema (up/down)
services/api/migrations/0002_*     roles and permissions reference data
docker-compose.yml          postgres:16 on 5433, api, worker, web
scripts/                    dev bring-up and contract sync
```

No Next.js or business endpoints in Phase 1 — the gate is a migrating,
health-checked, provenance-aware skeleton, which is what everything else stands on.

### Gate result (verified 2026-10-29, against the real repository)

| Gate | Evidence |
|---|---|
| Migrations apply and re-apply idempotently | `go run ./cmd/api -migrate-only` twice → `applied: 2, latest: 0002_reference_data` both times; checksums guard edited files |
| `/readyz` returns 200 | `{"database":"reachable","migrations":{"applied":2,…},"status":"ready"}` |
| `go test ./...` green | `auth`, `config`, `httpapi` pass; `httpapi` suite includes 12 integration tests against PostgreSQL |
| Compose stack | `docker compose build api web` succeed; API image run standalone → `/readyz` 200 and real market data served as non-root |
| Provenance wired | every market payload carries `source`, `as_of`, `license_class`, `freshness`, `coverage` |

The Phase 1 scope grew by one deliberate exception: the web tier and auth were
built in the same pass, because a connectivity demonstration needs a browser and
an identity, and building them against a throwaway stub would have been wasted
work. Phases 2–4 therefore start from auth-complete rather than auth-planned.

### Phase 3 gate result (verified 2026-10-29, against the real repository)

Phase 3 was scoped as "ingestion workers + 1-year backfill + reconciliation". Two
of the three were delivered; the backfill is blocked and reported as such.

| Gate | Evidence |
|---|---|
| Every legacy record reaches PostgreSQL | `floorsheet` 48,273 legacy → 48,273 platform (all attributed to a security); `index_snapshots` 17 → 17 daily rows plus 34 session quotes; `market_cap_snapshots` 1 → 1; `company_list` 650 → 650; `security_list` 569 → 569 |
| Field values survive the move | 5,000 contracts compared field by field on both sides: 0 missing, 0 differing values. Aggregates match exactly (`sum(amount)` = 5,892,370,553.08 on both sides) |
| Reconciliation is real, not asserted | `go run ./cmd/reconcile` → 79 checks matched, 0 mismatched, 1 declared expected difference; report written to [DISCREPANCY_REPORT.md](DISCREPANCY_REPORT.md) |
| Idempotency | A forced re-run writes 0 rows across all 11 streams; an unchanged archive is skipped entirely. Repeated runs converge with no drift |
| Time basis is correct | The legacy `trade_time` column is UTC despite having no zone marker; the exchange's JSON carries Nepal time with no offset. Each adapter declares its basis, and the field comparison compares instants, which is how the earlier 5h45m defect was found and fixed |
| Provenance and freshness are queryable | `ingest_runs`, `raw_archives` (sha256 per archive), `data_source_id` per row, `dataset_slas` + `/api/v1/health/freshness` |
| Ingestion is reproducible from stored bytes | `var/archive/**` holds the exact bytes each run parsed; the loader can rebuild the database without touching the legacy sources |
| 1-year backfill | **Blocked.** Re-testing `getPriceVolumeHistory` returned 403 for historical dates and `daily_prices` is empty. The corpus is one session; no historical bars exist to migrate |

Two data defects were found and corrected during this phase, both of which would
have silently distorted analytics:

1. The legacy `daily_prices` table is empty, so the earlier seeder had derived bars
   by subtracting 5h45m from a value that was already UTC. Fixed; bars are now
   built from the snapshots at their true observation time.
2. Two index adapters report the same index on the same day at different times.
   Keying observations by `(index, date)` made 16 rows flip on every run; the key
   is now `(index, date, observed_at)`, so they are two points in a series rather
   than competing values.

The platform also had to model something the legacy schema could not express: the
exchange's security list contains promoter, preference, debenture and mutual-fund
classes whose issuer is missing from its company registry. 216 contracts reference
those symbols. They are now attached to their issuer when the class suffix resolves
to a registered company (131 of 135), and to a placeholder issuer marked
`origin = 'listed_security'` for the remaining 4, so no contract is unattributed and
no placeholder is counted as a listed company.
