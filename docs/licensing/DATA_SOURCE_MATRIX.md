# DATA SOURCE MATRIX

**Every source the platform touches, what it can and cannot provide, and how each
claim was verified.** Probes were run on 2026-10-29 against the live exchange
through `nepse_client` 0.1.4, plus inspection of the datasets already on disk.

Legend: `✅ verified` · `🟠 partial` · `⛔ blocked` · `❌ not published`

---

## 1. Source registry

| Source | Kind | Access | Terms status | Used for |
|---|---|---|---|---|
| **Nepal Stock Exchange** (`nepalstock.com.np` / `nepalstock.com`) | Official operator | Unofficial Python wrapper (`nepse_client`), no API key | **Unverified** — no public developer API found; redistribution not established | All market, floorsheet, reference, company data |
| **`nepse-client` (PyPI, 0.1.4)** | Third-party library, not affiliated | Open source | Library licence ≠ data licence | Transport only |
| **SEBON** | Regulator | Public web documents | Public disclosure | Not integrated (see §7) |
| **NRB** | Central bank | Public datasets | Public disclosure | Not integrated |
| **Company investor-relations pages** | Issuer | Public documents | Per-document | Financial reports, AGMs |
| **Licensed commercial vendor** | Contract | — | — | **None connected** |

> Access to a public endpoint does not grant redistribution rights. See
> [LICENSING_REPORT.md](LICENSING_REPORT.md).

---

## 2. Endpoint matrix

Verified shapes/volumes are from live probes or the on-disk dataset that the
collector wrote from that endpoint.

### 2.1 Market and index

| Client method | Dataset produced | Verified shape | Cadence used | History available | Status |
|---|---|---|---|---|---|
| `getMarketStatus()` | `market_status.json` | dict, 3 keys | 30 s | current only | ✅ |
| `getSummary()` | `summary.json` | **list of 4**: turnover, traded shares, transactions, scrips traded | 30 s | current only | ✅ (**no market cap** — see §5) |
| `getNepseIndex()` | `nepse_index.json` | 4 rows | 30 s | current only | ✅ |
| `getNepseSubIndices()` | `sub_indices.json` | 13 rows | 30 s | current only | ✅ |
| `getLiveMarket()` | `live_market.json` | **349 rows** | 30 s | current only | ✅ |
| `getPriceVolume()` | `price_volume.json` | **278 rows** | 30 s | current only | 🟠 diverges from `live_market` (AUDIT D8) |
| `getSupplyDemand()` | `supply_demand.json` | 2 keys: `supplyList` (387), `demandList` (396) | 30 s | current only | ✅ |
| `getDaily*Graph()` × 17 | `daily_*_graph.json` | **233–445 points** | 5 min | **current session only** | 🟠 name is misleading (§5) |
| `getSymbolMarketDepth(symbol)` | on demand | bid/ask ladder | per request | current only | ✅ |
| `getTradingAverage(nDays=180)` | `trading_average.json` | 460 rows | 16:30 | 180 sessional days | ✅ |

### 2.2 Reference and corporate

| Client method | Dataset | Verified shape | Cadence | Status |
|---|---|---|---|---|
| `getCompanyList()` | `company_list.json` | **650**: 410 active / 216 delisted / 24 suspended; 490 equity, 89 debentures, 63 mutual funds, 8 preference; 13 sectors | daily | ✅ |
| `getSecurityList()` | `security_list.json` | 569 rows | daily | ✅ |
| `getCompanyIDKeyMap()` | `company_id_map.json` | 650 keys | daily | ✅ |
| `getSecurityIDKeyMap()` | `security_id_map.json` | 569 keys | daily | ✅ |
| `getSectorScrips()` | `sector_scrips.json` | 14 keys | daily | ✅ |
| `getCompanyDetails(symbol)` | `company_details/<SYM>.json` | 431 files / **425 symbols** | on demand | 🟠 65.4% coverage |
| `getCompanyFinancialDetails()` | per-company reports (≤22) | EPS, P/E, profit, net worth | on demand | 🟠 summary ratios only |
| `getCompanyAGM()` | per-company AGMs | up to 6 | on demand | 🟠 |
| `getCompanyDividend()` | per-company dividends | up to 5 | on demand | 🟠 |
| `getDebentureAndBondList('debenture')` | `debentures_bonds.json` | 78 rows | daily | ✅ |
| `getHolidayList(year)` | `holiday_list*.json` | 46 / 40 rows | daily | ✅ |
| `getNepseNotice(p0,s50)` | `nepse_notice.json` | 50 items | daily | ✅ |
| `getPressRelease(p0,s50)` | `press_release.json` | 21 items | daily | ✅ |
| `getCompanyNewsList(p0,s100)` | `company_news.json` | paged | not currently scheduled | 🟠 |
| `getNewsAndAlertList(p0,s100)` | `news_alerts.json` | paged | not currently scheduled | 🟠 |

### 2.3 Trading and history

| Client method | Verified result | Volume | Window | Status |
|---|---|---|---|---|
| `getFloorSheet()` | newest-first page of trades | page 0 = latest | current session | ✅ |
| `getFloorSheetOf(symbol, date)` | **HTTP 403** from nginx | — | **none** | ⛔ |
| `getPriceVolumeHistory()` (no date) | paged dict, `content` 348 | 348/page | today's snapshot list | ✅ |
| `getPriceVolumeHistory(date)` | bulk daily OHLCV for all securities, `totalPages` 1 | **360 securities** (2026-10-27); 321–362 observed | **≈365 days** | ✅ see §3 |
| `getCompanyPriceVolumeHistory(symbol)` | **233 rows** (NABIL) | per symbol | ~1 year of daily bars | ✅ |
| `getDailyScripPriceGraph(symbol)` | session series | per symbol | current session | 🟠 |
| per-security intraday | **price only, no volume** | 119 points observed | current session | 🟠 |

