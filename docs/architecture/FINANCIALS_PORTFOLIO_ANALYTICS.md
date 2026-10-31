# Financial Statements, Ratios, Corporate Actions, Portfolio & Risk Analytics

This document specifies the implementation of normalized company financial statements, reporting-period-aware ratios, corporate action adjustment policies, portfolio tracking with cost basis and P&L, benchmark performance, and risk analytics for the NEPSE platform.

---

## 1. High-Precision Arithmetic (`internal/numeric`)

All monetary amounts, share counts, index values, and ratio calculations use `numeric.Decimal`, backed by exact rational arithmetic (`math/big.Rat`). This avoids floating point cancellation and rounding drift when dealing with figures like Nepal's Rs 4.8-trillion market capitalisation or small fractional tax rates.

- **Storage**: `NUMERIC(24,6)`, `NUMERIC(24,4)`, `NUMERIC(20,4)`, `NUMERIC(18,6)`
- **JSON Serialization**: Encoded as exact string representations.
- **Operations**: `Add`, `Sub`, `Mul`, `Div` (with division-by-zero guards), `Quo`, `Round(scale)`, `Format(scale)`.

---

## 2. Normalized Company Financial Statements (`internal/financials`)

### 2.1 Reporting Periods
Financial statements are bound to explicit reporting periods:
- **Period Types**: `Q1`, `Q2`, `Q3`, `Q4`, `ANNUAL`, `TTM`.
- **Filing Date & As-Of Date**: Prevents look-ahead bias. Financial ratios only evaluate statements filed on or before the valuation date.
- **Audit Status**: `unaudited`, `audited`, `restated`, `preliminary`.

### 2.2 Standard Chart of Accounts & Normalized Lines
- **Balance Sheet**:
  - `total_assets`, `current_assets`, `cash_and_equivalents`, `investments`
  - `total_liabilities`, `current_liabilities`, `total_debt`, `long_term_debt`
  - `shareholders_equity`, `paid_up_capital`, `reserves_and_surplus`, `retained_earnings`
  - `total_shares_outstanding`, `book_value_per_share`
- **Income Statement**:
  - `revenue`, `cost_of_revenue`, `gross_profit`, `operating_expenses`
  - `operating_income`, `interest_expense`, `tax_expense`, `net_income`
  - `eps_basic`, `eps_diluted`
- **Cash Flow**:
  - `operating_cash_flow`, `investing_cash_flow`, `financing_cash_flow`
  - `capex`, `free_cash_flow`, `net_change_in_cash`

