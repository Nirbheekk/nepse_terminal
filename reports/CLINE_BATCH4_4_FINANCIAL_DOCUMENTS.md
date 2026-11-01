# Batch 4.4 — Financial Documents

## Adapter
IssuerFinancialReportAdapter (fixture-backed). Registers PDF metadata; uses pdfplumber for page count/OCR flag.

## Pipeline
run_financial_document_pipeline: inserts financial_report_documents with checksum/page_count/ocr_required.

## Rules
- No bulk PDF download without authorization.
- Scanned PDFs marked OCR_REQUIRED.
- Missing values never zero.
