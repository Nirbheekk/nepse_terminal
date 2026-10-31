-- 0010_financials_corporate_actions: normalized company financial statements, line items,
-- reporting-period-aware ratios, dividends, and corporate actions.

CREATE TABLE financial_periods (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id   uuid REFERENCES companies(id) ON DELETE CASCADE,
    symbol       text NOT NULL,
    fiscal_year  text NOT NULL CHECK (length(fiscal_year) BETWEEN 4 AND 12),
    period_type  text NOT NULL CHECK (period_type IN ('Q1', 'Q2', 'Q3', 'Q4', 'ANNUAL', 'TTM')),
    period_start date NOT NULL,
    period_end   date NOT NULL,
    filing_date  date NOT NULL,
    audit_status text NOT NULL DEFAULT 'unaudited' CHECK (audit_status IN ('unaudited', 'audited', 'restated', 'preliminary')),
    currency     text NOT NULL DEFAULT 'NPR',
    is_ttm       boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol, fiscal_year, period_type, audit_status)
);
CREATE INDEX financial_periods_sym_idx ON financial_periods (symbol, period_end DESC);
CREATE INDEX financial_periods_filing_idx ON financial_periods (filing_date DESC);

CREATE TABLE financial_statement_lines (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    period_id      uuid NOT NULL REFERENCES financial_periods(id) ON DELETE CASCADE,
    statement_type text NOT NULL CHECK (statement_type IN ('balance_sheet', 'income_statement', 'cash_flow')),
    line_code      text NOT NULL CHECK (length(line_code) <= 60),
    line_name      text NOT NULL,
    amount         NUMERIC(24,4) NOT NULL,
    line_order     int NOT NULL DEFAULT 0,
    unit           text NOT NULL DEFAULT 'NPR',
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (period_id, line_code)
);
CREATE INDEX fin_lines_period_idx ON financial_statement_lines (period_id, statement_type);

CREATE TABLE financial_ratios (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    period_id          uuid NOT NULL REFERENCES financial_periods(id) ON DELETE CASCADE,
    symbol             text NOT NULL,
    as_of_date         date NOT NULL,
    price_used         NUMERIC(18,4) NOT NULL,
    market_cap         NUMERIC(24,4),
    eps                NUMERIC(14,4),
    annualized_eps     NUMERIC(14,4),
    book_value_per_share NUMERIC(14,4),
    pe_ratio           NUMERIC(14,4),
    pb_ratio           NUMERIC(14,4),
    ps_ratio           NUMERIC(14,4),
    ev_to_ebitda       NUMERIC(14,4),
    roe                NUMERIC(10,4),
    roa                NUMERIC(10,4),
    gross_margin       NUMERIC(10,4),
    operating_margin   NUMERIC(10,4),
    net_margin         NUMERIC(10,4),
    current_ratio      NUMERIC(10,4),
    quick_ratio        NUMERIC(10,4),
    debt_to_equity     NUMERIC(10,4),
    interest_coverage  NUMERIC(10,4),
    asset_turnover     NUMERIC(10,4),
    formula_details    jsonb NOT NULL DEFAULT '{}',
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (period_id, as_of_date)
);
CREATE INDEX fin_ratios_sym_date_idx ON financial_ratios (symbol, as_of_date DESC);

CREATE TABLE corporate_actions (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id         uuid REFERENCES companies(id) ON DELETE CASCADE,
    symbol             text NOT NULL,
    action_type        text NOT NULL CHECK (action_type IN ('cash_dividend', 'bonus_share', 'rights_issue', 'stock_split', 'reverse_split', 'merger')),
    fiscal_year        text NOT NULL,
    announcement_date  date,
    book_closure_date  date NOT NULL, -- Ex-date / record date
    agm_date           date,
    effective_date     date,
    cash_percent       NUMERIC(10,4) NOT NULL DEFAULT 0,
    cash_amount        NUMERIC(18,4) NOT NULL DEFAULT 0,
    bonus_percent      NUMERIC(10,4) NOT NULL DEFAULT 0,
    rights_ratio       NUMERIC(14,6) NOT NULL DEFAULT 0,
    rights_price       NUMERIC(18,4) NOT NULL DEFAULT 0,
    split_from         NUMERIC(14,6) NOT NULL DEFAULT 0,
    split_to           NUMERIC(14,6) NOT NULL DEFAULT 0,
    status             text NOT NULL DEFAULT 'announced' CHECK (status IN ('announced', 'approved_bod', 'approved_agm', 'completed', 'cancelled')),
    notes              text NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol, action_type, fiscal_year, book_closure_date)
);
CREATE INDEX corp_actions_sym_date_idx ON corporate_actions (symbol, book_closure_date DESC);
CREATE INDEX corp_actions_type_idx ON corporate_actions (action_type, book_closure_date DESC);
