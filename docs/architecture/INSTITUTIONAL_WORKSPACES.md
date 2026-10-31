# Institutional Workspaces, Multi-Tenant RBAC & Risk Architecture

This document specifies the organization-aware institutional workspace architecture, 5-tier role-based access control (RBAC), multi-portfolio management, institutional research notes, liquidity and concentration risk engines, multi-factor scenario stress testing, four-eyes approval workflows, scheduled reporting, export permission gating, and immutable audit trails.

---

## 1. Multi-Tenant Organization Architecture & Tenant Isolation

Institutional clients operate in dedicated, isolated organization workspaces:
- **Organizations Table** (`organizations`):
  - `id`: Unique UUID tenant identifier.
  - `name`: Organization name (e.g., "Nepal Capital Investment Trust").
  - `slug`: Unique human-readable URL slug.
  - `plan`: `trial`, `pro`, `institutional`, `enterprise`.
  - `status`: `active`, `suspended`, `archived`.
- **Tenant Isolation Enforcement**:
  - Every institutional query and mutation filters strictly on `organization_id`.
  - Cross-tenant queries are blocked at the database and handler boundaries (`ErrNotOrgMember`, returning HTTP 403 Forbidden).
  - Cross-organization leakage of portfolio holdings, cash balances, research notes, approvals, or audit logs is prevented by schema and application invariants.

---

## 2. 5-Tier RBAC & Granular Permission Matrix (`internal/institutional/rbac.go`)

### 2.1 Defined Roles
1. **`org_admin`**: Full workspace governance, tenant settings, member invitation, role assignment, portfolio creation, note publication, and audit log inspection.
2. **`portfolio_manager`**: Creates and manages portfolios, enters orders and transaction records, submits approval requests, creates research notes, and runs scenario stress testing. **Maker-Checker Guard**: Cannot approve own or others' trades.
3. **`compliance_officer`**: Independent supervisor. Reviews and approves approval requests, monitors concentration and liquidity limit breaches, reviews immutable audit trails, and exports compliance reports. Cannot enter or execute trades.
4. **`analyst`**: Conducts fundamental and technical research, drafts research notes, views risk analytics, and runs stress scenarios. Cannot approve trades, manage members, or export sensitive data.
5. **`viewer`**: Read-only oversight of published notes and high-level dashboard summaries.

### 2.2 Granular Permissions Matrix

| Permission Code | Meaning | `org_admin` | `portfolio_manager` | `compliance_officer` | `analyst` | `viewer` |
|---|---|:---:|:---:|:---:|:---:|:---:|
| `portfolio:read` | View portfolio holdings, NAV, and P&L | Yes | Yes | Yes | Yes | Yes |
| `portfolio:write` | Create or configure portfolios | Yes | Yes | No | No | No |
| `trade:record` | Enter transaction records | Yes | Yes | No | No | No |
| `trade:approve` | Approve / reject approval requests | Yes | No | Yes | No | No |
| `note:create` | Draft research notes | Yes | Yes | No | Yes | No |
| `note:publish` | Approve and publish notes to workspace | Yes | Yes | Yes | No | No |
| `risk:view` | Inspect liquidity & concentration risk | Yes | Yes | Yes | Yes | Yes |
| `scenario:run` | Execute multi-factor stress tests | Yes | Yes | Yes | Yes | No |
| `report:schedule` | Configure scheduled reporting | Yes | Yes | Yes | No | No |
| `report:export` | Export portfolio, risk, and CSV data | Yes | Yes | Yes | No | No |
| `audit:read` | Inspect immutable audit trails | Yes | Yes | Yes | No | No |
| `member:manage` | Add members and assign roles | Yes | No | No | No | No |

---

## 3. Multiple Portfolios & Research Notes

