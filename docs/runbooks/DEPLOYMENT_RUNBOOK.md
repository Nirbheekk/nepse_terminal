# DEPLOYMENT RUNBOOK

**Phase 0 deliverable.** Two parts:

- **Part A** — operating the system that exists **today** (every command verified
  in this session).
- **Part B** — the target stack, written as the procedure Phase 1+ will follow.
  Nothing in Part B is implemented yet; it is a specification, and is labelled as
  such so it is not mistaken for a working deployment.

Environment: Windows with Git Bash. All commands are POSIX-shell compatible.

---

# Part A — Current operation (verified)

## A1. Ports and processes

| Unit | Address | Notes |
|---|---|---|
| FastAPI / dashboard | `http://127.0.0.1:8000` | serves API + `dashboard.html` |
| Static assets | `/assets/*`, `/static/*` | Chart.js vendored locally — no CDN |

### Start

```bash
mkdir -p .freebuff/logs
exec python server.py > .freebuff/logs/server.out 2>&1   # BACKGROUND process
```

### Readiness probe (do not assume the banner means ready)

```bash
for i in $(seq 1 30); do
  code=$(curl -s -m 2 -o /dev/null -w '%{http_code}' http://127.0.0.1:8000/api/health || true)
  [ "$code" = "200" ] && { echo "ready after ${i}s"; break; }
  sleep 1
done
```

### Stop (by PID — never by name pattern)

```bash
PID=$(netstat -ano 2>/dev/null | grep ":8000" | grep LISTENING | awk '{print $5}' | head -1)
[ -n "$PID" ] && taskkill //PID "$PID" //F
```

> Never use `pkill -f server.py` / `pgrep -f | xargs kill`: the agent shell's own
> command line can contain the same string.

### Logs

```bash
tail -n 200 .freebuff/logs/server.out
grep -a -E "floorsheet_live failed|backing off|Traceback" .freebuff/logs/server.out | tail -20
```

## A2. Health checks (what "healthy" means)

| Check | Command | Healthy |
|---|---|---|
| Process alive | `curl -s -o /dev/null -w '%{http_code}' :8000/api/health` | `200` |
| No job errors | `curl -s :8000/api/status` → `.errors` | `[]` / `0` |
| Upstream reachable | `.api_status` / `upstream_nepse` | `connected` |
| Data not stale | `/api/diagnostics` → `datasets[].stale` | all `false` |
| Collection ticking | floorsheet row count rises during session | increases |

## A3. Verified operating facts

- Scheduler: **5 jobs + 1 daemon thread** (`server.py:351-419`); market snapshot
  every 30 s; **floorsheet live every 1 s** during session hours; index graphs
  every 5 min; reference data 16:30; full-session floorsheet + archive rebuild
  post-close (15:25 NPT, Sun–Thu). Disable all collection with
  `NEPSE_AUTOCOLLECT=0` (useful for CI).
- Floorsheet archive `floorsheet/<date>.json` is **derived from SQLite**, rebuilt
  every 60 ticks. The database is authoritative — never edit the archive by hand.
- Exchange calendar and clock are Asia/Kathmandu; timestamps stored UTC.
- Live failures are tolerated: one malformed upstream payload skips a tick; ten
  consecutive misses are reported and backed off 30 s.
- Interval is tunable: `NEPSE_FLOORSHEET_INTERVAL` (seconds).

## A4. Routine tasks

```bash
# regression suite
python -m pytest -q

# lossless, checksummed baseline export (safe, read-only)
python tools/export_legacy.py

# drain new trades now instead of waiting for the timer
curl -s -X POST http://127.0.0.1:8000/api/collect/floorsheet
```

## A5. Troubleshooting

| Symptom | Cause | Action |
|---|---|---|
| Dashboard says "Backend unreachable" | server down (verified: last-good data retained) | start server; probe readiness |
| "Connection error" while APIs return 200 | a client-side script throw is being reported as transport failure | check console; vendored assets should prevent this — if it recurs, the asset hash/serving is wrong |
| `TypeError: list indices must be integers` in logs | exchange returned a bare list | already tolerated; if persistent, check upstream status |
| Company/depth endpoint slow | upstream slow, threadpool saturated | check `.freebuff/logs/server.out`; restart if wedged |
| Port 8000 occupied | previous instance still running | stop by PID (A1) |
| `getFloorSheetOf` → 403 | exchange blocks historical floorsheet | expected; not a bug |
| Stale data outside session hours | market closed | check `/api/status.market` before treating as failure |

## A5b. Version control (currently absent)

**This project has no git repository** — `git status` returns
`fatal: not a git repository`. Until that is fixed there is no undo. Before any
further code change:

```bash
git init
git add -A            # .gitignore already excludes data/, nepse.db, *.log, .freebuff
git commit -m "baseline: v4.1.0 as audited 2026-10-29"
git tag baseline-2026-10-29
```

