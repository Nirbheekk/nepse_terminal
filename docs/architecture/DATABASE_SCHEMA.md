# DATABASE SCHEMA

**Phase 0 deliverable; implemented as `migrations/0001_init.sql` in Phase 1.**

PostgreSQL 16, pgx. Conventions below are mandatory for every table.

---

## 0. Conventions

| Rule | Detail |
|---|---|
| Keys | `uuid` PKs (`gen_random_uuid()`, built into PG13+) for entities; natural composite PKs for time-series |
| Money / quantity | `NUMERIC` only. `rate NUMERIC(18,6)`, `quantity NUMERIC(20,4)`, `amount NUMERIC(20,4)`, `shares NUMERIC(24,4)` |
| Never | `float4/float8/double precision` for prices, quantities, capitalisation, P&L |
| Time | `TIMESTAMPTZ` for events; `DATE` for trading dates; all timestamps UTC, source zone retained in `raw` |
| JSON | `JSONB` for payloads, `GENERATED ALWAYS AS … STORED` for extracted keys if needed |
| Idempotency | unique natural keys + `ON CONFLICT DO NOTHING/UPDATE` |
| Provenance | market tables carry `data_source_id`, `ingestion_job_id`, `raw_sha256`, `ingested_at` |
| Deletes | market data is append-only; corrections are new rows flagged `is_current` |
| Migrations | `migrations/NNNN_name.up.sql` / `.down.sql`, applied by the runner, never edited after merge |

```sql
CREATE TABLE data_sources (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug          text NOT NULL UNIQUE,          -- 'nepse_unofficial', 'sebon', 'licensed_vendor_x'
  display_name  text NOT NULL,
  endpoint      text,
  auth_kind     text NOT NULL DEFAULT 'none',  -- none|api_key|oauth|contract
  rate_limit    jsonb NOT NULL DEFAULT '{}',   -- {"req_per_min": 60}
  license_class text NOT NULL,                 -- enum below
  redistribution text NOT NULL DEFAULT 'prohibited',
  terms_url     text,
  verified_at   timestamptz,
  notes         text
);

CREATE TYPE license_class AS ENUM (
  'official_verified','third_party_licensed','public_unverified',
  'derived','restricted','unavailable'
);

CREATE TYPE freshness_state AS ENUM ('live','delayed','stale','partial','closed','unknown');
```

---

## 1. Market core

