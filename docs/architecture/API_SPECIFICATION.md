# API SPECIFICATION (v1)

**Phase 0 deliverable.** The contract implemented in Phases 2–8. Written before
code so the Go handlers, the OpenAPI document, and the generated TypeScript types
cannot drift apart.

Base path: `/api/v1`. All responses `application/json; charset=utf-8` unless noted.

---

## 1. Envelope

Every response — success or failure — is wrapped identically.

```jsonc
// success
{
  "data": { },
  "meta": {
    "request_id": "01J9…",
    "as_of": "2026-10-29T13:42:11+05:45",
    "collected_at": "2026-10-29T13:42:14.201Z",
    "source": { "slug": "nepse_unofficial", "name": "NEPSE (unofficial client)" },
    "license_class": "public_unverified",
    "freshness": "live",
    "coverage": { "covered": 403, "eligible": 410, "unit": "companies" },
    "stale": false,
    "calculation": "market_cap@v1"
  },
  "error": null
}

// failure
{
  "data": null,
  "meta": { "request_id": "01J9…" },
  "error": {
    "code": "SOURCE_UNAVAILABLE",
    "message": "NEPSE upstream did not respond within 5s.",
    "retryable": true,
    "details": { "attempts": 3, "last_status": 503 },
    "doc_url": "/developers#SOURCE_UNAVAILABLE"
  }
}
```

`meta.stale: true` is the only signal the frontend needs to show a freshness
badge; it never has to infer from timestamps.

### Error codes

| Code | HTTP | Retryable | Meaning |
|---|---|---|---|
| `BAD_REQUEST` | 400 | no | malformed/missing parameter |
| `VALIDATION_FAILED` | 422 | no | schema violation, with `details.issues[]` |
| `UNAUTHENTICATED` | 401 | no | missing/expired access token |
| `FORBIDDEN` | 403 | no | authenticated, insufficient permission |
| `ENTITLEMENT_REQUIRED` | 403 | no | data exists but this subject may not receive it |
| `NOT_FOUND` | 404 | no | unknown symbol/resource |
| `CONFLICT` | 409 | no | duplicate (e.g. watchlist name) |
| `RATE_LIMITED` | 429 | yes | `Retry-After` header set |
| `SOURCE_UNAVAILABLE` | 503 | yes | exchange upstream failing |
| `SOURCE_DATA_ERROR` | 502 | yes | upstream payload unparseable |
| `DATA_UNAVAILABLE` | 404 | no | source does not publish this dataset (e.g. historical intraday) |
| `INTERNAL` | 500 | yes | unexpected; `request_id` traced |

`DATA_UNAVAILABLE` is deliberate: the brief requires *labelled* unavailability
rather than fabricated data. The message names the reason and, where known, the
licensed path to obtain it.

---

## 2. Common parameters

| Parameter | Type | Notes |
|---|---|---|
| `symbol` | string | company symbol, case-insensitive |
| `security_id` | uuid | preferred where ambiguous |
| `from`, `to` | `YYYY-MM-DD` | inclusive, max span 5 years |
| `interval` | enum | `1m\|5m\|15m\|30m\|1h\|1d\|1w\|1M\|1Q\|1y` |
| `limit` | int | default 50, max 500 |
| `cursor` | opaque string | keyset pagination; `meta.next_cursor` |
| `sort` | `field[,field]` | whitelisted per endpoint; `-` prefix = desc |
| `format` | enum | `json\|csv\|parquet` on export endpoints |

**Pagination is keyset, not offset.** `meta.next_cursor` is `null` at the end.
Offset paging on a 20k-row floorsheet table drifts while live data arrives.

---

## 3. Market

### `GET /market/status`
Market open/closed, next session, exchange timezone, calendar source.
```jsonc
{ "is_open": true, "trading_date": "2026-10-29",
  "session": { "open": "11:00", "close": "15:00", "tz": "Asia/Kathmandu" },
  "last_collection": "2026-10-29T13:42:14Z", "freshness": "live" }
```
Open/closed is computed from the exchange calendar independently of network
health — a closed market is never reported as an outage.