Data is deliberately **not** versioned; it needs its own checkpoint (A6).

## A6. Safety rules for this repository

- Never delete `nepse.db`, `floorsheet/`, `historical/`, or `data/`.
- Never rebuild the floorsheet archive from anything but the DB.
- Take `python tools/export_legacy.py` **before** any schema change; it is the
  rollback checkpoint.
- Do not touch the unrelated `postgres:16-alpine` container on port 5432.

---

# Part B — Target stack (Phase 1+, specification only)

## B1. Prerequisites

Go 1.27 · Node 24 · Docker + Compose · `psql` client (optional) ·
`golang-migrate` **not** required (the API runs its own embedded migrations).

## B2. Layout

```
cmd/api/            chi server
cmd/worker/         job supervisor
internal/{config,httpapi,store,auth,market,ingest,charts,calc}
migrations/         0001_init.up.sql / .down.sql
web/                Next.js App Router
deploy/             compose, nginx, backup scripts
```

## B3. Configuration

Environment only; validated at boot; the process **refuses to start** on missing
required values rather than defaulting to something insecure.

| Variable | Required | Example |
|---|---|---|
| `DATABASE_URL` | yes | `postgres://nepse:***@localhost:5433/nepse?sslmode=disable` |
| `HTTP_ADDR` | no | `:8080` |
| `APP_ORIGIN` | yes | `https://terminal.example.np` |
| `JWT_SIGNING_KEY` | yes | 32+ random bytes, base64 |
| `COOKIE_DOMAIN` / `COOKIE_SECURE` | yes | `.example.np` / `true` |
| `SOURCE_BASE_URL` | yes | exchange base |
| `SOURCE_RATE_LIMIT_RPM` | no | `60` |
| `FLOORSHEET_INTERVAL` | no | `1s` |
| `LOG_LEVEL`, `LOG_FORMAT` | no | `info`, `json` |
| `S3_*` (optional) | no | raw payload archive |

Secrets come from the environment or a secret manager — never committed, never
logged, never in a Dockerfile layer.

## B4. Bring-up

```bash
docker compose -f deploy/docker-compose.yml up -d postgres   # 5433
go run ./cmd/api -migrate                                    # apply migrations
go run ./cmd/api                                             # :8080
go run ./cmd/worker -role=ingest                             # workers
cd web && npm ci && npm run dev                              # :3000
```

Readiness gate (must pass before any deployment is called successful):

```bash
curl -fsS localhost:8080/healthz         # liveness
curl -fsS localhost:8080/readyz | jq     # DB reachable, migrations current, migration checksum match
```

## B5. Migrations

- Forward-only in production; every migration ships with a `.down.sql` tested on a
  scratch database.
- `-migrate` is idempotent and records a checksum per file; an edited applied
  migration aborts startup (prevents silent schema drift).
- Before applying on real data: `pg_dump` to `backups/pre_<migration>_<ts>.dump`
  and verify the dump restores (B7).

## B6. Backups

```bash
# nightly, retained 30 days
pg_dump -Fc -h localhost -p 5433 -U nepse nepse \
  > "backups/nepse_$(date -u +%Y%m%dT%H%M%SZ).dump"

# retire later the same day
pg_backup_rotated_directory (retention 30), checksummed
```

WAL archiving only if a measured RPO under 24h is required.

## B7. Restore rehearsal (quarterly, mandatory)

```bash
createdb -h localhost -p 5433 nepse_restore
pg_restore -h localhost -p 5433 -d nepse_restore backups/nepse_<ts>.dump
psql -h localhost -p 5433 -d nepse_restore -c \
  "select count(*), max(trading_date) from floorsheet_trades;"
# compare against the source-of-truth counts, then drop nepse_restore
```
A backup that has never been restored is not a backup.

## B8. Deploy and rollback

```
image build → migrate (expand-only) → start new api → readyz → shift traffic
→ observe 10 min (error rate, p95, ingestion lag) → keep old container 24h
```

Rollback: previous image tag + the `down` migration for the newest expansion
**only**. Destructive migrations are never combined with code release in the same
deploy.

## B9. Production checklist

- [ ] TLS terminated at the proxy; HSTS; secure headers; CSP without `unsafe-inline`
- [ ] `COOKIE_SECURE=true`, `SameSite=Lax`, CSRF double-submit active
- [ ] Postgres not exposed publicly; least-privilege role for the app
- [ ] Rate limits enabled; export limits at 10/hour/user
- [ ] Entitlements seeded; public export paths closed
- [ ] Metrics scraped; alerts on ingestion lag, job failures, 5xx rate
- [ ] Backup job scheduled and a restore rehearsal recorded
- [ ] Licence status reviewed before enabling public API/exports
      ([LICENSING_REPORT.md](LICENSING_REPORT.md))
- [ ] `go test ./...`, `npm run typecheck`, `npm run lint`, Playwright suite green
- [ ] Legacy Python still running until the Phase 10 acceptance gate