### 2.3 Accounting Validation Rules
- Balance Sheet Identity: $\text{Total Assets} = \text{Total Liabilities} + \text{Shareholders' Equity}$
- Gross Profit: $\text{Gross Profit} = \text{Revenue} - \text{Cost of Revenue}$
- Operating Income: $\text{Operating Income} = \text{Gross Profit} - \text{Operating Expenses}$
- Free Cash Flow: $\text{Free Cash Flow} = \text{Operating Cash Flow} - \text{CapEx}$
- Book Value Per Share: $\text{BVPS} = \frac{\text{Shareholders' Equity}}{\text{Total Shares Outstanding}}$
- Basic EPS: $\text{EPS} = \frac{\text{Net Income}}{\text{Total Shares Outstanding}}$

### 2.4 TTM (Trailing Twelve Months) Synthesis
When generating TTM statements from 4 consecutive quarters ($Q_t, Q_{t-1}, Q_{t-2}, Q_{t-3}$):
- **Flow items** (Revenue, Net Income, Operating Cash Flow, CapEx) are summed across the 4 quarters.
- **Stock items** (Assets, Liabilities, Equity, Cash) are taken from the latest quarter's closing balance sheet.
- $\text{TTM EPS} = \frac{\sum_{i=1}^4 \text{Net Income}_i}{\text{Latest Shares Outstanding}}$.

---

## 3. Reporting-Period-Aware Ratios (`internal/ratios`)

Flow metrics in interim statements (Q1, Q2, Q3) are annualized using audited quarterly factors:
$$\text{Factor}(Q_1) = 4.0, \quad \text{Factor}(Q_2) = 2.0, \quad \text{Factor}(Q_3) = \frac{4}{3} \approx 1.333333, \quad \text{Factor}(Q_4 / \text{Annual} / \text{TTM}) = 1.0$$

- **Price-to-Earnings (P/E)**:
  $$\text{P/E} = \frac{\text{Market Price}}{\text{Annualized EPS}}$$
- **Price-to-Book (P/B)**:
  $$\text{P/B} = \frac{\text{Market Price}}{\text{Book Value Per Share}}$$
- **Price-to-Sales (P/S)**:
  $$\text{P/S} = \frac{\text{Market Capitalization}}{\text{Annualized Revenue}}$$
- **Enterprise Value (EV) & EV/EBITDA**:
  $$\text{EV} = \text{Market Cap} + \text{Total Debt} - \text{Cash \& Cash Equivalents}$$
  $$\text{EV/EBITDA} = \frac{\text{EV}}{\text{Annualized Operating Income}}$$
- **Return on Equity (ROE)**:
  $$\text{ROE} = \frac{\text{Annualized Net Income}}{\text{Shareholders' Equity}}$$
- **Return on Assets (ROA)**:
  $$\text{ROA} = \frac{\text{Annualized Net Income}}{\text{Total Assets}}$$
- **Liquidity & Solvency**:
  $$\text{Current Ratio} = \frac{\text{Current Assets}}{\text{Current Liabilities}}$$
  $$\text{Quick Ratio} = \frac{\text{Cash} + \text{Investments}}{\text{Current Liabilities}}$$
  $$\text{Debt-to-Equity} = \frac{\text{Total Debt}}{\text{Shareholders' Equity}}$$
  $$\text{Interest Coverage} = \frac{\text{Operating Income}}{\text{Interest Expense}}$$

---

## 4. Dividends & Historical Price Adjustment Policies (`internal/corporateaction`)

### 4.1 Corporate Action Types
- `cash_dividend`: Cash distribution per share or percentage of par (par = 100).
- `bonus_share`: Stock dividend expanding share count.
- `rights_issue`: Rights offering with ratio and subscription price.
- `stock_split`: Split ratio ($N_{new} / N_{old}$).
- `reverse_split`: Consolidation.
- `merger`: Corporate acquisition/merger swap.

### 4.2 Adjustment Factor Formulas
Pre-event historical prices are multiplied by factor $f$; pre-event historical volume is divided by factor $f$:

1. **Stock Split ($S_{from} \to S_{to}$)**:
   $$f = \frac{S_{from}}{S_{to}}$$
2. **Bonus Share ($B\%$ bonus)**:
   $$f = \frac{1}{1 + \frac{B}{100}}$$
3. **Rights Issue ($r$ ratio at $P_{rights}$ subscription price)**:
   Theoretical Ex-Rights Price (TERP):
   $$\text{TERP} = \frac{P_{cum} + r \cdot P_{rights}}{1 + r}$$
   $$f_{rights} = \frac{\text{TERP}}{P_{cum}} = \frac{P_{cum} + r \cdot P_{rights}}{(1 + r) \cdot P_{cum}}$$
4. **Cash Dividend ($D$ cash per share)**:
   $$f_{div} = \frac{P_{cum} - D}{P_{cum}}$$

### 4.3 Policies Supported
- `unadjusted`: Raw traded prices.
- `split_bonus`: Adjusts only for non-dilutive share count changes (splits and bonus shares).
- `split_bonus_rights`: Adjusts for splits, bonus shares, and rights offerings via TERP.
- `total_return`: Fully adjusts for splits, bonus shares, rights, and cash dividend reinvestment.

Backward-chaining cumulative adjustment applies:
$$\text{Cumulative Factor}_t = \prod_{k \text{ with } \text{ex\_date}_k > t} f_k$$

---

## 5. Portfolio Tracking & Ledger (`internal/portfolio`)

### 5.1 Architectural Boundary: Separation from Brokerage Execution
Portfolio tracking is strictly decoupled from brokerage execution.
- **In-Scope for Tracking**: Post-trade ledger, cash balances, deposits/withdrawals, multi-lot FIFO and WAC cost basis accounting, realized and unrealized P&L, corporate action absorption, NEPSE transaction fees, SEBON fees, DP fees, and Capital Gains Tax (CGT).
- **Decoupled from Brokerage Execution**: Order placement, order routing gateways, execution state machines, FIX/OMSYS gateways, and broker TMS connectivity are separated out into dedicated external execution adapters.

### 5.2 NEPSE Transaction Fees & Capital Gains Tax (CGT)
- **Broker Commission**:
  - Turnover $\le 50,000 \implies 0.40\%$
  - $50,000 < \text{Turnover} \le 500,000 \implies 0.37\%$
  - $500,000 < \text{Turnover} \le 2,000,000 \implies 0.34\%$
  - $2,000,000 < \text{Turnover} \le 10,000,000 \implies 0.30\%$
  - $\text{Turnover} > 10,000,000 \implies 0.27\%$
  - Minimum commission: Rs 10.00.
- **SEBON Regulatory Fee**: $0.015\%$ of turnover.
- **DP Fee**: Flat Rs 25.00 per trade.
- **Capital Gains Tax (CGT)**:
  - Individual Short-Term ($\le 365$ days): $7.5\%$
  - Individual Long-Term ($> 365$ days): $5.0\%$
  - Institutional: $10.0\%$

### 5.3 Cost Basis Accounting Methods
- **Weighted Average Cost (WAC / Nepal WACC standard)**:
  - Purchases update running average:
    $$\text{WAC}_{new} = \frac{\text{Cost}_{prev} + \text{Net Purchase Cost}}{\text{Qty}_{prev} + \text{Qty}_{new}}$$
  - Sells realize P&L:
    $$\text{Realized P&L} = (\text{Sale Proceeds} - \text{Sale Fees}) - (\text{WAC} \times \text{Sold Qty})$$
  - Bonus Shares arrive at Rs 0 cost basis, reducing average cost per share:
    $$\text{WAC}_{after\_bonus} = \frac{\text{Total Cost}}{\text{Qty}_{prev} + \text{Qty}_{bonus}}$$
  - Rights Issues absorb shares at subscription price $P_{rights}$:
    $$\text{WAC}_{after\_rights} = \frac{\text{Cost}_{prev} + (\text{Qty}_{rights} \times P_{rights})}{\text{Qty}_{prev} + \text{Qty}_{rights}}$$
- **First-In, First-Out (FIFO)**:
  - Tracks discrete purchase lots (`portfolio_lots`).
  - Sells deplete oldest lots first and calculate holding periods for short-term vs long-term CGT rates.

---

## 6. Benchmark Performance & Risk Analytics (`internal/analytics`)

### 6.1 Return Metrics
- **Time-Weighted Return (TWR)**:
  Sub-period return neutralizing cash flows:
  $$R_{p, t} = \frac{V_t - (V_{t-1} + CF_t)}{V_{t-1} + CF_t}$$
  $$1 + \text{TWR} = \prod_{t=1}^N (1 + R_{p, t})$$
- **Money-Weighted Return (MWR / IRR)**:
  Solves for discount rate $r$ in discounted cash flow equation:
  $$V_0 + \sum_{t=1}^T \frac{CF_t}{(1 + r)^{\frac{t}{252}}} = \frac{V_T}{(1 + r)^{\frac{T}{252}}}$$
- **Annualized Return (CAGR)**:
  $$\text{CAGR} = (1 + \text{TWR})^{\frac{252}{N}} - 1$$

### 6.2 Risk Analytics (Benchmark: NEPSE Index)
- **Annualized Volatilities**: $\sigma_p = \text{std}(R_p) \times \sqrt{252}$, $\sigma_m = \text{std}(R_m) \times \sqrt{252}$
- **Beta ($\beta$)**:
  $$\beta = \frac{\text{Cov}(R_p, R_m)}{\text{Var}(R_m)}$$
- **Jensen's Alpha ($\alpha$)**:
  $$\alpha = R_{p, ann} - [R_f + \beta (R_{m, ann} - R_f)]$$
- **Sharpe Ratio**:
  $$\text{Sharpe} = \frac{R_{p, ann} - R_f}{\sigma_p}$$
- **Sortino Ratio**:
  $$\sigma_{down} = \sqrt{\frac{1}{N} \sum_{t=1}^N \min(0, R_{p, t} - R_{f, daily})^2} \times \sqrt{252}$$
  $$\text{Sortino} = \frac{R_{p, ann} - R_f}{\sigma_{down}}$$
- **Treynor Ratio**:
  $$\text{Treynor} = \frac{R_{p, ann} - R_f}{\beta}$$
- **Maximum Drawdown (MDD)**:
  $$\text{Drawdown}_t = \frac{\max_{s \le t} V_s - V_t}{\max_{s \le t} V_s}$$
  $$\text{MDD} = \max_t (\text{Drawdown}_t)$$
  Tracks Peak Date, Trough Date, and Current Drawdown.
- **Value at Risk (VaR)**:
  - Parametric VaR (95% and 99%): $\text{VaR}_\alpha = -(R_{mean} - Z_\alpha \cdot \sigma)$
  - Historical VaR (95% and 99% empirical quantiles).
  - Conditional VaR (CVaR / Expected Shortfall): Expected loss beyond VaR.
- **Tracking Error & Information Ratio**:
  $$\text{Tracking Error} = \text{std}(R_p - R_m) \times \sqrt{252}$$
  $$\text{Information Ratio} = \frac{R_{p, ann} - R_{m, ann}}{\text{Tracking Error}}$$
- **Market Capture Ratios**: Up-Market and Down-Market capture relative to NEPSE.

---

## 7. API Endpoints

| Method | Path | Summary | Auth |
|---|---|---|---|
| `GET` | `/api/v1/companies/{symbol}/financials` | Normalized multi-period financial statements | Public |
| `POST` | `/api/v1/companies/{symbol}/financials` | Ingest normalized financial statement | Public / Admin |
| `GET` | `/api/v1/companies/{symbol}/ratios` | Reporting-period-aware ratios as-of date | Public |
| `GET` | `/api/v1/corporate-actions` | Query corporate actions across exchange | Public |
| `GET` | `/api/v1/companies/{symbol}/corporate-actions` | Corporate actions for a security | Public |
| `POST` | `/api/v1/companies/{symbol}/corporate-actions` | Register corporate action | Public / Admin |
| `GET` | `/api/v1/companies/{symbol}/adjusted-prices` | Adjusted historical prices (`unadjusted`, `split_bonus`, `split_bonus_rights`, `total_return`) | Public |
| `GET` | `/api/v1/portfolios` | List user portfolios | Cookie / JWT |
| `POST` | `/api/v1/portfolios` | Create portfolio tracker | Cookie / JWT + CSRF |
| `GET` | `/api/v1/portfolios/{id}` | Get portfolio, positions, market value, P&L | Cookie / JWT |
| `DELETE` | `/api/v1/portfolios/{id}` | Delete portfolio | Cookie / JWT + CSRF |
| `GET` | `/api/v1/portfolios/{id}/transactions` | Paginated transaction ledger | Cookie / JWT |
| `POST` | `/api/v1/portfolios/{id}/transactions` | Record portfolio transaction | Cookie / JWT + CSRF |
| `GET` | `/api/v1/portfolios/{id}/lots` | Get tax lots (FIFO) | Cookie / JWT |
| `GET` | `/api/v1/portfolios/{id}/analytics` | Benchmark performance & risk analytics | Cookie / JWT |
