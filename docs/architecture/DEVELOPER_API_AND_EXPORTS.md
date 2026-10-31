# Developer API, Licensing Matrix, and Bulk Exports

## 1. Overview

The NEPSE platform developer subsystem enables programmatic data access, enterprise system integration, and verifiable reporting via:
1. **Cryptographic API Keys**: Prefixed keys (`np_live_...`) with SHA-256 hash storage, IP whitelisting, and monthly quota enforcement.
2. **Platform Data Licensing Matrix**: Explicit provider rights (NEPSE, SEBON, NRB, Issuers) and redistribution policies (`prohibited`, `internal_only`, `permitted_with_attribution`, `licensed`).
3. **Multi-Format Exports**: Automated streaming generation of tabular datasets in CSV (with embedded license comments), JSON (structured metadata envelope), and Parquet format (`PAR1` binary standard).
4. **Attribution & Provenance Headers**: Every exported asset and API response includes mandatory licensing classification and source provenance (`X-Licensing-Class`, `X-Attribution-Notice`).

---

## 2. API Key Architecture & Tiers

### Key Generation & Storage
- Keys are issued with format `np_live_<48 hex chars>`.
- The plaintext key is shown **once** upon generation and is never stored.
- Only the SHA-256 hash `sha256(key)` is persisted in PostgreSQL table `developer_keys`.
- Lookups match `WHERE key_hash = $1 AND revoked_at IS NULL`.

### Tier Quotas & Rate Limits

| Tier | RPM (Burst) | Monthly Quota | Max Export Rows | Permitted Datasets |
| :--- | :--- | :--- | :--- | :--- |
| **Free** | 60 | 10,000 | 1,000 | Companies, Disclosures |
| **Pro** | 300 | 100,000 | 50,000 | Free + Financials, OHLCV (internal) |
| **Institutional** | 1,200 | 1,000,000 | 500,000 | Pro + Risk Analytics, Portfolios |
| **Enterprise** | 3,000 | 10,000,000 | 2,000,000 | Institutional + Custom Feeds |

### Quota Enforcement & Auto-Reset
- Every API key records `requests_this_month` and `quota_reset_at`.
- If `now > quota_reset_at`, the window automatically rolls forward by 1 month and resets usage count.
- When `requests_this_month >= monthly_quota`, requests are rejected with `HTTP 429 Too Many Requests` (`ErrQuotaExceeded`) and a `Retry-After` header.

---

## 3. Platform Data Licensing Matrix

NEPSE market data, regulatory circulars, and financial reports carry differing legal and regulatory distribution rights. The platform formalizes these in `developer.PlatformLicensingMatrix`:

```
+----------------------------------------------------------------------------------------------------+
|                                    PLATFORM DATA LICENSING MATRIX                                  |
+-------------------+--------------------+--------------------+---------------------------+----------+
| Dataset           | Provider           | License Class      | Redistribution Policy     | Commer.  |
+-------------------+--------------------+--------------------+---------------------------+----------+
| market_live       | NEPSE              | public_unverified  | prohibited                | No       |
| daily_ohlcv       | NEPSE              | public_unverified  | internal_only             | No       |
| floorsheet        | NEPSE              | public_unverified  | prohibited                | No       |
| financials        | ISSUER / NEPSE     | official_verified  | permitted_with_attribution| Yes      |
| disclosures       | NEPSE / SEBON / NRB| official_verified  | permitted_with_attribution| Yes      |
| analytics         | PLATFORM_DERIVED   | derived            | internal_only             | Yes      |
| companies         | NEPSE              | official_verified  | permitted_with_attribution| Yes      |
+-------------------+--------------------+--------------------+---------------------------+----------+
```

### Provider Restrictions
1. **NEPSE Real-time & Floorsheet Feeds**:
   - Class: `public_unverified`.
   - Policy: `prohibited`.
   - Bulk export or external redistribution is blocked by code (`developer.CheckRedistributionPermission`).
2. **Official Disclosures & Financial Filings**:
   - Class: `official_verified`.
   - Policy: `permitted_with_attribution`.
   - Permitted with mandatory notice referencing the regulatory source (e.g. "Source: NEPSE / SEBON / NRB").

---

## 4. Multi-Format Export Formats

Exports can be initiated via `POST /api/v1/exports` and downloaded via `GET /api/v1/exports/{id}/download`.

### 1. CSV Format (`text/csv`)
Standard RFC 4180 CSV with licensing metadata and legal disclaimers prefixed with `#` comments:
```csv
# NEPSE Platform Data Export
# Dataset: companies
# License Class: official_verified
# Mandatory Notice: Company registry and security reference listing.
# Exported At: 2026-10-30T10:50:00Z
symbol,name,sector,status,instrument
NABIL,Nabil Bank Limited,Commercial Banks,active,equity
SCB,Standard Chartered Bank Nepal,Commercial Banks,active,equity
```

### 2. JSON Format (`application/json`)
Self-contained envelope including metadata, dataset provenance, column headers, and structured records:
```json
{
  "metadata": {
    "dataset": "companies",
    "license_class": "official_verified",
    "attribution_notice": "Company registry and security reference listing.",
    "exported_at": "2026-10-30T10:50:00Z",
    "row_count": 2,
    "sha256": "8f3a..."
  },
  "columns": ["symbol", "name", "sector", "status", "instrument"],
  "rows": [
    {"symbol": "NABIL", "name": "Nabil Bank Limited", "sector": "Commercial Banks", "status": "active", "instrument": "equity"},
    {"symbol": "SCB", "name": "Standard Chartered Bank Nepal", "sector": "Commercial Banks", "status": "active", "instrument": "equity"}
  ]
}
```

### 3. Parquet Format (`application/octet-stream`)
High-performance analytical storage using Apache Parquet format. Conforms to the `PAR1` magic header and footer standard with Thrift metadata block and Snappy compression metadata for direct query execution in DuckDB, ClickHouse, Apache Spark, or Python Pandas:
```
+-------------------------------------------------------+
|  PAR1 (4 bytes Magic Header)                          |
|  [Schema & Column Metadata Block]                     |
|  [Snappy Compressed Row Groups]                       |
|  [FileMetaData Thrift Length (4 bytes)]               |
|  PAR1 (4 bytes Magic Footer)                          |
+-------------------------------------------------------+
```

---

## 5. API Endpoints Reference

### Public Endpoints
- `GET /api/v1/developer/licensing-matrix`
  - Returns current provider licensing rights, redistribution policies, and attribution notices.

### Authenticated Endpoints (Bearer Token or `X-API-Key`)
- `GET /api/v1/developer/keys`
  - Lists all active and revoked API keys for the calling user/organization.
- `POST /api/v1/developer/keys`
  - Body: `{"name": "production-sync", "tier": "institutional"}`
  - Generates a new cryptographic key and returns `api_secret` (only visible once).
- `DELETE /api/v1/developer/keys/{id}`
  - Immediately revokes the specified API key.
- `POST /api/v1/exports`
  - Body: `{"dataset": "companies", "format": "csv"}`
  - Submits an export job, checks entitlement quotas, validates licensing permissions, and generates data.
- `GET /api/v1/exports`
  - Lists user export jobs with pagination (`limit`, `offset`).
- `GET /api/v1/exports/{id}`
  - Returns export status (`pending`, `completed`, `failed`), row count, file size, and SHA-256 digest.
- `GET /api/v1/exports/{id}/download`
  - Streams the formatted export file with headers:
    - `X-Licensing-Class: official_verified`
    - `X-Attribution-Notice: Company registry and security reference listing.`
