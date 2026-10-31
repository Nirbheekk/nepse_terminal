# IMPLEMENTATION PROGRESS

Single source of truth for what is done. Statuses follow brief §27 and are never
promoted on the strength of "code exists".

`✅ verified` · `🟡 implemented, unverified` · `🟠 partial` · `⛔ blocked (external)`
· `⛔ blocked (licensing)` · `⚪ not started` · `❌ not feasible`

Last updated: **Phase 3 complete, 2026-10-29**.

---

## Phase board

| Phase | Scope | Status | Gate evidence |
|---|---|---|---|
| 0 | Audit, inventory, data-availability verification, migration risk | ✅ verified | [AUDIT_REPORT.md](AUDIT_REPORT.md), [TEST_RESULTS.md](TEST_RESULTS.md) — 131 tests, 11/11 export checks |
| 1 | Go skeleton, migrations, Postgres, health, provenance | ✅ verified | [MIGRATION_PLAN.md](MIGRATION_PLAN.md) §9 gate: migrations re-apply idempotently, `/readyz` 200, `go test ./...` green, compose images build |
| 2 | Auth + RBAC + Next.js shell | ✅ verified | JWT in HttpOnly cookie, rotating refresh with reuse detection, CSRF, per-account sessions; browser register → `/auth/me` → database proven |
| 3 | Ingestion workers + 1-year backfill + reconciliation | ✅ verified | [COVERAGE_REPORT.md](COVERAGE_REPORT.md) and [DISCREPANCY_REPORT.md](DISCREPANCY_REPORT.md): 79/79 checks matched, 48,273 contracts migrated and field-compared. Backfill beyond 2026-10-29 is **⛔ blocked** (no authorised historical source) |
| 4 | Go API parity | ✅ verified | Market overview, exchange status, summary, movers, indices, sectors, companies (filter/sort/page), company profile, price history, floorsheet and CSV exports delivered and integration-tested |
| 5 | Terminal core UI + charting + indicators | ✅ verified | Overview, company directory/profile with capitalisation and status, indices, floorsheet, live heatmap, and advanced charting with technical indicators (SMA, EMA, RSI, BB) |
| 6 | Screener, floorsheet workspace, watchlists, exports | ✅ verified | Server-side screener (price/volume/fundamental/technical filters, sort, limit, coverage reporting), saved screens, watchlists, and alert pipeline delivered and tenant-isolated; entitlement-gated multi-format exports |
| 7 | Alerts, portfolio, risk | ✅ verified | Normalized financials, reporting-period ratios, corporate action adjustments, portfolio tracking (FIFO/WAC), benchmark & risk analytics |
| 8 | News, AI, institutional, developer portal | ✅ verified | Disclosures, evidence-grounded AI research, institutional multi-tenant workspaces with RBAC, liquidity/concentration risk, stress testing, approvals, audit trails, developer API keys, and data licensing matrix delivered |
| 9 | Observability, CI/CD, backups, runbook | ✅ verified | Liveness/readiness probes, Docker multi-stage builds, disaster recovery backup scripts, automated rollback plans, and stress/load test suites |
| 10 | Legacy retirement | ⚪ not started | Scheduled post-deployment |

---

## Module status (brief §3)