```sql
CREATE TABLE sectors (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code       text NOT NULL UNIQUE,
  name       text NOT NULL,
  index_symbol text
);

CREATE TABLE companies (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  symbol       text NOT NULL UNIQUE,
  name         text NOT NULL,
  sector_id    uuid REFERENCES sectors(id),
  status       text NOT NULL DEFAULT 'active',   -- active|suspended|delisted
  instrument   text,                             -- equity|debenture|mutual_fund|promoter
  description  text,
  website      text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX companies_sector_idx ON companies (sector_id) WHERE status = 'active';

CREATE TABLE securities (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id    uuid NOT NULL REFERENCES companies(id),
  symbol        text NOT NULL UNIQUE,
  security_type text NOT NULL,                    -- ordinary|promoter|debenture|mutual_fund
  listed_shares NUMERIC(24,4),
  public_shares NUMERIC(24,4),
  listing_date  date,
  delisted_at   date
);
CREATE INDEX securities_company_idx ON securities (company_id);

-- symbol renames and status transitions must be reconstructable point-in-time
CREATE TABLE security_status_history (
  id          bigserial PRIMARY KEY,
  security_id uuid NOT NULL REFERENCES securities(id),
  valid_from  date NOT NULL,
  valid_to    date,
  status      text NOT NULL,
  symbol      text NOT NULL,
  reason      text,
  UNIQUE (security_id, valid_from)
);

CREATE TABLE trading_sessions (
  trading_date   date PRIMARY KEY,
  opened_at      timestamptz,
  closed_at      timestamptz,
  is_trading_day boolean NOT NULL DEFAULT true,
  note           text
);

CREATE TABLE market_summaries (
  trading_date     date PRIMARY KEY REFERENCES trading_sessions(trading_date),
  nepse_index      NUMERIC(18,4),
  point_change     NUMERIC(18,4),
  percent_change   NUMERIC(10,4),
  turnover         NUMERIC(24,4),
  transactions     bigint,
  traded_shares    NUMERIC(24,4),
  traded_securities int,
  advanced         int,
  declined         int,
  unchanged        int,
  -- computed, never scraped: see MARKET_CAP_AUDIT.md
  market_cap       NUMERIC(28,4),
  float_market_cap NUMERIC(28,4),
  cap_method       text,          -- 'sum(listed_shares*close)@v1'
  cap_covered      int,           -- companies contributing
  cap_eligible     int,           -- listed companies considered
  -- provenance
  data_source_id   uuid NOT NULL REFERENCES data_sources(id),
  ingestion_job_id uuid,
  raw_sha256       bytea,
  ingested_at      timestamptz NOT NULL DEFAULT now(),
  close_price      NUMERIC(18,4)
);

CREATE TABLE daily_ohlcv (
  security_id    uuid NOT NULL REFERENCES securities(id),
  trading_date   date NOT NULL REFERENCES trading_sessions(trading_date),
  open           NUMERIC(18,4),
  high           NUMERIC(18,4),
  low            NUMERIC(18,4),
  close          NUMERIC(18,4),
  previous_close NUMERIC(18,4),
  volume         NUMERIC(24,4),
  turnover       NUMERIC(24,4),
  transactions   bigint,
  market_cap     NUMERIC(28,4),          -- exchange-reported where available
  source_close   NUMERIC(18,4),
  data_source_id uuid NOT NULL REFERENCES data_sources(id),
  ingestion_job_id uuid,
  ingested_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (security_id, trading_date),
  CONSTRAINT ohlc_positive CHECK (close IS NULL OR close > 0),
  CONSTRAINT ohlc_band CHECK (low IS NULL OR high IS NULL OR low <= high)
);
CREATE INDEX daily_ohlcv_date_idx ON daily_ohlcv (trading_date);

-- intraday exists only as observed snapshots (session-only, price-only)
CREATE TABLE intraday_points (
  security_id  uuid NOT NULL REFERENCES securities(id),
  trading_date date NOT NULL,
  observed_at  timestamptz NOT NULL,
  price        NUMERIC(18,4) NOT NULL,
  volume       NUMERIC(24,4),           -- NULL: upstream does not provide it
  source_kind  text NOT NULL DEFAULT 'session_snapshot',
  PRIMARY KEY (security_id, observed_at)
);
```

### Index tables

```sql
CREATE TABLE index_definitions (
  id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code     text NOT NULL UNIQUE,        -- 'NEPSE','SENSITIVE','FLOAT','SENSITIVE_FLOAT','BANKING',…
  name     text NOT NULL,
  kind     text NOT NULL,               -- headline|float|sector
  base_value NUMERIC(18,4),
  sector_id uuid REFERENCES sectors(id)
);

CREATE TABLE index_observations (
  index_id      uuid NOT NULL REFERENCES index_definitions(id),
  trading_date  date NOT NULL,
  observed_at   timestamptz,            -- set for intraday points, NULL for daily close
  value         NUMERIC(18,4) NOT NULL,
  point_change  NUMERIC(18,4),
  percent_change NUMERIC(10,4),
  granularity   text NOT NULL DEFAULT 'daily',  -- daily|intraday
  data_source_id uuid NOT NULL REFERENCES data_sources(id),
  ingestion_job_id uuid,
  PRIMARY KEY (index_id, trading_date, granularity, observed_at)
);
CREATE INDEX index_obs_daily_idx ON index_observations (index_id, trading_date)
  WHERE granularity = 'daily';
```

---

## 2. Floorsheet (authoritative, highest volume)

