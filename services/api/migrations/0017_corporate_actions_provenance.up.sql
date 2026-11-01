-- 0017_corporate_actions_provenance: add source document reference to corporate_actions.
ALTER TABLE corporate_actions ADD COLUMN IF NOT EXISTS source_document_id uuid REFERENCES source_documents(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS corp_actions_source_doc_idx ON corporate_actions (source_document_id);
