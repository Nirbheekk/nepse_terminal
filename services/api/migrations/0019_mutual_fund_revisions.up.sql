-- 0019_mutual_fund_revisions: track NAV revisions.
ALTER TABLE mutual_fund_nav_history ADD COLUMN IF NOT EXISTS revision_number int NOT NULL DEFAULT 1;
ALTER TABLE mutual_fund_nav_history ADD COLUMN IF NOT EXISTS supersedes_id uuid REFERENCES mutual_fund_nav_history(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS mfnh_revision_idx ON mutual_fund_nav_history (mutual_fund_id, valuation_date, revision_number);
