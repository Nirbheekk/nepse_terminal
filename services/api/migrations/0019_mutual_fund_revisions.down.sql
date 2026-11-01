-- 0019 down
DROP INDEX IF EXISTS mfnh_revision_idx;
ALTER TABLE mutual_fund_nav_history DROP COLUMN IF EXISTS supersedes_id;
ALTER TABLE mutual_fund_nav_history DROP COLUMN IF EXISTS revision_number;