- **Multi-Fund Management**: An organization can own multiple segregated fund portfolios (e.g., Growth Fund, Income Fund, Liquidity Reserve). Each portfolio is scoped by `org_id` and tracked with independent cash balances, tax lots, and cost basis methods (`WAC` or `FIFO`).
- **Institutional Research Notes** (`research_notes`):
  - Linked to specific tickers, sectors, or investment theses.
  - Life cycle states: `draft` $\to$ `under_review` $\to$ `approved` $\to$ `archived`.
  - Visibility scopes: `internal_team`, `restricted`, `published`.

---

## 4. Institutional Risk Engines (`internal/institutional/risk.go`)

### 4.1 Liquidity Risk & Days-to-Liquidate (DTL)
Measures the time required to liquidate a holding without causing severe market impact:
$$\text{Daily Liquidation Capacity}_i = \text{Average Daily Volume (ADV)}_i \times \text{Participation Rate}$$
$$\text{Days to Liquidate (DTL)}_i = \frac{\text{Shares Held}_i}{\text{Daily Liquidation Capacity}_i}$$
*(Default participation rate: $15\%$ of ADV)*

#### Liquidity Tiers
- **Tier 1 (Liquid)**: $\text{DTL} \le 1.0$ day
- **Tier 2 (Moderate)**: $1.0 < \text{DTL} \le 3.0$ days
- **Tier 3 (Illiquid)**: $3.0 < \text{DTL} \le 10.0$ days
- **Tier 4 (Severely Illiquid)**: $\text{DTL} > 10.0$ days

#### Portfolio Weighted Days to Liquidate
$$\text{Weighted DTL} = \sum_{i=1}^N \left( \text{DTL}_i \times \frac{\text{Market Value}_i}{\text{Total NAV}} \right)$$

#### Slippage Cost Estimation
$$\text{Slippage Rate}_i = \min\left(0.15, \text{DTL}_i \times 0.50\%\right)$$
$$\text{Estimated Slippage}_i = \text{Market Value}_i \times \text{Slippage Rate}_i$$

---

### 4.2 Concentration Risk & Regulatory Limit Enforcement
- **Top-N Concentration**: Tracks Top 1, Top 3, Top 5, and Top 10 holdings as a percentage of Total NAV.
- **Herfindahl-Hirschman Index (HHI)**:
  $$\text{HHI} = \sum_{i=1}^N \left( w_i \times 100 \right)^2 \quad \text{where } w_i = \frac{\text{Market Value}_i}{\text{Total NAV}}$$
  - $\text{HHI} < 1500 \implies$ *Unconcentrated*
  - $1500 \le \text{HHI} \le 2500 \implies$ *Moderately Concentrated*
  - $\text{HHI} > 2500 \implies$ *Highly Concentrated*
- **Regulatory Limit Checks**:
  - **SEBON Single Stock Limit**: Default $10\%$ cap of NAV.
  - **Sector Concentration Limit**: Default $25\%$ cap of NAV.
  - Automatically flags `warning` or `critical` breaches, reporting excess percentage and excess NPR capital required to be reallocated.

---

### 4.3 Multi-Factor Scenario Stress Testing
Simulates prospective capital loss under macroeconomic and exchange shocks:
- **Market Crash Scenario**: NEPSE drops $-10\%$, $-20\%$, or $-30\%$, scaled per position by its historical Beta:
  $$\text{Effective Shock}_i = \text{Market Drop} \times \beta_i$$
- **Sector-Specific Shock Scenario**: Simulates industry-specific shocks (e.g. $-15\%$ banking shock, $-25\%$ hydropower tariff shock).
- **Liquidity Freeze Scenario**: Volume drops by $50\%$ and market bid-ask spreads widen, applying an additional $-2.50\%$ slippage penalty.
- **Interest Rate Hike Scenario**: NRB policy rate hike of $+150$ to $+200$ bps compressing equity multiples.
- **Custom Shocks**: User passes explicit per-symbol or per-factor shock percentage matrices.

---

