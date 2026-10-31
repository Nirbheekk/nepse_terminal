# ARCHITECTURE

**Phase 0 deliverable.** Describes (a) the system as it actually exists today,
verified by inspection and by running it, and (b) the target architecture that
Phase 1 begins building.

---

## 1. Current architecture (verified, not aspirational)

```
┌──────────────────────────────────────────────────────────────────┐
│ dashboard.html  (single file, 1,650 lines, vanilla JS)           │
│  tabs: Overview · Market · Companies · Indices · Floorsheet ·     │
│        News · All files                                          │
│  Chart.js 4.4.1 — vendored at static/chart.umd.min.js            │
└───────────────┬──────────────────────────────────────────────────┘
                │ fetch(), same-origin, no auth
┌───────────────▼──────────────────────────────────────────────────┐
│ server.py — FastAPI + Uvicorn, 1,787 lines, 27 operations        │
│  lifespan → init_db() + BackgroundScheduler(5 jobs + 1 thread)   │
│  REST (mounted at / and /api) + /files + /export.csv             │
└───────┬───────────────────────┬──────────────────┬───────────────┘
        │                       │                  │
┌───────▼────────┐   ┌──────────▼─────────┐  ┌─────▼──────────────┐
│ market_data.py │   │ collector.py       │  │ db.py              │
│ 600 lines      │   │ 728 lines          │  │ SQLAlchemy → SQLite│
│ derive+serve   │   │ NepseClient calls  │  │ 6 tables           │
└───────┬────────┘   └──────────┬─────────┘  └────────────────────┘
        │                       │
┌───────▼────────────┐  ┌───────▼──────────────────────────────┐
│ data/*.json (11)   │  │ nepse_client (unofficial, PyPI)      │
│ historical/*.json  │  │ https://nepalstock.com.np endpoints   │
│ floorsheet/<date>  │  └──────────────────────────────────────┘
└────────────────────┘
```

### 1.1 Real defects that shaped the design

These are **fixed in the legacy system** but are recorded because the Go
implementation must not reintroduce them. Root causes are in
[BUG_TRACKER.md](../reports/BUG_TRACKER.md) and `AUDIT_REPORT.md` §5.

| Defect class | Root cause | Architectural rule for the rewrite |
|---|---|---|
| Whole dashboard showed "Connection error" while all APIs returned 200 | A wrong SRI hash on a CDN `<script>`; the resulting throw was caught by the global error banner | Vendor every asset; **never** let a render error masquerade as a transport error |
| Auto-refresh silently dead | `fastOnly` referenced outside its closure (TDZ) | Typed client; no implicit globals; the interval lives in one module |
| Company page stalled `/api/status` | Blocking `requests` calls inside `async def` | Workers are separate processes; the API never performs upstream I/O on a request path |
| Floorsheet archive truncated to 500 rows | Writer rewrote the file from the current batch | DB is authoritative; files are **derived** artifacts, rebuilt, never appended by a partial batch |
| `TypeError: list indices must be integers` | Library assumes `{floorsheets: [...]}`; exchange intermittently returns a bare list | Adapter validates the envelope; a malformed payload is a **retryable data error**, not an outage |
| Concurrent scheduler jobs corrupted the client | One `NepseClient` shared across threads; mutable request state | Adapters are per-call or properly pooled; one HTTP client per goroutine use, `context`-bound |
| Market snapshot starved by the 1 s job | Single shared lock across jobs | Job-scoped concurrency, not global locks; a slow job never blocks another kind |
| Timestamps looked wrong / "(browser fetch)" label | Naive datetimes and mixed NPT/UTC | One conversion at the boundary; `TIMESTAMPTZ` everywhere; UI renders NPT explicitly |

### 1.2 Data flow (current, per cycle)

```
source JSON (upstream)  →  data/<name>.json + data/_meta.json
                        →  market_data.py derives serving payloads
                        →  server.py routes read the JSON (no DB)
floorsheet (live, 1 s)  →  collector.store_floorsheet()
                        →  SQLite floorsheet (authoritative)
                        →  floorsheet/<date>.json rebuilt from DB, every 60 ticks
```

Note the asymmetry: **floorsheet is database-backed, everything else is
file-backed.** The migration intentionally makes the database authoritative for
all of it.

---

## 2. Target architecture

### 2.1 Component responsibilities

| Component | Owns | Never does |
|---|---|---|
| Next.js (App Router, TS strict) | Rendering, routing, client chart state, Zod validation of API responses | Upstream network calls to NEPSE, SQL, secrets |
| Go API (chi) | Authn/authz, entitlements, read models, screener execution, export signing | Long-running collection |
| Go workers | All source I/O, normalization, reconciliation, derived metrics, alert evaluation | Serving HTTP to browsers |
| PostgreSQL 16 | Durable truth, provenance, audit, job state | — |
| Redis *(optional)* | Only if measurement shows a need | Holding market data |