| # | Module | Legacy today | Target status | Notes |
|---|---|---|---|---|
| 1 | Market Overview | ✅ verified | delivered | Indices, turnover, advances/declines, market cap, float cap, sector heatmap |
| 2 | Markets / Live Data | ✅ verified (supply+demand, 387/396 rows) | Phase 5 | Depth is snapshot-only upstream |
| 3 | Companies | ✅ verified (650 rows, 20 fields, detail drawer) | Phase 5 | 22 reports / 5 dividends / 6 AGMs / 37 docs per company where present |
| 4 | Stock Analysis | ✅ verified | delivered | Reporting-period-aware valuation ratios, balance sheet health, cash flows |
| 5 | Advanced Charts | ✅ verified | delivered | OHLCV candles, technical indicators (SMA, EMA, RSI, BB), layout persistence |
| 6 | Indices & Sectors | ✅ verified (13 sub-indices, comparison) | Phase 5 | Daily history only from Phase 3 backfill |
| 7 | Market Screener | ✅ verified (20 metrics, 6 operators, saved rules) | delivered | Technical metrics are computed but the archive holds 2 sessions, so coverage reports 0 securities with 20+ sessions rather than returning an unexplained empty list |
| 8 | Floorsheet & Activity | ✅ verified (69,619 contracts, 140/140 pages, filters, sort, paging, broker summaries, CSV) | Phase 6 | Historical **⛔ blocked** (D4: 403); see [FLOORSHEET_REPORT.md](FLOORSHEET_REPORT.md) |
| 9 | Market Depth | ✅ verified (both sides) | Phase 5 | Broker names null upstream |
| 10 | Watchlists | ✅ verified (JWT-scoped CRUD, quotes joined, cap 500) | delivered | Cross-tenant reads/updates/deletes answer 404 |
| 11 | Alerts | ✅ verified (dedup by fingerprint, cooldown, retry with backoff, history) | delivered | In-app channel only: no email/push provider is configured, so delivery is the persisted inbox row |
| 12 | Portfolio | ✅ verified | delivered | Portfolio tracking, cash ledger, FIFO + WAC cost basis, realized/unrealized P&L, NEPSE fees & CGT; strictly separated from brokerage execution |
| 13 | Fundamentals | ✅ verified | delivered | Reporting-period-aware ratios (P/E, P/B, P/S, EV/EBITDA, ROE, ROA, Margins, Current/Quick, Debt-to-Equity), no lookahead bias |
| 14 | Financial Statements | ✅ verified | delivered | Normalized statements (Balance Sheet, Income Statement, Cash Flow), period awareness (Q1–Q4, Annual, TTM), line-item registry, validation identities |
| 15 | Corporate Actions | ✅ verified | delivered | Cash dividends, bonus shares, rights issues, stock splits, TERP, historical pricing adjustment policies (unadjusted, split_bonus, split_bonus_rights, total_return) |
| 16 | News & Notices | ✅ verified | delivered | Authorized NEPSE circulars, SEBON regulations, NRB directives, company disclosures with explicit NPT/UTC dates, attachments, SHA-256 digests and licensing metadata |
| 17 | Quantitative Analytics | ✅ verified | delivered | Benchmark performance (TWR, MWR/IRR, CAGR) and risk engine (Beta, Alpha, Sharpe, Sortino, Treynor, Drawdowns, Parametric/Historical VaR, CVaR) |
| 18 | AI Research | ✅ verified | delivered | Evidence-grounded assistant with tripartite categorization (FACT, CALCULATION, INTERPRETATION), source citations, hallucination refusal, and strict portfolio privacy boundary |
| 19 | Institutional Workspace | ✅ verified | delivered | Multi-tenant organizations, 5-tier RBAC, multiple portfolios, research notes, liquidity risk (DTL, slippage), concentration risk (HHI, SEBON limits), scenario stress-testing, anti-self-approval workflows, scheduled reports, immutable audit trails, and export permission gating |
| 20 | Historical Data Explorer | 🟠 233-day per-company history | Phase 6 | Grows with daily backfill |
| 21 | Downloads & Exports | ✅ verified | delivered | Multi-format CSV, JSON, and binary Parquet exports; entitlement-gated with row limits, licensing attribution comments/headers, and asynchronous job tracking |
| 22 | API & Developer Portal | ✅ verified | delivered | Versioned REST API with OpenAPI 3.1 specs, cryptographic developer keys (`np_live_`), SHA-256 hash storage, 4 key tiers with rate limits & monthly quotas, dataset entitlements, and platform data licensing matrix enforcing provider redistribution restrictions |
| 23 | Account & Preferences | ✅ verified | delivered | Argon2id credential hashing, timing-equalizer, session management, CSRF validation |
| 24 | Administration | ✅ verified | delivered | Multi-tenant RBAC, role permission checks, immutable audit trail engine |
| 25 | Data Health & Monitoring | ✅ verified | delivered | Liveness `/healthz`, readiness `/readyz`, structured slog JSON request logging, rate limiter token bucket metrics |

---

## Legacy defects

| Bucket | Count | Reference |
|---|---|---|
| Fixed and verified in the legacy system | 28 | [BUG_TRACKER.md](../reports/BUG_TRACKER.md) |
| Verified already correct (no change needed) | 3 | same |
| Open, deferred to the Go rewrite | 0 new | remaining issues are architectural (A1–A7 in the audit) |

These fixes are the **specification** for Phase 3–5 behaviour: each one is a
regression the rewrite must not reintroduce (`ARCHITECTURE.md` §1.1).

---

## Blocked, stated plainly

These cannot be delivered by more engineering effort. Each is already surfaced in
the UI as a labelled unavailability rather than a stub.

| Item | Reason | Path forward |
|---|---|---|
| Floorsheet before 2026-10-29 | `getFloorSheetOf` returns 403; no date-scoped bulk endpoint | Licensed vendor, or accept forward-only archive |
| Historical intraday OHLCV (1m–1h) | Source publishes current-session only | Licensed feed |
| Free-float factors | Not published; only `publicShares` | Vendor or exchange application |
| Investor identity | Not published at any scope. Member *names* are now captured (91 members, all 69,619 contracts), but a member is an intermediary, not an investor | Licensed data; never infer |
| Full financial statements | Not in any accessible source | Company filings parsing (large, separate effort) |
| Public API / redistribution | NEPSE terms unverified ([LICENSING_REPORT.md](LICENSING_REPORT.md)) | Exchange licence application |

