-- 0001_init: core market + identity schema.
-- Money is NUMERIC (never float). Events are TIMESTAMPTZ. Trading dates are DATE.

CREATE TABLE data_sources (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug           text NOT NULL UNIQUE,
    display_name   text NOT NULL,
    endpoint       text,
    license_class  text NOT NULL DEFAULT 'public_unverified',
    redistribution text NOT NULL DEFAULT 'prohibited',
    terms_url      text,
    verified_at    timestamptz,
    notes          text
);

CREATE TABLE sectors (
    id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code   text NOT NULL UNIQUE,
    name   text NOT NULL
);

CREATE TABLE companies (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    symbol     text NOT NULL UNIQUE,
    name       text NOT NULL,
    sector_id  uuid REFERENCES sectors(id),
    status     text NOT NULL DEFAULT 'active',
    instrument text NOT NULL DEFAULT 'equity',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX companies_sector_idx ON companies (sector_id) WHERE status = 'active';

CREATE TABLE securities (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    uuid NOT NULL REFERENCES companies(id),
    symbol        text NOT NULL UNIQUE,
    security_type text NOT NULL DEFAULT 'ordinary',
    listed_shares NUMERIC(24,4),
    public_shares NUMERIC(24,4)
);
CREATE INDEX securities_company_idx ON securities (company_id);

CREATE TABLE trading_sessions (
    trading_date date PRIMARY KEY,
    opened_at    timestamptz,
    closed_at    timestamptz,
    is_trading_day boolean NOT NULL DEFAULT true
);

CREATE TABLE market_summaries (
    trading_date      date PRIMARY KEY REFERENCES trading_sessions(trading_date),
    nepse_index       NUMERIC(18,4),
    point_change      NUMERIC(18,4),
    percent_change    NUMERIC(10,4),
    turnover          NUMERIC(24,4),
    transactions      bigint,
    traded_shares     NUMERIC(24,4),
    traded_securities integer,
    advanced          integer,
    declined          integer,
    unchanged         integer,
    market_cap        NUMERIC(28,4),
    float_market_cap  NUMERIC(28,4),
    cap_covered       integer,
    cap_eligible      integer,
    cap_method        text,
    data_source_id    uuid REFERENCES data_sources(id),
    ingested_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE daily_ohlcv (
    security_id      uuid NOT NULL REFERENCES securities(id),
    trading_date     date NOT NULL REFERENCES trading_sessions(trading_date),
    open             NUMERIC(18,4),
    high             NUMERIC(18,4),
    low              NUMERIC(18,4),
    close            NUMERIC(18,4),
    previous_close   NUMERIC(18,4),
    volume           NUMERIC(24,4),
    turnover         NUMERIC(24,4),
    transactions     bigint,
    data_source_id   uuid REFERENCES data_sources(id),
    ingested_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (security_id, trading_date),
    CONSTRAINT ohlc_band CHECK (low IS NULL OR high IS NULL OR low <= high)
);
CREATE INDEX daily_ohlcv_date_idx ON daily_ohlcv (trading_date);

CREATE TABLE index_definitions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code       text NOT NULL UNIQUE,
    name       text NOT NULL,
    kind       text NOT NULL DEFAULT 'headline'
);

CREATE TABLE index_observations (
    index_id       uuid NOT NULL REFERENCES index_definitions(id),
    trading_date   date NOT NULL,
    granularity    text NOT NULL DEFAULT 'daily',
    observed_at    timestamptz,
    value          NUMERIC(18,4) NOT NULL,
    point_change   NUMERIC(18,4),
    percent_change NUMERIC(10,4),
    PRIMARY KEY (index_id, trading_date, granularity, observed_at)
);
CREATE INDEX index_obs_daily_idx ON index_observations (index_id, trading_date)
    WHERE granularity = 'daily';

CREATE TABLE floorsheet_trades (
    contract_id      NUMERIC(30,0) PRIMARY KEY,
    trading_date     date NOT NULL REFERENCES trading_sessions(trading_date),
    symbol           text NOT NULL,
    security_id      uuid REFERENCES securities(id),
    buyer_broker     text,
    seller_broker    text,
    quantity         NUMERIC(20,4) NOT NULL CHECK (quantity > 0),
    rate             NUMERIC(18,6) NOT NULL CHECK (rate > 0),
    amount           NUMERIC(24,4) NOT NULL,
    executed_at      timestamptz,
    data_source_id   uuid REFERENCES data_sources(id),
    ingested_at      timestamptz NOT NULL DEFAULT now(),
    raw              jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX floorsheet_date_symbol_idx ON floorsheet_trades (trading_date, symbol);
CREATE INDEX floorsheet_amount_idx ON floorsheet_trades (trading_date, amount DESC);

-- ---------------------------------------------------------------- identity
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL,
    password_hash text NOT NULL,
    display_name  text,
    status        text NOT NULL DEFAULT 'active',
    failed_logins integer NOT NULL DEFAULT 0,
    locked_until  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE roles (
    code text PRIMARY KEY,
    name text NOT NULL
);
CREATE TABLE permissions (code text PRIMARY KEY, description text);
CREATE TABLE role_permissions (
    role_code   text NOT NULL REFERENCES roles(code) ON DELETE CASCADE,
    permission  text NOT NULL REFERENCES permissions(code) ON DELETE CASCADE,
    PRIMARY KEY (role_code, permission)
);
CREATE TABLE user_roles (
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_code  text NOT NULL REFERENCES roles(code) ON DELETE CASCADE,
    granted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_code)
);

CREATE TABLE sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_hash bytea NOT NULL UNIQUE,
    family_id    uuid NOT NULL,
    user_agent   text,
    ip           inet,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    rotated_from uuid REFERENCES sessions(id)
);
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_family_idx ON sessions (family_id);

CREATE TABLE audit_logs (
    id       bigserial PRIMARY KEY,
    at       timestamptz NOT NULL DEFAULT now(),
    actor_id uuid REFERENCES users(id),
    actor_ip inet,
    action   text NOT NULL,
    target   text,
    outcome  text NOT NULL,
    request_id text,
    detail   jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX audit_actor_idx ON audit_logs (actor_id, at DESC);