One binary, two roles: `cmd/api` and `cmd/worker` share `internal/*`. This keeps a
modular monolith while allowing the worker to scale or be paused independently.

### 2.2 Ingestion pipeline contract

Every adapter implements the same shape (planned interface, Phase 3):

```go
type Adapter interface {
    Name() string
    Source() SourceDescriptor        // auth, cadence, rate limit, license class
    Kind() []Dataset                 // Market, DailyOHLCV, Index, Floorsheet, Reference
    Fetch(ctx context.Context, req Request) (RawBatch, error)
    Validate(raw RawBatch) ([]Issue, error)   // envelope + schema + sanity
    Normalize(raw RawBatch) ([]Record, error) // NPT→UTC, decimal parsing
}

// RawBatch is archived before normalization and referenced by every row.
type RawBatch struct {
    JobID     uuid.UUID
    FetchedAt time.Time
    Endpoint  string
    Body      []byte
    SHA256    [32]byte
}
```

Guarantees the pipeline must provide, each independently tested:

1. **Raw archive first.** Persist `RawBatch` (object storage or `raw_payloads`)
   before any parse. A parse failure is reproducible from the archive.
2. **Idempotent persist.** `INSERT … ON CONFLICT (contract_id) DO NOTHING` for
   floorsheet; `(security_id, trading_date)` for OHLCV. Re-running any job in any
   order converges to the same rows.
3. **Reconciliation, not overwrite.** A conflicting value for an existing key
   creates a `data_quality_issues` row retaining both values; nothing is silently
   replaced.
4. **Provenance on every row.** `data_source_id`, `ingestion_job_id`,
   `raw_sha256`, `license_class`, `ingested_at`.
5. **Checkpointed, resumable backfill.** `ingestion_checkpoints(job_id, cursor)`
   so a 230-date backfill resumes rather than restarts.

### 2.3 Error taxonomy (carried over deliberately)

The legacy system learned to distinguish these; the rewrite keeps all four:

| Class | Meaning | Effect |
|---|---|---|
| `backend_unavailable` | API unreachable | banner + last-good data |
| `source_unavailable` | NEPSE upstream failing | backoff, freshness degrades, market status untouched |
| `source_data_error` | envelope/schema bad for a tick | one skip; escalate after N consecutive |
| `stale` / `partial` | collection behind watermark | freshness badge, never an outage |

A closed exchange is **never** reported as a connection failure — market calendar
(Asia/Kathmandu, Sun–Thu) is evaluated independently of network state.

### 2.4 Freshness and provenance model

Every API response carries a metadata envelope:

```json
{
  "data": "...",
  "meta": {
    "as_of": "2026-10-29T13:42:11+05:45",
    "collected_at": "2026-10-29T13:42:14.201Z",
    "source": "nepalstock.com.np (unofficial client)",
    "license_class": "public_unverified",
    "coverage": { "covered": 403, "eligible": 410 },
    "stale": false,
    "calculation": "market_cap@v1"
  }
}
```

This is what makes claimed-vs-verified auditable, and what an entitlement check
can act on without touching business logic.

### 2.5 Security boundaries

```
public  ──►  unauth read (license_class = public_unverified only)
user    ──►  own watchlists/alerts/portfolios (row-level, by user_id)
pro     ──►  + licensed delayed data (entitlement-gated)
analyst ──►  + institutional workspaces, exports
admin   ──►  + ingestion control, data quality, audit logs
```

Enforced in middleware + query scoping, never in the browser. Org-scoped rows are
filtered by `organization_id` derived from the session, never from a request
parameter.

## 3. Deployment topology

Local/dev: `docker compose` with `postgres:16` on **5433**, `api`, `worker`, and
`web` (Next dev on host). Production: same four units behind a TLS reverse proxy;
backups via `pg_dump` on a schedule with a rehearsed restore. No orchestrator
until a real need appears.

## 4. Decisions and rejected alternatives

| Decision | Chosen | Rejected | Why |
|---|---|---|---|
| Charting | `lightweight-charts` (Apache-2.0) | TradingView Charting Library (licensed, redistribution restricted; also requires data-feed contract) | Compatible license, purpose-built for OHLCV, small bundle |
| Money representation | `NUMERIC` in PG, `decimal` in Go, decimal strings in JSON | float64 | Auditable totals; no 0.1+0.2 surprises in P&L |
| Job queue | PostgreSQL-backed durable jobs | Kafka / Redis Streams | Volume is thousands of rows/day; PG already transactional |
| Auth | First-party JWT in HttpOnly cookies + Argon2id | Hosted auth vendor | No third-party PII processor for a financial tool; full audit control |
| Data access | pgx + hand-written SQL | ORM | Explicit queries, no hidden N+1, index-friendly |
| Frontend state | RSC + TanStack Query where interactive | Global store everywhere | Server-first rendering of market data |
| Legacy Python | Kept as read-only reference through Phase 4 | Delete on switch | Rollback path; contract parity testing |
