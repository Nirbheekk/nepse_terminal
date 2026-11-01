-- 0015_fundamental_data_foundation: additive company fundamentals, corporate-action extensions,
-- financial report facts, mutual funds, source-document provenance and ingestion operations.
-- Preserves all existing tables. No destructive replacement.

CREATE TABLE company_profiles (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          uuid REFERENCES companies(id) ON DELETE CASCADE,
    symbol              text NOT NULL,
    legal_name          text NOT NULL DEFAULT '',
    registered_name     text NOT NULL DEFAULT '',
    registration_number text NOT NULL DEFAULT '',
    pan_number          text NOT NULL DEFAULT '',
    sector              text NOT NULL DEFAULT '',
    sub_sector          text NOT NULL DEFAULT '',
    address             text NOT NULL DEFAULT '',
    website             text NOT NULL DEFAULT '',
    email               text NOT NULL DEFAULT '',
    phone               text NOT NULL DEFAULT '',
    fiscal_year_end     text NOT NULL DEFAULT '',
    incorporation_date  date,
    listed_date         date,
    paid_up_capital     NUMERIC(24,4),
    authorized_capital  NUMERIC(24,4),
    total_shares        NUMERIC(24,4),
    promoter_shares     NUMERIC(24,4),
    public_shares       NUMERIC(24,4),
    currency            text NOT NULL DEFAULT 'NPR',
    source_document_id  uuid,
    effective_date      date,
    published_at        timestamptz,
    status              text NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive','delisted','merged')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol, effective_date)
);
CREATE INDEX company_profiles_symbol_idx ON company_profiles (symbol, effective_date DESC);

CREATE TABLE company_identifiers (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          uuid REFERENCES companies(id) ON DELETE CASCADE,
    symbol              text NOT NULL,
    identifier_type     text NOT NULL CHECK (identifier_type IN ('ISIN','REGISTRATION','PAN','TIN','SEBON_REG','CDSC_CODE','OTHER')),
    identifier_value    text NOT NULL,
    source              text NOT NULL DEFAULT '',
    valid_from          date,
    valid_to            date,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (identifier_type, identifier_value)
);
CREATE INDEX company_identifiers_symbol_idx ON company_identifiers (symbol);

CREATE TABLE company_directors (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          uuid REFERENCES companies(id) ON DELETE CASCADE,
    symbol              text NOT NULL,
    full_name           text NOT NULL,
    role                text NOT NULL DEFAULT '',
    designation         text NOT NULL DEFAULT '',
    appointment_date    date,
    cessation_date      date,
    is_independent      boolean,
    source_document_id  uuid,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol, full_name, role, appointment_date)
);
CREATE INDEX company_directors_symbol_idx ON company_directors (symbol);

CREATE TABLE company_director_appointments (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    director_id         uuid NOT NULL REFERENCES company_directors(id) ON DELETE CASCADE,
    symbol              text NOT NULL,
    event_type          text NOT NULL CHECK (event_type IN ('appointed','reappointed','resigned','removed','term_ended')),
    event_date          date NOT NULL,
    reason              text NOT NULL DEFAULT '',
    source_document_id  uuid,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (director_id, event_type, event_date)
);
CREATE INDEX cda_symbol_date_idx ON company_director_appointments (symbol, event_date DESC);

CREATE TABLE ownership_snapshots (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          uuid REFERENCES companies(id) ON DELETE CASCADE,
    symbol              text NOT NULL,
    effective_date      date NOT NULL,
    observation_date    date,
    publication_date    date,
    promoter_percent    NUMERIC(10,4),
    public_percent      NUMERIC(10,4),
    government_percent  NUMERIC(10,4),
    institutional_percent NUMERIC(10,4),
    foreign_percent     NUMERIC(10,4),
    other_percent       NUMERIC(10,4),
    promoter_shares     NUMERIC(24,4),
    public_shares       NUMERIC(24,4),
    total_shares        NUMERIC(24,4),
    source_document_id  uuid,
    status              text NOT NULL DEFAULT 'PROVISIONAL' CHECK (status IN ('VERIFIED','PROVISIONAL','UNVERIFIED','REVIEW_REQUIRED','REJECTED')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol, effective_date)
);
CREATE INDEX ownership_symbol_date_idx ON ownership_snapshots (symbol, effective_date DESC);
-- corporate actions extensions
CREATE TABLE public_issues (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          uuid REFERENCES companies(id) ON DELETE SET NULL,
    symbol              text NOT NULL,
    issue_type          text NOT NULL CHECK (issue_type IN ('IPO','FPO','RIGHTS','BONUS','MERGER','ACQUISITION','OTHER')),
    issue_name          text NOT NULL DEFAULT '',
    registrar           text NOT NULL DEFAULT '',
    issue_manager       text NOT NULL DEFAULT '',
    open_date           date,
    close_date          date,
    allotment_date      date,
    listing_date        date,
    issue_size          NUMERIC(24,4),
    issue_price         NUMERIC(18,4),
    units_offered       NUMERIC(24,4),
    minimum_units       NUMERIC(24,4),
    status              text NOT NULL DEFAULT 'announced' CHECK (status IN ('announced','open','closed','allotted','listed','cancelled','withdrawn')),
    source_document_id  uuid,
    source_url          text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol, issue_type, open_date)
);
CREATE INDEX public_issues_symbol_idx ON public_issues (symbol, open_date DESC);
CREATE INDEX public_issues_status_idx ON public_issues (status, open_date DESC);

