# DATA PROVENANCE

**Phase 0 deliverable.** How every number in the terminal can be traced to a
source, a moment, a transformation and a licence — and how that trace is shown to
the user rather than hidden in a log.

---

## 1. Why this document exists

Two facts from the audit make provenance non-optional:

1. **The exchange summary does not contain market capitalisation.** `getSummary()`
   returns exactly four fields (`AUDIT_REPORT.md` §4.2). Total market cap in this
   product is therefore **derived**, not reported. A derived number with no
   stated method is indistinguishable from an invented one.
2. **The client library is unofficial** ([LICENSING_REPORT.md](LICENSING_REPORT.md)).
   Access to a public endpoint is not permission to redistribute. Without a
   licence class attached to data, the product cannot tell a permitted read from
   a prohibited export.

Provenance is the mechanism that lets the terminal show real, useful numbers
while remaining honest about their standing.

---

## 2. Required metadata on every market record

| Field | Column | Example |
|---|---|---|
| Provider | `data_source_id` → `data_sources.slug` | `nepse_unofficial` |
| Endpoint / document | `raw_payloads.endpoint` | `POST /api/nots/security/floorsheet` |
| Original identifier | `symbol` as received, `contract_id` raw | `NABIL`, `2026102912345678` |
| Business date | `trading_date DATE` | `2026-10-29` |
| Event time | `executed_at TIMESTAMPTZ` | `null` when source omits it |
| Collection time | `ingested_at TIMESTAMPTZ` | `2026-10-29T13:42:14.201Z` |
| Job | `ingestion_job_id` | uuid → kind, attempt, trace_id |
| Raw payload | `raw_sha256` → `raw_payloads` | SHA-256 of the exact bytes |
| Transformation | `calculation` string | `market_cap@v1` |
| Validation | `data_quality_issues` rows | `ohlc_band`, `turnover_mismatch` |
| Licence | `data_sources.license_class`, `.redistribution` | `public_unverified`, `prohibited` |

Where a record is the result of a computation, the record also carries the
**method version** and the **coverage numerator/denominator** that produced it.

---

## 3. Classification

| Class | Meaning | Examples in this system | Public API | Export |
|---|---|---|---|---|
| `official_verified` | Published by the exchange/regulator, terms permit use | *none currently* | ✅ | per terms |
| `third_party_licensed` | Contracted vendor feed | *none yet* | per contract | per contract |
| `public_unverified` | Publicly accessible, redistribution terms unverified | All NEPSE market data via the unofficial client; floorsheet; company reports | ⚠️ read-only, attributed, rate-limited | ❌ **blocked** |
| `derived` | Computed from other classes by a documented formula | market cap, float cap, breadth ratio, indicators, TWR/XIRR | inherits the **most restrictive** input class | inherits |
| `restricted` | Known to require permission | licensed vendor data before contract | ❌ | ❌ |
| `unavailable` | Source does not publish it | historical intraday, pre-2026-10-29 floorsheet, broker names, free-float factors | labelled `DATA_UNAVAILABLE` | ❌ |

**Rule:** a derived dataset is never less restricted than its strictest input.
Market cap inherits `public_unverified` from `stockListedShares` and `close`.

---

## 4. Calculation registry

Every computed value names a versioned method. A change to the formula creates a
new version; historical rows keep the version that produced them.

### `market_cap@v1`
```
total_market_cap = Σ (listed_shares_i × close_price_i)
  over securities where status = listed AND close_price_i is present
coverage = { covered: 403, eligible: 410, excluded_delisted: 240 }
```
Disclosed because a partial sum presented as "total" understates the market by the
excluded share. Reproducible from `securities.listed_shares` +
`daily_ohlcv.close`.

### `float_market_cap@v1`
```
float_market_cap = Σ (public_shares_i × close_price_i)
```
`public_shares` is the exchange's `publicShares`. It is **not** a verified
free-float factor — it is presented as *public-share capitalisation*, and labelled
as such wherever it appears.

### `breadth@v1`
```
ratio = advanced / declined            (null when declined = 0)
unchanged includes securities with zero change and zero trades
```

### `change@v1`
```
point_change = close − previous_close
percent_change = point_change / previous_close × 100   (null when previous_close = 0)
```
Never computed across a corporate action without adjustment; adjusted series are
flagged `adjusted: true`.

### `indicator:<name>@v1`
Each indicator registers its parameters, warm-up length and missing-data policy.
Output arrays align to input bars with explicit `null`s during warm-up rather than
dropping the leading bars (dropping misaligns the x-axis and silently shifts dates).

### `twr@v1` / `xirr@v1`
Time-weighted return uses daily valuation breakpoints; XIRR solves the cash-flow
polynomial with a documented tolerance and iteration cap, returning `null` rather
than a divergent guess when no sign change exists.

---

## 5. Temporal correctness rules

These are the specific bugs the legacy system hit; they become invariants.

| Rule | Rationale |
|---|---|
| Store UTC; convert NPT once, at the adapter boundary | Duplicate conversions shifted times by 5h45m |
| `DATE` for trading dates, from the exchange, never from server clock | Server clock produced a "today" with no session |
| `executed_at` is `null` when the source omits it — never back-filled with `ingested_at` | Conflating them fabricates execution times |
| Watermarks are per-dataset, not global | A 1-second floorsheet poll must not make the 30-second summary look fresh |
| "Last successful update" excludes high-frequency maintenance jobs | Otherwise the field reads "just now" even when market data failed |
| Market open/closed derives from the calendar, independent of network | A closed exchange is not a connection failure |

---

## 6. What the user sees

Every panel carries a provenance footer, and the pattern is uniform:

```
Source: NEPSE (unofficial client) · public, terms unverified
As of: 29 Oct 2026, 13:42 NPT · collected 13:42:14Z
Method: market_cap@v1 — 403 of 410 listed companies
```

On unavailability the panel states the reason, not an empty box:

| Situation | UI |
|---|---|
| Source doesn't publish it | `Not published by NEPSE — no licensed source connected` |
| Data too old to use | `Stale: last update 4h 12m ago` (amber, timestamp shown) |
| Partial coverage | `403 / 410 companies` with a tooltip listing exclusions |
| Licence blocks an action | Export button disabled with `Requires a data licence — see Licensing` |
| AI lacks evidence | Citations block with `Insufficient evidence for this question` |

---

## 7. Export and redistribution gate

```
request → session → role → entitlement(subject, license_class, dataset) → audit_logs
```

- `public_unverified` datasets: **never** exportable through a public endpoint,
  regardless of role. The current legacy export endpoint is a demonstration of the
  gap, and is one of the items this migration closes.
- Every successful export writes `audit_logs(action='export', target=dataset,
  detail={filters, rows, license_class})`.
- Signed URLs expire in 5 minutes and are single-use.
- Rate limit: 10 exports/hour/user.

---

## 8. AI grounding rules

The research assistant may only state facts that appear in retrieved rows, and
every claim carries a citation to a record id. Responses are structured:

```jsonc
{
  "facts":       [ { "claim": "…", "citation": { "table": "financial_metrics", "id": "…" } } ],
  "computed":    [ { "claim": "…", "calculation": "pe@v1", "inputs": [ … ] } ],
  "interpretation": "…",           // clearly separated from facts
  "coverage_notes": [ "Only 22 of 650 companies have a full report on file." ],
  "insufficient_evidence": false
}
```

Hard prohibitions: no price or earnings figure not present in the database; no
citation to a document not stored; no causal claim ("fell because…") without a
cited event; no portfolio data sent to an external model provider without explicit
per-user consent.