---

## 3. Historical depth — the decisive numbers

| Question | Answer | Evidence |
|---|---|---|
| How far back does **bulk daily OHLCV** go? | **≈365 days** | `getPriceVolumeHistory("2025-10-29")` → OK; `("2025-06-02")` → `[500] Searched Date is not valid` |
| How much data is that? | ~230 trading days × ~340 securities ≈ **78,000 OHLCV rows** | 360 securities on 2026-10-27; 321–362 range observed across probes |
| Does bulk daily include market cap? | **Yes** — each row carries `marketCapitalization` | probe of `content[0]` |
| How far back does **per-symbol** history go? | **233 daily bars** (NABIL) | `getCompanyPriceVolumeHistory` |
| How far back does **floorsheet** go? | **Zero.** Only what we collect forward | 403 on `getFloorSheetOf`; DB has 1 date |
| How far back does **intraday** go? | **Zero.** Current session only | `daily_*_graph.json` spans 3.8 h of today |
| How far back does **index daily** go? | Only our own `index_snapshots` (**17 rows**) | DB count |

**Consequence:** a professional terminal can open with ~1 year of daily price
history and ~1 year of market-cap history — but **cannot** show historical
floorsheet or historical intraday from this source, at any price.

---

## 4. Field-level availability

| Field | Availability | Notes |
|---|---|---|
| Open / high / low / close | ✅ (daily, bulk) | from `getPriceVolumeHistory(date)` |
| Volume / turnover / trades | ✅ (daily, bulk) | |
| Last traded price | ✅ live | |
| Previous close | ✅ | |
| `listedShares` / `publicShares` | ✅ per-security | basis of capitalisation |
| **Free-float factor** | ❌ | `publicShares` is the only public proxy — not a free-float factor |
| **Market cap in summary** | ❌ | computed from shares × close, disclosed as derived |
| Intraday volume | ❌ | intraday series is price-only |
| **Broker name** | ❌ | `buyerBrokerName` / `sellerBrokerName` are `null`; member ids present but masked |
| Buyer/seller member id | 🟠 | present on some rows (`45`, `12`), `null` on others |
| Execution timestamp | ✅ | `tradeTime`, e.g. `2026-10-29T11:53:59.507269` |
| Company email / website | ✅ | business contact data, not personal |
| Full financial statements | ❌ | only summary metrics |

---

## 5. Naming traps (verified — do not trust the method name)

| Trap | Reality | Impact if trusted |
|---|---|---|
| `getDailyNepseIndexGraph` | Returns **one session**, 445 points over 13,620 s | A "daily" chart built from it silently shows 4 hours |
| `getTopTenTradeScrips` | Returns **349 rows** (the full ranked list) | "Top 10" widgets show everything |
| `getTopGainers` | Returns **187 rows** | same |
| `getSummary` | Returns **4 fields**, no capitalisation | Anyone assuming market cap is present will compute from nothing |
| `getPriceVolume` vs `getLiveMarket` | 278 vs 349 rows, different capture times | Breadth/movers computed from the wrong one disagree |
| `getFloorSheetOf` | Exists in the client, **always 403** | A backfill loop that trusts it will spin |

---

## 6. Reliability observations

| Behaviour | Observed | Handling in code |
|---|---|---|
| Intermittent bare-list payload from floorsheet endpoint | `TypeError: list indices must be integers` inside the library | Treated as a transient data error: skip the tick, escalate after 10 consecutive misses |
| Server errors on old dates | `[500] Searched Date is not valid` | Not retried — treated as out-of-range |
| 403 with HTML body | nginx edge rejection | Not retried |
| Slow responses (up to 100 s) | company detail / depth | Offloaded via `run_in_threadpool` |
| Concurrent client use | corrupted internal request state | Single API lock |

---

## 7. Acquisition plan for the missing data

| Missing | Realistic path | Effort | Cost |
|---|---|---|---|
| Historical floorsheet | Licensed vendor, or accept forward-only | Requires contract | Unknown |
| Historical intraday | Licensed feed | Requires contract | Unknown |
| Free-float factors | Exchange application or vendor | Low, once licensed | Unknown |
| Broker-level identity | Licensed data; **never** infer | n/a | n/a |
| Full financial statements | Company filings parsing (PDF/HTML) | High | Free |
| Pre-2025 daily prices | Vendor archive | Low | Unknown |

Each of these is already surfaced in the UI as a labelled unavailability rather
than a stub, per [DATA_PROVENANCE.md](DATA_PROVENANCE.md) §6.

---

## 8. Reproduction

```bash
python - <<'PY'
from nepse_client.sync_client import NepseClient
c = NepseClient()
print(len(c.getMarketStatus()), len(c.getSummary()))          # 3 keys, 4 rows
b = c.getPriceVolumeHistory("2026-10-27")
print(b["totalElements"], b["totalPages"])                    # 360 1
print(c.getPriceVolumeHistory("2025-10-29")["totalElements"]) # OK, ~350
try: c.getPriceVolumeHistory("2025-06-02")
except Exception as e: print(type(e).__name__, e)              # NepseServerError 500
try: c.getFloorSheetOf("NABIL", "2026-10-28")
except Exception as e: print(type(e).__name__, e)              # NepseNetworkError 403
PY
```