```sql
CREATE TABLE floorsheet_trades (
  contract_id   NUMERIC(30,0) PRIMARY KEY,       -- numeric, not text: sorting and dedupe
  trading_date  date NOT NULL REFERENCES trading_sessions(trading_date),
  security_id   uuid REFERENCES securities(id),  -- nullable: symbol may be unknown yet
  symbol        text NOT NULL,                   -- always preserved as received
  buyer_broker  text,
  seller_broker text,
  quantity      NUMERIC(20,4) NOT NULL CHECK (quantity > 0),
  rate          NUMERIC(18,6) NOT NULL CHECK (rate > 0),
  amount        NUMERIC(24,4) NOT NULL,
  executed_at   timestamptz,                     -- NULL when source omits it; never guessed
  data_source_id uuid NOT NULL REFERENCES data_sources(id),
  ingestion_job_id uuid NOT NULL,
  ingested_at   timestamptz NOT NULL DEFAULT now(),
  raw           jsonb NOT NULL
);
CREATE INDEX floorsheet_date_symbol_idx ON floorsheet_trades (trading_date, symbol);
CREATE INDEX floorsheet_buyer_idx  ON floorsheet_trades (buyer_broker, trading_date);
CREATE INDEX floorsheet_seller_idx ON floorsheet_trades (seller_broker, trading_date);
CREATE INDEX floorsheet_amount_idx ON floorsheet_trades (trading_date, amount DESC);
```

**Idempotency rule:** insertion is `ON CONFLICT (contract_id) DO NOTHING`. Sum of
`amount` per `trading_date` must equal the exchange's reported turnover for that
date — asserted by the reconciliation job, recorded in `data_quality_issues` on
mismatch.

Partitioning is intentionally deferred: at ~1M trades/year per the observed
~20k/session rate, a single table with these indexes is comfortable. Revisit at
~50M rows.

```sql
CREATE TABLE market_depth_snapshots (
  id            bigserial PRIMARY KEY,
  security_id   uuid NOT NULL REFERENCES securities(id),
  observed_at   timestamptz NOT NULL,
  side          text NOT NULL CHECK (side IN ('bid','ask')),
  rank          int  NOT NULL,
  price         NUMERIC(18,4),
  quantity      NUMERIC(20,4),
  orders        int
);
CREATE INDEX depth_latest_idx ON market_depth_snapshots (security_id, observed_at DESC);
```

---

## 3. Fundamentals and corporate actions

```sql
CREATE TABLE financial_periods (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id   uuid NOT NULL REFERENCES companies(id),
  fiscal_year  int NOT NULL,
  quarter      int,                 -- NULL = annual
  period_end   date NOT NULL,
  published_at date,
  basis        text NOT NULL DEFAULT 'reported',  -- reported|restated|derived
  source_doc   text,
  UNIQUE (company_id, fiscal_year, quarter, basis)
);

CREATE TABLE financial_metrics (
  period_id    uuid NOT NULL REFERENCES financial_periods(id),
  metric       text NOT NULL,      -- eps, pe, pb, roe, net_profit, revenue, net_worth …
  value        NUMERIC(24,6),
  unit         text,
  formula      text,               -- documented, reproducible
  PRIMARY KEY (period_id, metric)
);

CREATE TABLE corporate_actions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  company_id   uuid NOT NULL REFERENCES companies(id),
  action_type  text NOT NULL,      -- dividend|bonus|rights|agm|split|book_closure
  announced_on date,
  record_date  date,
  ex_date      date,
  ratio        NUMERIC(18,6),
  cash_amount  NUMERIC(18,6),
  details      jsonb NOT NULL DEFAULT '{}',
  source_url   text,
  UNIQUE (company_id, action_type, announced_on, record_date)
);

CREATE TABLE news_articles (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id    uuid NOT NULL REFERENCES data_sources(id),
  title        text NOT NULL,
  summary      text,               -- short extract only; never full copyrighted text
  url          text NOT NULL UNIQUE,
  published_at timestamptz NOT NULL,
  category     text,
  symbols      text[] NOT NULL DEFAULT '{}',
  body_stored  boolean NOT NULL DEFAULT false
);
CREATE INDEX news_published_idx ON news_articles (published_at DESC);
CREATE INDEX news_symbols_idx ON news_articles USING GIN (symbols);
```

---

## 4. Identity, access, tenancy