### `GET /market/summary`
All 19 overview cards in one call (`AUDIT_REPORT.md` §4 feature list):
index level/change/prev-close/open/high/low, turnover, transactions, shares,
traded securities, listed companies, total and float market cap **with
`coverage`**, breadth (advanced/declined/unchanged), status, trading date,
last update.

### `GET /market/breadth`
`{ "advanced": 178, "declined": 96, "unchanged": 41, "total": 315,
   "ratio": 1.85, "by_sector": [ … ] }`

### `GET /market/top-movers?type=gainers|losers|turnover|volume|transactions&limit=20`

### `GET /market/health`
Collection health: per-job timestamps, consecutive failures, last error class.
Public-safe subset of `/admin/data-quality`.

---

## 4. Companies

| Endpoint | Returns |
|---|---|
| `GET /companies?q=&sector=&status=&sort=&limit=&cursor=` | list: symbol, name, sector, close, change, volume, turnover, market cap |
| `GET /companies/{symbol}` | full profile + latest quote + capitalisation + 52w range + source meta |
| `GET /companies/{symbol}/prices?from=&to=&interval=` | OHLCV series (daily via `daily_ohlcv`) |
| `GET /companies/{symbol}/financials` | periods + metrics, each with `formula` and `period_end` |
| `GET /companies/{symbol}/corporate-actions` | dividends, bonus, rights, AGMs |
| `GET /companies/{symbol}/dividends` | subset |
| `GET /companies/{symbol}/news` | articles referencing the symbol |
| `GET /companies/{symbol}/peers` | same-sector comparison rows |
| `GET /companies/{symbol}/floorsheet?date=` | that day's trades |
| `GET /companies/{symbol}/depth` | latest bid/ask ladder, `as_of` |

Unavailable fields are **absent**, not zero, and the profile includes
`"unavailable": [{ "field": "free_float_shares", "reason": "not published" }]`.

---

## 5. Charts

| Endpoint | Notes |
|---|---|
| `GET /charts/ohlcv?symbol=&interval=&from=&to=` | server-side resampling from `daily_ohlcv`; `1m–1h` return `DATA_UNAVAILABLE` unless genuine intraday exists for the range |
| `GET /charts/indicators?symbol=&interval=&set=sma:20,ema:50,rsi:14` | computed in Go, cached; response includes `warmup` and `params` per indicator |
| `GET /charts/layouts` | user's saved layouts |
| `POST /charts/layouts` | create/update; body validated by Zod on the client and struct tags server-side |
| `DELETE /charts/layouts/{id}` | |

Indicator response shape:
```jsonc
{ "indicator": "rsi", "params": { "period": 14 },
  "warmup_bars": 14,
  "series": [ { "t": "2026-10-28", "v": 58.31 } ] }
```
`warmup_bars` is mandatory — it is how the UI knows to render blanks instead of
fake values at the left edge.

---

## 6. Indices & sectors

| Endpoint | Notes |
|---|---|
| `GET /indices` | all definitions + latest value/change |
| `GET /indices/{id}/history?from=&to=&granularity=daily` | daily closes from `index_observations` |
| `GET /indices/{id}/intraday?date=` | session points; labelled `granularity: intraday` |
| `GET /sectors` | 13 sectors with value/change/turnover |
| `GET /sectors/comparison?period=1d\|5d\|30d\|90d\|1y\|custom` | %/points/value per sector |
| `GET /sectors/{code}/constituents` | member securities |

Period comparison uses stored daily closes. Current-session intraday points are
**never** substituted for daily closes.

---

## 7. Screener

### `POST /screener/run`
```jsonc
{
  "logic": "and",
  "filters": [
    { "field": "market_cap", "op": "gte", "value": 10000000000 },
    { "field": "rsi_14",     "op": "lt",  "value": 30 },
    { "field": "sector",     "op": "in",  "value": ["BANKING"] }
  ],
  "sort": "-turnover", "limit": 50, "cursor": null
}
```
Server-side execution against indexed columns plus a materialised
`security_metrics_latest` view. Supported field list is returned by
`GET /screener/fields` with each field's data availability, so the UI cannot
offer a filter the system cannot honour.