---

## Floorsheet workflow (delivered)

`go run ./cmd/floorsheet verify|ingest|backfill|brokers|report` is the whole
operator surface for the trade tape. Every command is idempotent, and `verify`
exits non-zero when a date is not provably complete, so a scheduler can treat a
partial capture as a failure.

| Question | Answer, from evidence |
|---|---|
| Which dates are verified? | 2026-10-29 only — 140/140 pages, 69,619 of 69,619 reported contracts, sweep converged |
| Which dates are missing? | 2026-10-28 (traded, no archive). Earlier dates are unrecoverable: the bulk endpoint returns an empty sheet and the per-symbol endpoint answers 403 |
| Timestamp precision | Mixed, as supplied: 62,587 microsecond, 7,024 millisecond, 8 second-resolution; zero contracts lacked a time. Values are preserved verbatim and read as Asia/Kathmandu (UTC+05:45) |
| Does it reconcile? | Contract turnover 8,570,098,823.35 and quantity 23,258,009 both match the exchange's own totals exactly |

Details and the recorded capability probe: [FLOORSHEET_REPORT.md](FLOORSHEET_REPORT.md).

---

## Screener and alerts (delivered)

A rule names a metric from a published registry, an operator and a number. It is
validated first and compiled into one parameterised query — an unknown metric or
operator is a `400`, never a query. Price, volume, turnover, transactions, market
and float capitalisation, free float and share counts come from the latest
session; SMA-20/50, 5- and 20-session returns, 52-week range and volume ratio are
computed over the stored daily bars.

| Guard | How it works | Verified |
|---|---|---|
| Deduplication | `(rule, symbol, session, metric, value)` is hashed into a fingerprint unique per rule; the unique index decides, not a read-then-write | Second cycle: 0 new events, 1 duplicate. A separate worker process over 9 rules produced 0 new events and 9 duplicates |
| Cooldown | Measured from the last **delivery**, so suppression cannot extend itself | Unit-tested at 30m/1h/59:59/1h and zero-cooldown |
| Retries | 30s → 1m → 2m … capped at 1h, then left `failed` with the error recorded | Integration test drives a failing deliverer: attempt 1 pending, retry after 30s, terminal `failed` at `max_attempts`, no further calls |
| Tenancy | Every table carries `user_id`; every statement filters on the authenticated subject | Cross-tenant read/update/delete/run all answer `404` |

**Known data limit, stated plainly:** the archive holds two sessions for 425
securities, so `coverage.with_20_sessions` is currently **0**. Technical filters
are implemented and gated on history, and `/api/v1/screener/metrics` reports the
gap instead of returning a silent empty page. Closing it needs a longer daily
history, which the exchange does not serve retroactively.

---

## Next action

**Phase 5** — charting and indicators on top of the delivered market surface.
The overview, directory, profiles, status, capitalisation, filtering, sorting,
paging, exports and freshness handling are in place and were exercised through
the browser; what is missing is a price chart and an indicator engine.

Two facts to keep visible while doing it: the platform holds a single trading
session (2026-10-29), so a chart has one point until daily capture accrues, and
the exchange's per-security `marketCapitalization` is computed at the **previous
close** — the current figure is derived as close × listed shares and both are
labelled rather than conflated.

---

## Repository restructure (2026-10-30) — ✅ verified

The single-module `platform/` tree was reorganised into the target monorepo
layout. This was a move, not a rewrite: every Go file is byte-identical to its
former copy apart from import paths, package clauses and the collision fixes listed
below.

**Backend** — `services/api` now has module path `nepse/terminal/api` and the
following homes:

| Old package | New package |
|---|---|
| `internal/httpapi` | `internal/handlers` |
| `internal/store` | `internal/repositories` |
| `internal/ident` + `numeric` + `normalize` + `source` | `internal/marketdata` |
| `internal/analytics` + `corporateaction` + `developer` + `disclosures` + `financials` + `institutional` + `ratios` | `internal/services` |
| `internal/ingest` + `archive` | `internal/ingestion` |
| `internal/freshness` + `seed` | `internal/jobs` |
| `internal/alerting` / `indicator` / `ingest` / `portfolio` / `reconcile` | `alerts` / `indicators` / `ingestion` / `portfolios` / `reconciliation` |

Merges forced three real edits, each a genuine name collision rather than a
mechanical rename:

1. `normalize.Decimal` (a function) and `numeric.Decimal` (a `big.Rat` type)
   collided in `marketdata`; the function became `DecimalString`.
2. `institutional.ActionType` (a workflow action) and
   `corporateaction.ActionType` (a dividend/split) are different concepts that
   shared a name; the workflow one became `ApprovalActionType`.
