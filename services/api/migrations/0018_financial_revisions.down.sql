-- 0018_financial_revisions down
DROP INDEX IF EXISTS financial_reports_revision_idx;
ALTER TABLE financial_reports DROP COLUMN IF EXISTS parser_version;
ALTER TABLE financial_reports DROP COLUMN IF EXISTS supersedes_id;
ALTER TABLE financial_reports DROP COLUMN IF EXISTS revision_number;