| Endpoint | Notes |
|---|---|
| `GET /screener/fields` | field catalogue + availability |
| `GET /screener/presets` / `POST` / `DELETE /{id}` | saved screens, owned by user |

---

## 8. Floorsheet

| Endpoint | Notes |
|---|---|
| `GET /floorsheet?date=&symbol=&buyer=&seller=&min_amount=&contract_id=&sort=&limit=&cursor=` | keyset paginated |
| `GET /floorsheet/coverage` | available dates, row counts, per-date DB/archive consistency, first/last date |
| `GET /floorsheet/brokers?date=` | broker-level aggregates, explicitly labelled *reported identifiers, not beneficial ownership* |
| `GET /floorsheet/analytics?date=&symbol=` | concentration, size distribution, large trades, avg price |
| `GET /floorsheet/export?date=&format=csv\|parquet\|json` | entitlement-gated, audit-logged, signed URL |

`executed_at` may be `null`; `ingested_at` is always present. The two are never
interchanged.

---

## 9. Watchlists, alerts, portfolios

| Endpoint | Purpose |
|---|---|
| `GET/POST /watchlists`, `PATCH/DELETE /watchlists/{id}` | CRUD, owner-scoped |
| `POST /watchlists/{id}/items`, `DELETE /watchlists/{id}/items/{security_id}` | membership, ordering |
| `GET /watchlists/{id}/quotes` | enriched rows |
| `GET/POST /alerts`, `PATCH/DELETE /alerts/{id}` | rules |
| `GET /alerts/events?limit=` | delivery history |
| `GET/POST /portfolios`, `PATCH/DELETE /portfolios/{id}` | CRUD |
| `GET/POST /portfolios/{id}/transactions` | ledger; `POST /import` for CSV |
| `GET /portfolios/{id}/holdings` | qty, avg cost, cost basis, realized/unrealized P&L |
| `GET /portfolios/{id}/performance?from=&to=` | TWR, XIRR, series |
| `GET /portfolios/{id}/risk` | volatility, beta, Sharpe, Sortino, max drawdown, concentration, sector exposure |

Every portfolio query is scoped by the session's `user_id` (and `organization_id`
where applicable) — never by a client-supplied id.

---

## 10. News, research, admin

| Endpoint | Notes |
|---|---|
| `GET /news?symbol=&category=&from=&to=&q=` | headline + short summary + link; full text never mirrored |
| `GET /announcements` | NEPSE notices, AGM, dividends |
| `POST /research/query` | AI, evidence-grounded; response separates `facts[]`, `computed[]`, `interpretation`, `citations[]`, `coverage_notes[]` |
| `POST /research/reports` | generates a brief from a template; same provenance rules |
| `GET /admin/ingestion` | job list with states, durations, row counts |
| `POST /admin/ingestion/jobs` | trigger a job (`kind`, `params`), idempotency key required |
| `GET /admin/data-quality` | open issues, grouped by code/severity |
| `GET /admin/audit-logs?actor=&action=&from=&to=` | |
| `GET /admin/licenses` | source registry incl. redistribution rights |

AI responses **must** fail closed: if retrieval returns nothing above threshold,
the answer is `{"insufficient_evidence": true, "coverage_notes": [...]}` rather
than prose generated from model memory.

---

## 11. Auth

| Endpoint | Notes |
|---|---|
| `POST /auth/register` | Argon2id, email verification token |
| `POST /auth/login` | sets `access` (15 min) + `refresh` (30 d, rotating) as HttpOnly cookies |
| `POST /auth/refresh` | rotates; reuse of an old token revokes the entire `family_id` |
| `POST /auth/logout` | revokes the session; clears cookies |
| `POST /auth/password/reset-request`, `/reset-confirm` | single-use tokens |
| `GET /auth/sessions`, `DELETE /auth/sessions/{id}` | device management |
| `GET /me` | profile, roles, entitlements, preferences |