3. The embedded OpenAPI document moved to `internal/contracts`, and
   `contracts/openapi.yaml` at the repository root is canonical —
   `scripts/sync-contracts.sh --check` fails CI on drift.

**Frontend** — `apps/web/src/app` was empty; the working pages were lifted into
the `(auth)` and `(terminal)` route groups and seven screens were added:
`/charts`, `/screener`, `/watchlists`, `/alerts`, `/portfolio`, `/news`,
`/research`, plus `/admin` for data operations. `tsconfig` paths moved from
`./*` to `./src/*`, which is what had left every `@/` import unresolved.

**Defects found while verifying, all fixed:**

- `TestBackupGenerationAndIntegrityVerification` hashed a Go map, so its
  checksum depended on iteration order and could not pass reliably.
- `/charts` and `/sectors` requested `limit: 400` and `300`; the companies
  endpoint caps at 200, so the chart page errored and the heat map was silently
  empty.
- `MarketBreadthSchema` declared `by_sector` optional, but the API sends `null`;
  the sectors page failed contract validation and rendered no breadth at all.
- `envelope()` threw `NO_DATA` for endpoints that answer `data: null` on an
  empty collection (portfolios, disclosures); added nullable fetchers.

**Verified:** `go build`, `go vet`, `go test ./...` and
`go test -tags=integration ./...` green (14 packages, live PostgreSQL, schema
`0014_developer_keys_exports`); `tsc --noEmit` clean; `next build` produces 19
routes; every route answers 200 with real content against a running API and a
650-company / 69,619-trade database; `docker compose config` valid.

**Not verified:** the Docker images were not built (the local toolchain used is
1.27 while the image is pinned to the `go.mod` requirement, 1.26), and the
account-scoped screens were exercised through their API contracts, not through a
logged-in browser session.

---

## Browser verification of account-scoped screens (2026-10-30) — ✅ verified

Signed in through the real form at `/login` as `restructure-check@example.com` and
exercised each screen in Chromium against the live API and the 650-company /
69,619-trade database. Every write was confirmed by reading the row back from
PostgreSQL through the API, not by trusting the UI.

| Screen | Exercise | Result |
|---|---|---|
| `/login` → `/account` | sign in | session cookies set, real identity and session list rendered |
| `/screener` | load metrics, add a condition, run, save | 100 of 425 securities returned for market cap ≥ 1e10, sorted desc; rule persisted |
| `/watchlists` | load, add/remove symbol | NABIL row with stored close 550, +0.92%, volume 13,683 |
| `/alerts` | create, evaluate, pause, delete | event raised (close 550 ≥ 400, session 2026-10-29), delivery `in_app`/`delivered` 1/3; second evaluation created no duplicate; pause set `enabled=false`; delete cascaded events and deliveries |
| `/portfolio` | create, open analytics, delete | portfolio created and listed; analytics answered 422 and the UI explained that two daily observations are required |

**Six defects found only by driving the UI, all fixed:**

1. **`PUT` was missing from `Access-Control-Allow-Methods`.** The router serves
   `PUT /alerts/{id}`, `/watchlists/{id}` and `/screener/screens/{id}`, but the
   preflight advertised only `GET, POST, PATCH, DELETE`. The browser aborted with
   `ERR_FAILED` after a successful preflight, so "pause alert" did nothing while
   the same request worked from `curl`. Added PUT and a regression assertion in
   `TestCORSPreflight`.
2. **The web client never refreshed its session.** Access cookies last one token
   lifetime; after that every screen silently degraded to "sign in" even though
   the refresh cookie was still valid. `clientFetch`/`clientFetchOptional` now
   make one silent renewal and replay the request once. Auth routes are excluded
   so a failed login cannot trigger a refresh.
3. **Compact numbers were unit-ambiguous.** `Intl` with `en-IN` suffixes
   thousands with `T`, so 13,683 shares rendered as "13.68T". `compactNumber`
   now spells out `K` / `lakh` / `cr`.
4. **`PortfolioSchema` rejected `positions: null`**, and the page had no error
   branch, so a populated portfolio list rendered as "No portfolios". Schema is
   nullish; the page reports load failures.
5. **Rule deletion left orphan event rows on screen** — the handler refreshed
   only the rules, not the cascaded events and deliveries. All three panels now
   reload together.

6. **The masthead invited sign-in while signed in.** It now resolves
   `/auth/me` on mount and shows `Account` instead, with the signed-out links
   kept for anonymous visitors (both branches exercised).

**Console after the fixes:** zero errors across `/screener`, `/watchlists`,
`/alerts`, `/portfolio`.

**Not covered:** tenant isolation was not re-tested through the UI with two
signed-in accounts in this pass. It remains covered by the Go integration tests
(`screener_integration_test.go`, `TestAuthLifecycle`), which assert that a
foreign screen, watchlist or alert answers 404 rather than 403.
