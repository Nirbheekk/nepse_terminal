# Disclosures Integration & Evidence-Grounded AI Research Assistant

This document specifies the integration of authorized regulatory and corporate disclosures (NEPSE, SEBON, NRB, listed companies) and the architecture of the evidence-grounded AI research assistant.

---

## 1. Authorized Disclosures Integration (`internal/disclosures`)

### 1.1 Four Authorized Regulatory & Corporate Authorities
- **NEPSE (Nepal Stock Exchange)**: Trading circulars, listing notices, market suspensions/resumptions, circuit breaker notices, AGM announcements, auction notices.
- **SEBON (Securities Board of Nepal)**: Securities regulations, board circulars, IPO/FPO/Right share approvals, broker licensing, market enforcement actions.
- **NRB (Nepal Rastra Bank)**: Central bank monetary policies, unified directives (e.g., prudential norms, single-borrower limits, CRR/SLR, share pledge loan caps).
- **COMPANY (Listed Issuers)**: Quarterly and annual financial reports, BOD dividend recommendations, AGM notices, management transitions, credit rating updates.

### 1.2 Date Normalization & Provenance
- Explicit timezone handling:
  - Source timestamps recorded in Nepal Local Time (NPT UTC+05:45).
  - Normalization to UTC for database storage, preserving `published_at_npt` string for human presentation.
- Effective date tracking: records when a regulatory circular or corporate action becomes operational.

### 1.3 Attachments & Integrity Verification
Every official document attachment carries:
- `file_name` & `file_url`
- `mime_type` (e.g. `application/pdf`)
- `file_size_bytes`
- `sha256`: Hexadecimal SHA-256 digest of document bytes ensuring tamper-evidence and reproducibility.

### 1.4 Licensing Metadata & Provenance
Every disclosure explicitly declares:
- `license_class`: `official_verified`, `third_party_licensed`, `public_unverified`, `derived`, `restricted`.
- `redistribution`: `permitted_with_attribution`, `internal_only`, `prohibited`, `licensed`.
- `is_verified`: Boolean validation flag.

---

## 2. Evidence-Grounded AI Research Assistant (`internal/research`)

### 2.1 Multi-Index Evidence Retrieval
Queries are grounded against:
1. Authorized disclosures and circulars (NEPSE, SEBON, NRB, Issuers).
2. Normalized financial statements (Balance Sheet, Income Statement, Cash Flow).
3. Reporting-period-aware financial ratios (P/E, P/B, ROE, margins).
4. Corporate actions (Dividends, Bonus shares, Rights issues, splits).
5. Market prices (Daily OHLCV, market cap, turnover).
6. Private portfolio ledger (ONLY when query is user-authenticated and portfolio-scoped).

### 2.2 Tripartite Epistemic Grounding
Findings returned by the research assistant are segregated into three distinct categories:

| Category | Definition | Audit Requirement |
|---|---|---|
| **`FACT`** | Directly sourced assertion from an authoritative disclosure or statement record. | Citation with source ID, authority, publication date, reference number, attachment URL, and SHA-256 digest. |
| **`CALCULATION`** | Deterministic numeric metric derived via tested mathematical formula. | Citation specifying the formula used, inputs, valuation date, and financial reporting period. |
| **`INTERPRETATION`** | AI analytical reasoning, evaluation, or contextual inference. | Explicitly labeled as AI reasoning drawn strictly from the cited facts and calculations. |

### 2.3 Strict Guardrails

#### 1. Unsupported Claims & Hallucination Prevention
- If no authorized disclosure or financial record is retrieved for a query, the assistant issues an explicit refusal:
  > *"No authorized NEPSE, SEBON, NRB or company disclosures found in platform records matching '[query]'. Unsupported claims are refused."*
- Generates an evidence confidence score: $\frac{\text{Citations Found}}{\text{Required Claims}}$ (returns $0.00$ on empty evidence).

#### 2. Unauthorized Portfolio Disclosure Prevention
- **Strict Tenant Boundary**:
  - Public market, company, or regulatory research queries NEVER have access to private portfolio data.
  - Portfolio-scoped queries (`scope = "portfolio"`) require authenticated JWT credentials matching the portfolio's `user_id`.
  - Cross-tenant queries are blocked with `ErrUnauthorizedPortfolioAccess` (HTTP 403 Forbidden).
  - Private user IDs, cash balances, and tax lots are redacted from cross-tenant context.

---

## 3. Database Schema (`migrations/0012_disclosures_research.up.sql`)

- `disclosures`: Core table storing authority, symbol, category, title, summary, body, published_at, effective_date, source_url, reference_number, license_class, and redistribution rights.
- `disclosure_attachments`: Stores file_name, file_url, file_size_bytes, mime_type, sha256 checksum, and storage_key.
- `research_queries`: Audit log of research questions, user ID, scope, summary answer, and confidence score.
- `research_citations`: Maps each individual fact or calculation to its source record ID, authority, as-of date, and attachment URL.

---

## 4. API Endpoints

| Method | Endpoint | Description | Access |
|---|---|---|---|
| `GET` | `/api/v1/disclosures` | List authorized disclosures (filterable by authority, symbol, category) | Public |
| `GET` | `/api/v1/disclosures/{id}` | Single disclosure with attachments and licensing metadata | Public |
| `POST` | `/api/v1/disclosures` | Ingest authorized disclosure with attachments | Authenticated / Collector |
| `POST` | `/api/v1/research/query` | Evidence-grounded research query with tripartite grounding | Public / User-scoped for portfolio |