```sql
CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email         citext NOT NULL UNIQUE,
  password_hash text,                        -- argon2id; NULL = OAuth-only
  display_name  text,
  email_verified_at timestamptz,
  mfa_secret    text,
  status        text NOT NULL DEFAULT 'active',
  failed_logins int NOT NULL DEFAULT 0,
  locked_until  timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  last_login_at timestamptz
);

CREATE TABLE roles (
  id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text NOT NULL UNIQUE,   -- guest,user,pro,analyst,institutional,org_admin,admin,super_admin
  name text NOT NULL
);
CREATE TABLE permissions (code text PRIMARY KEY, description text);
CREATE TABLE role_permissions (
  role_id uuid REFERENCES roles(id) ON DELETE CASCADE,
  permission text REFERENCES permissions(code) ON DELETE CASCADE,
  PRIMARY KEY (role_id, permission)
);
CREATE TABLE user_roles (
  user_id uuid REFERENCES users(id) ON DELETE CASCADE,
  role_id uuid REFERENCES roles(id) ON DELETE CASCADE,
  granted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role_id)
);

CREATE TABLE sessions (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  refresh_hash  bytea NOT NULL,        -- SHA-256 of the refresh token, never the token
  family_id     uuid NOT NULL,         -- rotation family; reuse ⇒ revoke family
  user_agent    text,
  ip            inet,
  created_at    timestamptz NOT NULL DEFAULT now(),
  last_used_at  timestamptz,
  expires_at    timestamptz NOT NULL,
  revoked_at    timestamptz,
  rotated_from  uuid REFERENCES sessions(id)
);
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE organizations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  slug text NOT NULL UNIQUE,
  plan text NOT NULL DEFAULT 'trial',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE organization_members (
  organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
  user_id uuid REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL DEFAULT 'member',   -- member|analyst|org_admin
  joined_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, user_id)
);

CREATE TABLE entitlements (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subject_type   text NOT NULL,      -- user|org|role
  subject_id     uuid NOT NULL,
  license_class  license_class NOT NULL,
  dataset        text NOT NULL,      -- 'floorsheet','exports','ai_research'
  granted_at     timestamptz NOT NULL DEFAULT now(),
  expires_at     timestamptz,
  UNIQUE (subject_type, subject_id, license_class, dataset)
);
```

---

## 5. User-owned analytical data

```sql
CREATE TABLE watchlists (
  id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id  uuid REFERENCES organizations(id) ON DELETE CASCADE,
  name    text NOT NULL,
  sort_order int NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE watchlist_items (
  watchlist_id uuid REFERENCES watchlists(id) ON DELETE CASCADE,
  security_id  uuid REFERENCES securities(id) ON DELETE CASCADE,
  position     int NOT NULL DEFAULT 0,
  note         text,
  added_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (watchlist_id, security_id)
);

CREATE TABLE alert_rules (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  security_id  uuid REFERENCES securities(id) ON DELETE CASCADE,
  kind         text NOT NULL,   -- price_above|pct_change|volume_spike|rsi_cross|…
  params       jsonb NOT NULL,
  cooldown_sec int NOT NULL DEFAULT 300,
  enabled      boolean NOT NULL DEFAULT true,
  last_fired_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE alert_events (
  id          bigserial PRIMARY KEY,
  rule_id     uuid NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
  fired_at    timestamptz NOT NULL DEFAULT now(),
  observed    jsonb NOT NULL,
  dedupe_key  text NOT NULL,
  delivered   boolean NOT NULL DEFAULT false,
  UNIQUE (rule_id, dedupe_key)          -- idempotent delivery
);

CREATE TABLE portfolios (
  id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id  uuid REFERENCES organizations(id) ON DELETE CASCADE,
  name    text NOT NULL,
  currency char(3) NOT NULL DEFAULT 'NPR',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE portfolio_transactions (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  portfolio_id  uuid NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
  security_id   uuid REFERENCES securities(id),
  txn_type      text NOT NULL,    -- buy|sell|dividend|fee|tax|deposit|withdrawal
  traded_on     date NOT NULL,
  quantity      NUMERIC(20,4) NOT NULL DEFAULT 0,
  price         NUMERIC(18,6) NOT NULL DEFAULT 0,
  fees          NUMERIC(18,4) NOT NULL DEFAULT 0,
  taxes         NUMERIC(18,4) NOT NULL DEFAULT 0,
  note          text,
  import_batch  uuid,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ptx_portfolio_idx ON portfolio_transactions (portfolio_id, traded_on);

CREATE TABLE portfolio_valuations (
  portfolio_id  uuid REFERENCES portfolios(id) ON DELETE CASCADE,
  valuation_date date NOT NULL,
  market_value  NUMERIC(24,4) NOT NULL,
  cash          NUMERIC(24,4) NOT NULL DEFAULT 0,
  cost_basis    NUMERIC(24,4),
  twr           NUMERIC(14,8),
  PRIMARY KEY (portfolio_id, valuation_date)
);

CREATE TABLE research_notes (
  id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id  uuid REFERENCES organizations(id) ON DELETE CASCADE,
  security_id uuid REFERENCES securities(id),
  title   text NOT NULL,
  body    text NOT NULL,
  visibility text NOT NULL DEFAULT 'private',  -- private|org|public
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE chart_layouts (
  id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name    text NOT NULL,
  symbol  text,
  payload jsonb NOT NULL,
  is_default boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL DEFAULT now()
);
```

