# NEPSE Platform Verified Feature Inventory & Audit Report

**Date of Audit**: 2026-10-30T11:05:00+05:45  
**Version**: 1.0.0-rc1  
**Audit Standard**: Exhaustive evidence-backed verification across all 25 system modules.

---

## 1. Feature Verification Summary Board

| # | System Module | Verification Method | Status | Verified Evidence & Test Coverage |
| :--- | :--- | :--- | :--- | :--- |
| **1** | Market Overview | Go unit test & Next.js page | ✅ Verified | Indices, turnovers, advances/declines, market cap, float cap |
| **2** | Markets / Live Data | API & Frontend Heatmap | ✅ Verified | Real-time snapshots, 10% circuit halt boundary coloring |
| **3** | Companies Directory | REST API + DB Queries | ✅ Verified | Filter, search, pagination, profile, paid-up capital, status |
| **4** | Stock Valuation Analysis | Go rational arithmetic | ✅ Verified | NUMERIC math for trailing P/E, P/B, P/S, EV, EV/EBITDA |
| **5** | Advanced Charts | Next.js canvas & indicators | ✅ Verified | OHLCV bars, minute vs daily separation, SMA, EMA, RSI, BB |
| **6** | Indices & Sectors | Backend calculation | ✅ Verified | 13 sub-indices, sector weights, daily percentage changes |
| **7** | Market Screener | In-memory evaluation engine | ✅ Verified | 20 metrics, 6 comparison operators, min-bar history gating |
| **8** | Floorsheet & Activity | Trade tape reconciliation | ✅ Verified | Contract dedup, broker buyer/seller mapping, volume tracking |
| **9** | Market Depth | Snapshot ingestion | ✅ Verified | Buy/Sell bid-ask queues, order quantity aggregation |
| **10** | Watchlists | Multi-tenant CRUD | ✅ Verified | User-isolated watchlists, security symbols join, 404 on cross-read |
| **11** | Alerting Pipeline | Fingerprint deduplication | ✅ Verified | Exponential backoff, cooldown windows, delivery inbox |
| **12** | Portfolio Tracking | Multi-lot FIFO & running WAC | ✅ Verified | Strictly decoupled from execution; cash ledger, realized/unrealized P&L |
| **13** | Fundamentals | Accounting identity engine | ✅ Verified | Annual & quarterly annualized ratios, no lookahead bias |
| **14** | Financial Statements | Normalized chart of accounts | ✅ Verified | Balance Sheet, Income Statement, Cash Flow, TTM synthesis |
| **15** | Corporate Actions | Backward adjustment chaining | ✅ Verified | Stock splits, bonus shares, rights issues (TERP), cash dividends |
| **16** | Disclosures & Circulars | Regulatory archive | ✅ Verified | NEPSE, SEBON, NRB circulars, attachments, SHA-256 digests |
| **17** | Quantitative Analytics | Risk & Performance engine | ✅ Verified | TWR, MWR (IRR), Beta, Alpha, Sharpe, Sortino, VaR, CVaR |
| **18** | AI Research Assistant | Evidence grounding | ✅ Verified | Tripartite classification (FACT, CALCULATION, INTERPRETATION) |
| **19** | Institutional Workspaces | Multi-tenant RBAC | ✅ Verified | 5 roles, anti-self-approval rule, concentration, liquidity risk |
| **20** | Historical Explorer | Time-series query | ✅ Verified | Multi-year daily OHLCV bar navigation and query filtering |
| **21** | Bulk Exports | Multi-format generator | ✅ Verified | CSV (comments), JSON (envelopes), Parquet (PAR1 magic) |
| **22** | Developer API & Portal | Cryptographic API keys | ✅ Verified | Prefixed keys (`np_live_`), SHA-256 storage, 4 tier quotas |
| **23** | Account & Authentication | Argon2id + JWT + CSRF | ✅ Verified | Password hashing, timing-equalizer, double-submit CSRF cookies |
| **24** | Platform Administration | Audit trail engine | ✅ Verified | Immutable audit log recording user, action, target, metadata |
| **25** | Health & Observability | Liveness & Readiness probes | ✅ Verified | `/healthz`, `/readyz`, structured slog JSON request logging |

---

## 2. Key Architectural Invariants Proven

1. **Exact Precision Financial Arithmetic**:
   - Zero floating-point drift: All financial arithmetic is backed by Go `math/big.Rat` (`numeric.Decimal`) and stored in PostgreSQL as `NUMERIC`.
2. **Strict Decoupling of Portfolio Tracking from Brokerage**:
   - The platform provides institutional portfolio risk, cost basis, and performance tracking without coupling to broker order-routing APIs, OMS, or trade execution.
3. **Evidence-Grounded AI Research**:
   - The AI assistant refuses speculative hallucination when no source records exist and strictly safeguards portfolio data from unauthorized exposure.
4. **Data Provider Licensing & Attribution Enforcement**:
   - Enforces the Platform Data Licensing Matrix, prohibiting redistribution of raw exchange market feeds while permitting official filings with source attribution.
5. **Maker-Checker Anti-Self-Approval**:
   - Institutional approval workflows programmatically enforce $requester\_id \neq reviewer\_id$.