CREATE TABLE public_issue_updates (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    public_issue_id     uuid NOT NULL REFERENCES public_issues(id) ON DELETE CASCADE,
    update_type         text NOT NULL,
    update_date         date NOT NULL,
    detail              text NOT NULL DEFAULT '',
    source_document_id  uuid,
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX piu_issue_idx ON public_issue_updates (public_issue_id, update_date DESC);

CREATE TABLE company_meetings (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          uuid REFERENCES companies(id) ON DELETE SET NULL,
    symbol              text NOT NULL,
    meeting_type        text NOT NULL CHECK (meeting_type IN ('AGM','SGM','EGM','OTHER')),
    meeting_date        date NOT NULL,
    book_closure_date   date,
    agenda              text NOT NULL DEFAULT '',
    venue               text NOT NULL DEFAULT '',
    status              text NOT NULL DEFAULT 'announced' CHECK (status IN ('announced','held','postponed','cancelled')),
    source_document_id  uuid,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol, meeting_type, meeting_date)
);
CREATE INDEX company_meetings_symbol_idx ON company_meetings (symbol, meeting_date DESC);

CREATE TABLE merger_events (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_name          text NOT NULL,
    event_type          text NOT NULL CHECK (event_type IN ('merger','acquisition','demerger','amalgamation')),
    announcement_date   date,
    effective_date      date,
    status              text NOT NULL DEFAULT 'announced' CHECK (status IN ('announced','approved','completed','cancelled')),
    surviving_symbol    text NOT NULL DEFAULT '',
    swap_ratio          NUMERIC(18,6),
    source_document_id  uuid,
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX merger_events_date_idx ON merger_events (announcement_date DESC);

CREATE TABLE merger_participants (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merger_event_id     uuid NOT NULL REFERENCES merger_events(id) ON DELETE CASCADE,
    company_id          uuid REFERENCES companies(id) ON DELETE SET NULL,
    symbol              text NOT NULL,
    role                text NOT NULL CHECK (role IN ('acquirer','target','surviving','dissolved')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merger_event_id, symbol)
);

-- financial reports
CREATE TABLE financial_reports (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          uuid REFERENCES companies(id) ON DELETE SET NULL,
    symbol              text NOT NULL,
    report_type         text NOT NULL CHECK (report_type IN ('quarterly','annual','ttm','interim')),
    fiscal_year         text NOT NULL,
    period_type         text NOT NULL CHECK (period_type IN ('Q1','Q2','Q3','Q4','ANNUAL','TTM')),
    period_start        date NOT NULL,
    period_end          date NOT NULL,
    filing_date         date NOT NULL,
    publication_date    date,
    audit_status        text NOT NULL DEFAULT 'unaudited' CHECK (audit_status IN ('unaudited','audited','restated','preliminary')),
    scope               text NOT NULL DEFAULT 'standalone' CHECK (scope IN ('standalone','consolidated')),
    currency            text NOT NULL DEFAULT 'NPR',
    original_units      text NOT NULL DEFAULT 'NPR',
    normalized_units    text NOT NULL DEFAULT 'NPR',
    source_document_id  uuid,
    status              text NOT NULL DEFAULT 'PROVISIONAL' CHECK (status IN ('VERIFIED','PROVISIONAL','UNVERIFIED','REVIEW_REQUIRED','REJECTED')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol, fiscal_year, period_type, audit_status, scope)
);
CREATE INDEX financial_reports_symbol_idx ON financial_reports (symbol, period_end DESC);

CREATE TABLE financial_statement_facts (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    financial_report_id uuid NOT NULL REFERENCES financial_reports(id) ON DELETE CASCADE,
    statement_type      text NOT NULL CHECK (statement_type IN ('balance_sheet','income_statement','cash_flow','equity_changes','notes')),
    line_code           text NOT NULL,
    line_name           text NOT NULL,
    original_label      text NOT NULL DEFAULT '',
    amount              NUMERIC(28,6),
    amount_text         text,
    unit                text NOT NULL DEFAULT 'NPR',
    normalized_amount   NUMERIC(28,6),
    normalized_unit     text NOT NULL DEFAULT 'NPR',
    line_order          int NOT NULL DEFAULT 0,
    is_calculated       boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (financial_report_id, statement_type, line_code)
);
CREATE INDEX fsf_report_idx ON financial_statement_facts (financial_report_id, statement_type);

CREATE TABLE financial_metric_definitions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    metric_code         text NOT NULL UNIQUE,
    metric_name         text NOT NULL,
    category            text NOT NULL DEFAULT 'ratio' CHECK (category IN ('ratio','growth','profitability','liquidity','solvency','valuation','other')),
    description         text NOT NULL DEFAULT '',
    formula             text NOT NULL DEFAULT '',
    unit                text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE financial_ratio_observations (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    financial_report_id uuid REFERENCES financial_reports(id) ON DELETE CASCADE,
    symbol              text NOT NULL,
    metric_code         text NOT NULL,
    as_of_date          date NOT NULL,
    value               NUMERIC(20,6),
    currency            text NOT NULL DEFAULT 'NPR',
    methodology         text NOT NULL DEFAULT '',
    input_lineage       jsonb NOT NULL DEFAULT '{}'::jsonb,
    is_reported         boolean NOT NULL DEFAULT false,
    is_calculated       boolean NOT NULL DEFAULT false,
    price_used          NUMERIC(18,4),
    price_date          date,
    status              text NOT NULL DEFAULT 'PROVISIONAL' CHECK (status IN ('VERIFIED','PROVISIONAL','UNVERIFIED','REVIEW_REQUIRED','REJECTED')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (financial_report_id, metric_code, as_of_date)
);
CREATE INDEX fro_symbol_date_idx ON financial_ratio_observations (symbol, as_of_date DESC);

CREATE TABLE financial_report_documents (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    financial_report_id uuid REFERENCES financial_reports(id) ON DELETE SET NULL,
    source_document_id  uuid,
    file_name           text NOT NULL DEFAULT '',
    file_url            text NOT NULL DEFAULT '',
    mime_type           text NOT NULL DEFAULT 'application/pdf',
    sha256              text,
    page_count          int,
    parser_confidence   NUMERIC(5,4),
    ocr_required        boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now()
);

-- mutual funds
CREATE TABLE mutual_funds (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fund_code           text NOT NULL UNIQUE,
    fund_name           text NOT NULL,
    manager             text NOT NULL DEFAULT '',
    custodian           text NOT NULL DEFAULT '',
    fund_type           text NOT NULL DEFAULT 'open_ended' CHECK (fund_type IN ('open_ended','closed_ended','interval')),
    inception_date      date,
    currency            text NOT NULL DEFAULT 'NPR',
    website             text NOT NULL DEFAULT '',
    status              text NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive','liquidated')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE mutual_fund_schemes (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mutual_fund_id      uuid NOT NULL REFERENCES mutual_funds(id) ON DELETE CASCADE,
    scheme_code         text NOT NULL,
    scheme_name         text NOT NULL,
    units_outstanding   NUMERIC(24,4),
    nav_per_unit        NUMERIC(18,6),
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (mutual_fund_id, scheme_code)
);

CREATE TABLE mutual_fund_nav_history (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mutual_fund_id      uuid NOT NULL REFERENCES mutual_funds(id) ON DELETE CASCADE,
    scheme_id           uuid REFERENCES mutual_fund_schemes(id) ON DELETE SET NULL,
    valuation_date      date NOT NULL,
    publication_date    date,
    nav_per_unit        NUMERIC(18,6) NOT NULL,
    total_nav           NUMERIC(24,4),
    units_outstanding   NUMERIC(24,4),
    currency            text NOT NULL DEFAULT 'NPR',
    source_document_id  uuid,
    status              text NOT NULL DEFAULT 'PROVISIONAL' CHECK (status IN ('VERIFIED','PROVISIONAL','UNVERIFIED','REVIEW_REQUIRED','REJECTED')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (mutual_fund_id, scheme_id, valuation_date)
);
CREATE INDEX mfnh_fund_date_idx ON mutual_fund_nav_history (mutual_fund_id, valuation_date DESC);

CREATE TABLE mutual_fund_portfolio_disclosures (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mutual_fund_id      uuid NOT NULL REFERENCES mutual_funds(id) ON DELETE CASCADE,
    as_of_date          date NOT NULL,
    holding_symbol      text NOT NULL DEFAULT '',
    holding_name        text NOT NULL DEFAULT '',
    units               NUMERIC(24,4),
    market_value        NUMERIC(24,4),
    weight_percent      NUMERIC(10,4),
    source_document_id  uuid,
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mfpd_fund_date_idx ON mutual_fund_portfolio_disclosures (mutual_fund_id, as_of_date DESC);

-- ingestion operations
CREATE TABLE source_documents (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    data_source_id      uuid REFERENCES data_sources(id) ON DELETE SET NULL,
    source_slug         text NOT NULL,
    document_type       text NOT NULL,
    title               text NOT NULL DEFAULT '',
    source_url          text NOT NULL DEFAULT '',
    file_name           text NOT NULL DEFAULT '',
    mime_type           text NOT NULL DEFAULT 'application/octet-stream',
    sha256              text CHECK (sha256 IS NULL OR sha256 ~ '^[0-9a-f]{64}$'),
    size_bytes          bigint,
    fetched_at          timestamptz,
    published_at        timestamptz,
    license_class       text NOT NULL DEFAULT 'public_unverified',
    redistribution      text NOT NULL DEFAULT 'prohibited',
    status              text NOT NULL DEFAULT 'captured' CHECK (status IN ('captured','parsed','validated','rejected','archived')),
    meta                jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX source_documents_slug_idx ON source_documents (source_slug, fetched_at DESC);
CREATE UNIQUE INDEX source_documents_digest_key ON source_documents (source_slug, sha256) WHERE sha256 IS NOT NULL;

CREATE TABLE ingestion_jobs (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_name            text NOT NULL,
    source_slug         text NOT NULL,
    status              text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','success','partial','failed','cancelled')),
    started_at          timestamptz,
    finished_at         timestamptz,
    worker              text NOT NULL DEFAULT '',
    parameters          jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ingestion_jobs_recent_idx ON ingestion_jobs (started_at DESC);

CREATE TABLE ingestion_runs (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id              uuid REFERENCES ingestion_jobs(id) ON DELETE SET NULL,
    source_slug         text NOT NULL,
    stream              text NOT NULL,
    status              text NOT NULL CHECK (status IN ('success','partial','failed','skipped')),
    source_document_id  uuid REFERENCES source_documents(id) ON DELETE SET NULL,
    rows_read           integer NOT NULL DEFAULT 0,
    rows_written        integer NOT NULL DEFAULT 0,
    rows_unchanged      integer NOT NULL DEFAULT 0,
    rows_rejected       integer NOT NULL DEFAULT 0,
    error               text,
    started_at          timestamptz NOT NULL DEFAULT now(),
    finished_at         timestamptz
);
CREATE INDEX ingestion_runs_stream_idx ON ingestion_runs (stream, started_at DESC);

CREATE TABLE ingestion_errors (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id              uuid REFERENCES ingestion_runs(id) ON DELETE CASCADE,
    source_slug         text NOT NULL,
    entity_type         text NOT NULL DEFAULT '',
    entity_key          text NOT NULL DEFAULT '',
    error_code          text NOT NULL,
    error_message       text NOT NULL,
    payload             jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ingestion_errors_run_idx ON ingestion_errors (run_id, created_at DESC);

CREATE TABLE data_quality_findings (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_slug         text NOT NULL,
    entity_type         text NOT NULL,
    entity_key          text NOT NULL DEFAULT '',
    finding_type        text NOT NULL,
    severity            text NOT NULL CHECK (severity IN ('info','warning','error','critical')),
    message             text NOT NULL,
    status              text NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved','ignored','false_positive')),
    run_id              uuid REFERENCES ingestion_runs(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    resolved_at         timestamptz
);
CREATE INDEX dqf_source_idx ON data_quality_findings (source_slug, created_at DESC);

CREATE TABLE entity_resolution_reviews (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type         text NOT NULL,
    canonical_key       text NOT NULL,
    candidate_key       text NOT NULL,
    confidence          NUMERIC(5,4),
    status              text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','merged')),
    reviewer            text NOT NULL DEFAULT '',
    notes               text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    resolved_at         timestamptz
);
CREATE INDEX err_status_idx ON entity_resolution_reviews (status, created_at DESC);

