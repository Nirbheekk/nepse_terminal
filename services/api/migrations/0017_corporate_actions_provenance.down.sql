-- 0017_corporate_actions_provenance down
DROP INDEX IF EXISTS corp_actions_source_doc_idx;
ALTER TABLE corporate_actions DROP COLUMN IF EXISTS source_document_id;