---

## 6. Operations: jobs, quality, audit

```sql
CREATE TYPE job_state AS ENUM ('queued','running','succeeded','failed','dead','cancelled');

CREATE TABLE ingestion_jobs (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  kind         text NOT NULL,       -- market_summary|daily_ohlcv|floorsheet_live|backfill|…
  source_id    uuid REFERENCES data_sources(id),
  state        job_state NOT NULL DEFAULT 'queued',
  scheduled_for timestamptz,
  started_at   timestamptz,
  finished_at  timestamptz,
  attempt      int NOT NULL DEFAULT 0,
  max_attempts int NOT NULL DEFAULT 5,
  next_retry_at timestamptz,
  rows_in      int,
  rows_upserted int,
  error_class  text,
  error        text,
  trace_id     text,
  params       jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX jobs_state_idx ON ingestion_jobs (state, next_retry_at);

CREATE TABLE ingestion_checkpoints (
  job_kind    text NOT NULL,
  cursor_key  text NOT NULL,     -- e.g. trading date, page offset
  cursor_value text NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (job_kind, cursor_key)
);

CREATE TABLE raw_payloads (
  sha256      bytea PRIMARY KEY,
  source_id   uuid NOT NULL REFERENCES data_sources(id),
  endpoint    text NOT NULL,
  fetched_at  timestamptz NOT NULL,
  content_type text,
  bytes       int NOT NULL,
  body        bytea,             -- NULL when spilled to object storage
  storage_key text
);

CREATE TABLE data_quality_issues (
  id           bigserial PRIMARY KEY,
  detected_at  timestamptz NOT NULL DEFAULT now(),
  trading_date date,
  dataset      text NOT NULL,
  severity     text NOT NULL,    -- info|warn|error|critical
  code         text NOT NULL,    -- turnover_mismatch|duplicate_contract|ohlc_band|missing_date
  subject      text,
  observed     jsonb NOT NULL,
  expected     jsonb,
  resolved_at  timestamptz,
  resolution   text
);
CREATE INDEX dq_open_idx ON data_quality_issues (severity, detected_at DESC)
  WHERE resolved_at IS NULL;

CREATE TABLE audit_logs (
  id         bigserial PRIMARY KEY,
  at         timestamptz NOT NULL DEFAULT now(),
  actor_id   uuid REFERENCES users(id),
  actor_ip   inet,
  action     text NOT NULL,      -- login|export|admin_job|portfolio_read|entitlement_change
  target     text,
  outcome    text NOT NULL,
  trace_id   text,
  detail     jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX audit_actor_idx ON audit_logs (actor_id, at DESC);
```

---

## 7. Catalogue summary

| Table | Rows today (legacy) | Rows after Phase 3 |
|---|---|---|
| `companies` / `securities` | 650 / 650 (`company_list.json`) | 650 |
| `sectors` | 13 (`sub_indices.json`) | 13 |
| `daily_ohlcv` | **0** (legacy `daily_prices` empty) | **≈78,000** (~230 dates × ~340) |
| `market_summaries` | 0 (JSON only) | ≈230 |
| `index_observations` | 0 (JSON only) | ≈230 daily + session intraday |
| `floorsheet_trades` | 28,000+ and growing | same + forward history |
| `market_depth_snapshots` | 0 (JSON only, 387 bid / 396 ask) | rolling snapshots |
| `financial_metrics` | 0 (JSON only, 22 reports/company) | ≈650 × 22 |

The three zeros are the reason the migration is worth doing: the legacy system
observes the present but keeps almost no history.
