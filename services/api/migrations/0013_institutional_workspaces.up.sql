-- 0013_institutional_workspaces: organization-aware multi-tenant institutional workspaces,
-- RBAC, multiple portfolios, research notes, approvals, scheduled reports, and audit trails.

CREATE TABLE organizations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    slug       text NOT NULL UNIQUE CHECK (length(btrim(slug)) BETWEEN 1 AND 50),
    plan       text NOT NULL DEFAULT 'institutional' CHECK (plan IN ('trial', 'pro', 'institutional', 'enterprise')),
    status     text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE organization_members (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            text NOT NULL DEFAULT 'analyst' CHECK (role IN (
                        'org_admin', 'portfolio_manager', 'analyst', 'compliance_officer', 'viewer')),
    joined_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);
CREATE INDEX org_members_user_idx ON organization_members (user_id);

-- Link portfolios to organization
ALTER TABLE portfolios ADD COLUMN IF NOT EXISTS org_id uuid REFERENCES organizations(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS portfolios_org_idx ON portfolios (org_id);

CREATE TABLE research_notes (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    author_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol       text NOT NULL DEFAULT '',
    portfolio_id uuid REFERENCES portfolios(id) ON DELETE SET NULL,
    title        text NOT NULL CHECK (length(btrim(title)) > 0),
    summary      text NOT NULL DEFAULT '',
    body         text NOT NULL DEFAULT '',
    tags         text[] NOT NULL DEFAULT '{}',
    visibility   text NOT NULL DEFAULT 'internal_team' CHECK (visibility IN ('internal_team', 'restricted', 'published')),
    status       text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'under_review', 'approved', 'archived')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX research_notes_org_idx ON research_notes (org_id, updated_at DESC);
CREATE INDEX research_notes_sym_idx ON research_notes (org_id, symbol) WHERE symbol != '';

CREATE TABLE approval_requests (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    portfolio_id uuid REFERENCES portfolios(id) ON DELETE CASCADE,
    requester_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action_type  text NOT NULL CHECK (action_type IN ('large_trade', 'rebalance', 'note_publish', 'cash_withdrawal', 'risk_limit_override')),
    payload      jsonb NOT NULL DEFAULT '{}',
    status       text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    reviewer_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at  timestamptz,
    review_notes text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX approval_requests_org_idx ON approval_requests (org_id, status, created_at DESC);

CREATE TABLE scheduled_reports (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    portfolio_id uuid REFERENCES portfolios(id) ON DELETE CASCADE,
    created_by   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (length(btrim(name)) > 0),
    report_type  text NOT NULL CHECK (report_type IN ('daily_pnl', 'weekly_risk', 'monthly_executive_summary', 'compliance_audit', 'concentration_liquidity')),
    schedule     text NOT NULL CHECK (schedule IN ('daily', 'weekly', 'monthly')),
    recipients   text[] NOT NULL DEFAULT '{}',
    parameters   jsonb NOT NULL DEFAULT '{}',
    is_active    boolean NOT NULL DEFAULT true,
    last_run_at  timestamptz,
    next_run_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX scheduled_reports_org_idx ON scheduled_reports (org_id, is_active);

CREATE TABLE institutional_audit_logs (
    id            bigserial PRIMARY KEY,
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    actor_id      uuid REFERENCES users(id) ON DELETE SET NULL,
    action        text NOT NULL,
    resource_type text NOT NULL,
    resource_id   text NOT NULL DEFAULT '',
    detail        jsonb NOT NULL DEFAULT '{}',
    ip_address    text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX inst_audit_logs_org_idx ON institutional_audit_logs (org_id, created_at DESC);
