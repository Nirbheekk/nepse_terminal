# Batch 4.3 — Parser Implementation

## Implemented
- parsers/html_tables.py: BeautifulSoup (lxml) table extraction, header detection, dict conversion.
- parsers/pdf.py: pdfplumber text + table extraction; OCR_REQUIRED flag for scanned/empty pages.
- parsers/structured.py: CSV, Excel (openpyxl via pandas), JSON.
- parsers/cdsc_ipo.py: CDSC IPO list HTML -> normalized records with date/number parsing.
- normalizers/numbers.py: decimal parsing (no zero-for-missing), unit conversion, fiscal period mapping, fiscal year derivation, symbol/text cleaning.

## Tests
- tests/unit/test_numbers.py
- tests/unit/test_html_tables.py
- tests/unit/test_cdsc_parser.py
All passing.
