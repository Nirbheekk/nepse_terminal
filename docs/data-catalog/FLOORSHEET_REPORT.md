# Floorsheet ingestion, coverage and analysis

Generated **2026-10-30T02:44:56Z** by `go run ./cmd/floorsheet report`.

Every figure below is read from PostgreSQL or recomputed from the archived page
files at generation time. Where the source cannot supply something, it is named
as unavailable rather than filled in.

## 1. What the source actually serves

The trade tape is published at `/api/nots/nepse-data/floorsheet` (bulk, paged
by `page`, sorted by `contractId` descending) and
`/api/nots/security/floorsheet/{securityId}?businessDate=…` (per symbol).
Both were probed directly. Results:

| probe | endpoint | HTTP | reported pages | reported rows | first row date |
|---|---|---|---|---|---|
| `bulk live page 0` | `/api/nots/nepse-data/floorsheet?…` | 200 | 140 | 69619 | 2026-10-29 |
| `bulk live page 1` | `/api/nots/nepse-data/floorsheet?…` | 200 | 140 | 69619 | 2026-10-29 |
| `bulk businessDate=2026-10-29` | `/api/nots/nepse-data/floorsheet?…` | 200 | 140 | 69619 | 2026-10-29 |
| `bulk businessDate=2026-10-28 (a trading day)` | `/api/nots/nepse-data/floorsheet?…` | 200 | — | — | empty (`list`) |
| `bulk businessDate=2026-09-15` | `/api/nots/nepse-data/floorsheet?…` | 200 | — | — | empty (`list`) |
| `per-symbol NABIL 2026-10-28` | `/api/nots/security/floorsheet/131?…` | 403 | — | — | nginx block |
| `per-symbol NABIL 2026-09-15` | `/api/nots/security/floorsheet/131?…` | 403 | — | — | nginx block |

Probed 2026-10-29T17:40:48.432958+00:00 against `https://www.nepalstock.com`.

Conclusions, from those responses:

- **Only the live session is served.** `businessDate` is honoured as a filter
  and returns an empty sheet for any earlier date — including dates that
  demonstrably traded, which is why an empty answer cannot be read as a
  holiday.
- **The per-symbol historical endpoint is blocked** (`403`, an nginx denial,
  not an application error), so it is not an alternative route to history.
- Pagination is real and bounded: `page` is zero-based, `size` is capped by
  the server, and `totalPages`/`totalElements` describe the whole session.

## 2. Verified dates

- Earliest verified session: **2026-10-29**
- Latest verified session: **2026-10-29**
- Sessions holding at least one contract: **2026-10-29 → 2026-10-29**
- Trading sessions tracked in total: **2**

### Sessions with no trade archive

1 session(s) have price data but no stored contracts:

- 2026-10-28 — not served by the source retroactively; unrecoverable without a licensed feed

### Run receipts

| date | status | pages | rows in pages | unique | reported | missing | converged | written |
|---|---|---|---|---|---|---|---|---|
| 2026-10-29 | success | 140/140 | 69619 | 69619 | 69619 | 0 | true | 69619 |

## 3. Pagination and completeness

### 2026-10-29

- contracts stored: **69,619**
- contracts the exchange reported: **69,619**
- pages recorded: **140 of 140**
- capture converged: **true**
- scrips traded: **350**
- contract turnover: **8570098823.3500**
- verdict: **complete**

Re-verified from the page files during this report:

- digest failures: 0 · date mismatches: 0
- duplicate contract rows across sweeps: 0
- contracts missing against the reported total: 0

## 4. Execution timestamps

### 2026-10-29

- precision as supplied: **mixed: 62,587 microsecond, 7,024 millisecond, 8 second-resolution**
- zone: **naive local time read as Asia/Kathmandu (UTC+05:45); stored as UTC**
- contracts with a trade time: 69,619
- contracts with none: 0 (stored as NULL, never replaced with the capture clock)
- original values are preserved verbatim in `floorsheet_trades.raw`

## 5. Deduplication

`contract_id` is the natural key and is unique across the whole table, so
replaying any page, sweep or date is a no-op. Duplicates are collapsed before
staging and counted, not silently overwritten:

- 2026-10-29: 0 duplicate row(s) collapsed

## 6. Broker summaries

Session 2026-10-29:

- contracts: 69,619
- with a buyer member id: 69,619
- with a seller member id: 69,619
- attributable to at least one member: 69,619
- **not attributable: 0**
- distinct members seen: 91
- contracts carrying a member **name**: 69,619 across 91 distinct names

Broker turnover is summed from contract amounts per member id and covers
only the attributable contracts; it is labelled with its coverage in the UI
and the remainder is never allocated by inference. A member id is the key,
so a contract that has a name but no id is still not attributable.

## 7. Reconciliation against the exchange's own totals

Session 2026-10-29:

| measure | from contracts | exchange-reported | difference |
|---|---|---|---|
| turnover | 8570098823.3500 | 8570098823.3500 | 0 |
| quantity | 23258009.0000 | 23258009.0000 | 0 |

The contract tape reconciles to the exchange's reported turnover exactly.
This is the completeness proof for the session: a skipped page cannot
reconcile, and the earlier single-pass capture did not.

## 8. Limits, stated plainly

- **No historical backfill is possible from this source.** Only the live
  session is served; earlier dates return an empty sheet and the per-symbol
  historical endpoint is blocked. Nothing here fills that gap, and no date is
  presented as covered unless contracts for it are stored.
- **Intraday history** does not exist: the tape is a session snapshot, not a
  time series. Contracts are stamped with the exchange's execution time, but
  the capture is not continuous, so whether a price traded at a given minute
  cannot be answered from this archive alone.
- **Broker identity**: a member id is the stable key. Names are present on the
  contracts that disclose them and absent (not inferred) on the rest.
- **Licence**: the tape is captured from an unofficial client and is
  unlicensed for redistribution. Exports carry that notice.
- **Page drift** is only detectable against `totalElements`. If the exchange
  under-reported its own total, the archive would agree with it and both would
  be wrong together — the turnover reconciliation is the independent check.

## 9. How to reproduce

```bash
go run ./cmd/floorsheet verify            # page, digest and count checks
go run ./cmd/floorsheet ingest -date YYYY-MM-DD
go run ./cmd/floorsheet brokers -date YYYY-MM-DD
go run ./cmd/floorsheet report            # this file
```
