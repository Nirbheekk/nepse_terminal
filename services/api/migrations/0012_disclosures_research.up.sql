-- 0012_disclosures_research: authorized NEPSE, SEBON, NRB and company disclosures,
-- attachments, licensing provenance, and evidence-grounded research tracking.

CREATE TABLE disclosures (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    authority        text NOT NULL CHECK (authority IN ('NEPSE', 'SEBON', 'NRB', 'COMPANY')),
    company_id       uuid REFERENCES companies(id) ON DELETE SET NULL,
    symbol           text NOT NULL DEFAULT '',
    category         text NOT NULL CHECK (category IN (
                         'circular', 'directive', 'dividend_notice', 'financial_report',
                         'agm_notice', 'suspension', 'listing_notice', 'ipo_approval',
                         'monetary_policy', 'enforcement', 'material_disclosure')),
    title            text NOT NULL CHECK (length(btrim(title)) > 0),
    summary          text NOT NULL DEFAULT '',
    body             text NOT NULL DEFAULT '',
    published_at     timestamptz NOT NULL,
    effective_date   date,
    source_url       text NOT NULL DEFAULT '',
    reference_number text NOT NULL DEFAULT '',
    license_class    text NOT NULL DEFAULT 'official_verified' CHECK (license_class IN (
                         'official_verified', 'third_party_licensed', 'public_unverified', 'derived', 'restricted')),
    redistribution   text NOT NULL DEFAULT 'permitted_with_attribution' CHECK (redistribution IN (
                         'prohibited', 'internal_only', 'permitted_with_attribution', 'licensed')),
    is_verified      boolean NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX disclosures_authority_idx ON disclosures (authority, published_at DESC);
CREATE INDEX disclosures_symbol_idx ON disclosures (symbol, published_at DESC) WHERE symbol != '';
CREATE INDEX disclosures_category_idx ON disclosures (category, published_at DESC);
CREATE INDEX disclosures_pub_idx ON disclosures (published_at DESC);

CREATE TABLE disclosure_attachments (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    disclosure_id    uuid NOT NULL REFERENCES disclosures(id) ON DELETE CASCADE,
    file_name        text NOT NULL,
    file_url         text NOT NULL,
    file_size_bytes  bigint NOT NULL DEFAULT 0,
    mime_type        text NOT NULL DEFAULT 'application/pdf',
    sha256           text NOT NULL,
    storage_key      text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX disc_att_disclosure_idx ON disclosure_attachments (disclosure_id);

CREATE TABLE research_queries (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid REFERENCES users(id) ON DELETE SET NULL, -- nullable for guest/public research
    query_text       text NOT NULL,
    symbol           text NOT NULL DEFAULT '',
    scope            text NOT NULL DEFAULT 'market' CHECK (scope IN ('market', 'company', 'regulatory', 'portfolio')),
    portfolio_id     uuid REFERENCES portfolios(id) ON DELETE CASCADE, -- only valid when scope = 'portfolio'
    answer           text NOT NULL,
    confidence_score NUMERIC(5,4) NOT NULL DEFAULT 1.0000,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX research_queries_user_idx ON research_queries (user_id, created_at DESC);

CREATE TABLE research_citations (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    query_id         uuid NOT NULL REFERENCES research_queries(id) ON DELETE CASCADE,
    source_type      text NOT NULL CHECK (source_type IN ('disclosure', 'financial_statement', 'ratio', 'market_price', 'corporate_action', 'portfolio')),
    source_id        text NOT NULL,
    authority        text NOT NULL DEFAULT '',
    citation_text    text NOT NULL,
    as_of_date       date,
    claim_type       text NOT NULL CHECK (claim_type IN ('FACT', 'CALCULATION', 'INTERPRETATION')),
    attachment_url   text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX research_citations_query_idx ON research_citations (query_id);
