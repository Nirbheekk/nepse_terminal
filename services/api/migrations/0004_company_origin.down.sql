DROP FUNCTION IF EXISTS class_parent_symbol(text);

DROP INDEX IF EXISTS companies_origin_idx;

ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_origin_check;

ALTER TABLE companies DROP COLUMN IF EXISTS origin;
