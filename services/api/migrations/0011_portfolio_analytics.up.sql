-- 0011_portfolio_analytics: portfolio tracking, tax lot accounting, cost basis engines,
-- cash balances, transactions, and performance analytics.
--
-- Architectural separation:
-- Portfolio tracking is strictly decoupled from brokerage execution.
-- No execution orders, broker gateways or OMS/TMS state machines exist in this ledger schema.

CREATE TABLE portfolios (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name              text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    description       text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
    currency          text NOT NULL DEFAULT 'NPR',
    cost_basis_method text NOT NULL DEFAULT 'WAC' CHECK (cost_basis_method IN ('FIFO', 'WAC')),
    benchmark_symbol  text NOT NULL DEFAULT 'NEPSE',
    cash_balance      NUMERIC(24,4) NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX portfolios_owner_name_key
    ON portfolios (user_id, lower(btrim(name)));
CREATE INDEX portfolios_owner_idx ON portfolios (user_id, updated_at DESC);

CREATE TABLE portfolio_positions (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    portfolio_id       uuid NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    symbol             text NOT NULL,
    quantity           NUMERIC(20,4) NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    cost_basis         NUMERIC(24,4) NOT NULL DEFAULT 0,
    avg_cost_per_share NUMERIC(18,6) NOT NULL DEFAULT 0,
    market_price       NUMERIC(18,4) NOT NULL DEFAULT 0,
    market_value       NUMERIC(24,4) NOT NULL DEFAULT 0,
    unrealized_pnl     NUMERIC(24,4) NOT NULL DEFAULT 0,
    unrealized_pnl_pct NUMERIC(10,4) NOT NULL DEFAULT 0,
    realized_pnl       NUMERIC(24,4) NOT NULL DEFAULT 0,
    dividends_received NUMERIC(24,4) NOT NULL DEFAULT 0,
    last_updated       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (portfolio_id, symbol)
);
CREATE INDEX ppos_portfolio_idx ON portfolio_positions (portfolio_id);

CREATE TABLE portfolio_lots (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    portfolio_id       uuid NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    symbol             text NOT NULL,
    transaction_id     text NOT NULL DEFAULT '',
    purchase_date      date NOT NULL,
    original_quantity  NUMERIC(20,4) NOT NULL CHECK (original_quantity > 0),
    remaining_quantity NUMERIC(20,4) NOT NULL CHECK (remaining_quantity >= 0),
    cost_per_share     NUMERIC(18,6) NOT NULL DEFAULT 0,
    total_cost         NUMERIC(24,4) NOT NULL DEFAULT 0,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX plots_portfolio_sym_idx ON portfolio_lots (portfolio_id, symbol, purchase_date);

CREATE TABLE portfolio_transactions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    portfolio_id      uuid NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    symbol            text NOT NULL DEFAULT '',
    txn_type          text NOT NULL CHECK (txn_type IN (
                          'BUY', 'SELL', 'DIVIDEND_CASH', 'DIVIDEND_BONUS', 'RIGHTS_EXERCISE',
                          'DEPOSIT', 'WITHDRAWAL', 'FEE', 'TAX', 'SPLIT_ADJUSTMENT',
                          'TRANSFER_IN', 'TRANSFER_OUT')),
    traded_on         date NOT NULL,
    settlement_date   date,
    quantity          NUMERIC(20,4) NOT NULL DEFAULT 0,
    price             NUMERIC(18,6) NOT NULL DEFAULT 0,
    gross_amount      NUMERIC(24,4) NOT NULL DEFAULT 0,
    broker_commission NUMERIC(18,4) NOT NULL DEFAULT 0,
    sebon_fee         NUMERIC(18,4) NOT NULL DEFAULT 0,
    dp_fee            NUMERIC(18,4) NOT NULL DEFAULT 0,
    cgt               NUMERIC(18,4) NOT NULL DEFAULT 0,
    total_fees        NUMERIC(18,4) NOT NULL DEFAULT 0,
    net_amount        NUMERIC(24,4) NOT NULL DEFAULT 0,
    cost_basis        NUMERIC(24,4) NOT NULL DEFAULT 0,
    realized_pnl      NUMERIC(24,4) NOT NULL DEFAULT 0,
    notes             text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ptxns_portfolio_date_idx ON portfolio_transactions (portfolio_id, traded_on DESC);

CREATE TABLE portfolio_daily_snapshots (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    portfolio_id      uuid NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    snapshot_date     date NOT NULL,
    portfolio_value   NUMERIC(24,4) NOT NULL,
    cash_balance      NUMERIC(24,4) NOT NULL DEFAULT 0,
    total_cost_basis  NUMERIC(24,4) NOT NULL DEFAULT 0,
    net_cash_flow     NUMERIC(24,4) NOT NULL DEFAULT 0,
    benchmark_value   NUMERIC(18,4),
    daily_return      NUMERIC(14,8) NOT NULL DEFAULT 0,
    twr               NUMERIC(14,8) NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (portfolio_id, snapshot_date)
);
CREATE INDEX psnap_portfolio_date_idx ON portfolio_daily_snapshots (portfolio_id, snapshot_date DESC);