## 5. Four-Eyes Principle & Anti-Self-Approval Workflow (`approval_requests`)

- Actions requiring independent review:
  - Large trades exceeding regulatory or firm risk thresholds (`large_trade`).
  - Portfolio rebalancing orders (`rebalance`).
  - Publishing public research notes (`note_publish`).
  - Material cash withdrawals (`cash_withdrawal`).
- **Anti-Self-Approval Enforcement**:
  - The store layer asserts $\text{requester\_id} \ne \text{reviewer\_id}$.
  - If a portfolio manager attempts to approve their own request, the system rejects it with `ErrSelfApprovalNotAllowed` (HTTP 403 Forbidden).

---

## 6. Scheduled Reports & Automated Snapshots (`scheduled_reports`)

- Recurring schedule execution (`daily`, `weekly`, `monthly`).
- Report templates:
  - `daily_pnl`: Daily P&L, closing NAV, turnover, and cash changes.
  - `weekly_risk`: DTL breakdown, illiquid NAV %, Top 5 concentration, and HHI.
  - `monthly_executive_summary`: TWR performance, benchmark comparison, alpha, beta, and Sharpe.
  - `compliance_audit`: Log of limit breaches, approval decisions, and maker-checker audit trails.

---

## 7. Immutable Audit Trail (`institutional_audit_logs`)

Every state mutation in an institutional workspace is recorded:
- `org_id`, `actor_id`, `action`, `resource_type`, `resource_id`, `detail`, `ip_address`, `created_at`.
- Immutable append-only design. Accessible by `compliance_officer` and `org_admin`.

---

## 8. Export Permission Gating (`PermReportExport`)

- Endpoints returning raw CSV, sensitive position data, or compliance records (`/export.csv`) verify that the calling member holds `PermReportExport`.
- Analysts and Viewers attempting export requests receive `HTTP 403 Forbidden: role lacks PermReportExport export permission`.

---

## 9. Institutional API Surface

| Method | Endpoint | Description | Permission Required |
|---|---|---|---|
| `GET` | `/api/v1/workspaces` | List user's institutional workspaces | Authenticated |
| `POST` | `/api/v1/workspaces` | Create institutional organization | Authenticated |
| `GET` | `/api/v1/workspaces/{orgId}/members` | List members and RBAC roles | `portfolio:read` |
| `POST` | `/api/v1/workspaces/{orgId}/members` | Add member or update role | `member:manage` (OrgAdmin) |
| `GET` | `/api/v1/workspaces/{orgId}/portfolios` | List organization portfolios | `portfolio:read` |
| `POST` | `/api/v1/workspaces/{orgId}/portfolios` | Create organization portfolio | `portfolio:write` |
| `GET` | `/api/v1/workspaces/{orgId}/notes` | List research notes | `portfolio:read` |
| `POST` | `/api/v1/workspaces/{orgId}/notes` | Create research note | `note:create` |
| `GET` | `/api/v1/workspaces/{orgId}/approvals` | List approval requests | `portfolio:read` |
| `POST` | `/api/v1/workspaces/{orgId}/approvals/{id}/review` | Review approval (maker-checker) | `trade:approve` (Anti-self-approval) |
| `GET` | `/api/v1/workspaces/{orgId}/portfolios/{portfolioId}/liquidity` | Liquidity risk (DTL, slippage) | `risk:view` |
| `GET` | `/api/v1/workspaces/{orgId}/portfolios/{portfolioId}/concentration` | Concentration risk (HHI, breaches) | `risk:view` |
| `POST` | `/api/v1/workspaces/{orgId}/portfolios/{portfolioId}/stress` | Multi-factor stress testing | `scenario:run` |
| `GET` | `/api/v1/workspaces/{orgId}/portfolios/{portfolioId}/export.csv` | Export portfolio & risk CSV | `report:export` |
| `GET` | `/api/v1/workspaces/{orgId}/audit` | Immutable audit trail | `audit:read` |
