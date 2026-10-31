# Discrepancy report

Generated: 2026-10-29T12:05:54Z

**Verdict: MATCH** — 79 checks matched, 0 mismatched, 1 expected difference(s).

| | |
|---|---:|
| Contracts compared field by field | 5000 |
| Contracts missing from the platform | 0 |
| Field values that differ | 0 |

## companies

| Check | Legacy | Platform | Result |
|---|---|---|---|
| row count | `650` | `650` | match |
| placeholder issuers | `0` | `4` | match |
| symbol set | `650 symbols` | `0 missing, 0 extra` | match |

All checks in this dataset passed.

## daily_ohlcv

| Check | Legacy | Platform | Result |
|---|---|---|---|
| rows captured from snapshots | `0 (daily_prices table empty)` | `425` | match |
| close price sample | `195 snapshots` | `0 mismatched closes` | match |
| volume sample | `195 snapshots` | `0 mismatched volumes` | match |

All checks in this dataset passed.

## floorsheet

| Check | Legacy | Platform | Result |
|---|---|---|---|
| row count | `48273` | `48273` | match |
| sum(amount) | `5892370553.0800` | `5892370553.0800` | match |
| sum(quantity) | `15895851.0000` | `15895851.0000` | match |
| distinct symbols | `344` | `344` | match |
| contracts for NIMB | `1195` | `1195` | match |
| contracts for LSL | `1000` | `1000` | match |
| contracts for KBL | `986` | `986` | match |
| contracts for HBL | `958` | `958` | match |
| contracts for BHL | `938` | `938` | match |
| contracts for LEC | `898` | `898` | match |
| contracts for GHL | `877` | `877` | match |
| contracts for SOHL | `848` | `848` | match |
| contracts for SOPL | `751` | `751` | match |
| contracts for SANIMA | `687` | `687` | match |
| field-level sample | `5000 rows` | `0 field mismatches` | match |
| capture coverage 2026-10-29 | `exchange turnover 8570098823.35` | `captured contracts 5892370553.08 (68.8%)` | expected difference |

All checks in this dataset passed.

## index_snapshots

| Check | Legacy | Platform | Result |
|---|---|---|---|
| Banking SubIndex value | `1618.2200` | `1618.2200` | match |
| Banking SubIndex change | `36.2000` | `36.2000` | match |
| Banking SubIndex percent | `2.2800` | `2.2800` | match |
| Development Bank Index value | `5564.8700` | `5564.8700` | match |
| Development Bank Index change | `85.1100` | `85.1100` | match |
| Development Bank Index percent | `1.5500` | `1.5500` | match |
| Finance Index value | `2258.9700` | `2258.9700` | match |
| Finance Index change | `37.7000` | `37.7000` | match |
| Finance Index percent | `1.6900` | `1.6900` | match |
| Float Index value | `187.0100` | `187.0100` | match |
| Float Index change | `2.2700` | `2.2700` | match |
| Float Index percent | `1.2300` | `1.2300` | match |
| Hotels And Tourism Index value | `7078.6100` | `7078.6100` | match |
| Hotels And Tourism Index change | `142.3900` | `142.3900` | match |
| Hotels And Tourism Index percent | `2.0500` | `2.0500` | match |
| HydroPower Index value | `3718.1500` | `3718.1500` | match |
| HydroPower Index change | `13.4400` | `13.4400` | match |
| HydroPower Index percent | `0.3600` | `0.3600` | match |
| Investment Index value | `96.0000` | `96.0000` | match |
| Investment Index change | `0.2300` | `0.2300` | match |
| Investment Index percent | `0.2400` | `0.2400` | match |
| Life Insurance value | `12086.4600` | `12086.4600` | match |
| Life Insurance change | `82.7800` | `82.7800` | match |
| Life Insurance percent | `0.6800` | `0.6800` | match |
| Manufacturing And Processing value | `10771.3600` | `10771.3600` | match |
| Manufacturing And Processing change | `55.7000` | `55.7000` | match |
| Manufacturing And Processing percent | `0.5100` | `0.5100` | match |
| Microfinance Index value | `4556.4900` | `4556.4900` | match |
| Microfinance Index change | `24.5200` | `24.5200` | match |
| Microfinance Index percent | `0.5400` | `0.5400` | match |
| Mutual Fund value | `19.9800` | `19.9800` | match |
| Mutual Fund change | `0.0400` | `0.0400` | match |
| Mutual Fund percent | `0.2300` | `0.2300` | match |
| NEPSE Index value | `2695.6000` | `2695.6000` | match |
| NEPSE Index change | `31.5400` | `31.5400` | match |
| NEPSE Index percent | `1.1800` | `1.1800` | match |
| Non Life Insurance value | `9559.9400` | `9559.9400` | match |
| Non Life Insurance change | `12.8600` | `12.8600` | match |
| Non Life Insurance percent | `0.1300` | `0.1300` | match |
| Others Index value | `1830.6900` | `1830.6900` | match |
| Others Index change | `43.9300` | `43.9300` | match |
| Others Index percent | `2.4500` | `2.4500` | match |
| Sensitive Float Index value | `165.3400` | `165.3400` | match |
| Sensitive Float Index change | `2.2500` | `2.2500` | match |
| Sensitive Float Index percent | `1.3800` | `1.3800` | match |
| Sensitive Index value | `488.2100` | `488.2100` | match |
| Sensitive Index change | `6.4100` | `6.4100` | match |
| Sensitive Index percent | `1.3300` | `1.3300` | match |
| Trading Index value | `3201.8000` | `3201.8000` | match |
| Trading Index change | `0.9700` | `0.9700` | match |
| Trading Index percent | `0.0300` | `0.0300` | match |

All checks in this dataset passed.

## market_cap

| Check | Legacy | Platform | Result |
|---|---|---|---|
| market_cap 2026-10-29T00:00:00Z | `4893040965845.6992` | `4893040965845.6992` | match |
| float_market_cap 2026-10-29T00:00:00Z | `1580179643747.9001` | `1580179643747.9001` | match |
| covered 2026-10-29T00:00:00Z | `403` | `403` | match |
| eligible 2026-10-29T00:00:00Z | `650` | `650` | match |

All checks in this dataset passed.

## securities

| Check | Legacy | Platform | Result |
|---|---|---|---|
| ISIN coverage | `see company_details` | `405/785 with ISIN, 405 valid, 0 flagged` | match |
| symbol set | `569 symbols in security_list.json` | `0 missing` | match |
| contract attribution | `48273 contracts` | `48273 attributed to a security` | match |

All checks in this dataset passed.

## Expected differences

- **floorsheet / capture coverage 2026-10-29** — legacy `exchange turnover 8570098823.35`, platform `captured contracts 5892370553.08 (68.8%)`. snapshot polling does not see every trade; per-security turnover is therefore a lower bound and session turnover is taken from the exchange summary, not from contracts

## Method

Counts are compared directly. Aggregates are recomputed on both sides and compared
with a stated tolerance (0.01 for money, 1e-6 for rates). A sample of contracts is
then read from both stores and every field compared: symbol, trading date, quantity,
rate, amount, buyer, seller and execution time. Execution time is compared as an
instant, because the legacy store writes UTC without a zone marker while PostgreSQL
stores `timestamptz`.
