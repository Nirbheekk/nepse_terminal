-- 0018_financial_revisions: add revision tracking to financial_reports.
ALTER TABLE financial_reports ADD COLUMN IF NOT EXISTS revision_number int NOT NULL DEFAULT 1;
ALTER TABLE financial_reports ADD COLUMN IF NOT EXISTS supersedes_id uuid REFERENCES financial_reports(id) ON DELETE SET NULL;
ALTER TABLE financial_reports ADD COLUMN IF NOT EXISTS parser_version text NOT NULL DEFAULT '1';
CREATE INDEX IF NOT EXISTS financial_reports_revision_idx ON financial_reports (symbol, fiscal_year, revision_number);
