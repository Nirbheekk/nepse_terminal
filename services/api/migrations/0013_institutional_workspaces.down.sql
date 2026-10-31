DROP TABLE IF EXISTS institutional_audit_logs;
DROP TABLE IF EXISTS scheduled_reports;
DROP TABLE IF EXISTS approval_requests;
DROP TABLE IF EXISTS research_notes;
ALTER TABLE portfolios DROP COLUMN IF EXISTS org_id;
DROP TABLE IF EXISTS organization_members;
DROP TABLE IF EXISTS organizations;