Cookies: `HttpOnly; Secure; SameSite=Lax; Path=/`. Access token is **never**
placed in `localStorage`. State-changing cookie-authenticated requests require a
double-submit CSRF token; `Origin`/`Referer` must match the configured origin.

---

## 12. Cross-cutting

| Concern | Implementation |
|---|---|
| Rate limits | per-IP 120 rpm anonymous, per-user 600 rpm; `429` + `Retry-After`; export endpoints 10/hour |
| Trace IDs | `X-Request-Id` echoed; propagated into logs and `ingestion_jobs.trace_id` |
| Caching | `Cache-Control: private, max-age=0` on personalized; `public, max-age=5, stale-while-revalidate=25` on market reads |
| Compression | gzip/brotli for JSON > 1 KiB |
| CORS | same-origin only in production; explicit allow-list in dev |
| Versioning | additive-only within `v1`; breaking changes go to `/v2` |
| OpenAPI | generated from handler annotations into `api/openapi.yaml`, committed and CI-checked |
| TS types | `openapi-typescript` → `web/src/lib/api/schema.d.ts`; runtime responses parsed with Zod in `web/src/lib/api/client.ts` |

### Documented in `api/openapi.yaml`

Every operation carries `x-data-source`, `x-license-class`, `x-entitlement`, and
`x-freshness-sla`. CI fails the build if a route is added without them — this is
how §16 (provenance) and §22 (licensing) stay enforced instead of aspirational.

---

## 13. Legacy parity map

The 27 live legacy routes (inventoried from `/openapi.json`, `AUDIT_REPORT.md`
§2.3) map onto v1 as follows. Phase 4 is complete only when every row marked
*parity* passes a contract test against both implementations.

| Legacy | v1 | Gate |
|---|---|---|
| `GET /api/health` | `GET /healthz`, `/readyz` | parity |
| `GET /api/status` | `GET /market/status` + `/market/health` | parity |
| `GET /api/diagnostics` | `GET /admin/data-quality` (public subset: `/market/health`) | parity |
| `GET /api/overview` | `GET /market/summary` | parity |
| `GET /api/summary` | `GET /market/summary` (merged) | parity |
| `GET /api/market-cap` | inside `/market/summary` + `coverage` | parity + method field |
| `GET /api/companies` | `GET /companies` | parity |
| `GET /api/company/{symbol}` | `GET /companies/{symbol}` | parity |
| `GET /api/history/{symbol}` | `GET /companies/{symbol}/prices` | parity (now DB-backed) |
| `GET /api/market-depth/{symbol}` | `GET /companies/{symbol}/depth` | parity |
| `GET /api/indices` | `GET /indices` | parity |
| `GET /api/indices/{key}/graph` | `GET /indices/{id}/intraday` | parity, labelled `intraday` |
| `GET /api/index-graph/{name}` | `GET /indices/{id}/intraday` (alias retired) | replaced |
| `GET /api/sectors` | `GET /sectors` | parity |
| `GET /api/sectors/comparison` | `GET /sectors/comparison` | parity |
| `GET /api/floorsheet` | `GET /floorsheet` | parity, keyset pagination |
| `GET /api/floorsheet/dates` | `GET /floorsheet/coverage` | extended |
| `GET /api/floorsheet/export.csv` | `GET /floorsheet/export` | **entitlement-gated** (legacy was not) |
| `GET /api/export/{filename}.csv` | `GET /floorsheet/export` / `/data-explorer` exports | **entitlement-gated** |
| `GET /api/data/{filename}` | `GET /data-explorer/datasets/{name}` | **entitlement-gated**, path-traversal already fixed |
| `GET /api/files` | `GET /admin/ingestion/archives` | admin-only |
| `GET /assets/{filename}`, `GET /` | Next.js static/RSC | replaced |
| `POST /api/refresh` | `POST /admin/ingestion/jobs` | now authorized + audit-logged |
| `POST /api/collect/floorsheet` | `POST /admin/ingestion/jobs {kind:'floorsheet'}` | now authorized + audit-logged |
| `GET /api/dashboard` | — | replaced by `/` RSC |
