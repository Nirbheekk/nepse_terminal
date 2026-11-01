"""PDF text and table extraction utilities."""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import List, Optional

import pdfplumber


@dataclass
class PdfPage:
    page_number: int
    text: str
    tables: List[List[List[str]]] = field(default_factory=list)


@dataclass
class PdfDocument:
    path: str
    pages: List[PdfPage] = field(default_factory=list)
    ocr_required: bool = False

    @property
    def full_text(self) -> str:
        return "\n".join(p.text for p in self.pages)


def extract_pdf(path: str | Path) -> PdfDocument:
    doc = PdfDocument(path=str(path))
    with pdfplumber.open(path) as pdf:
        for i, page in enumerate(pdf.pages, start=1):
            text = page.extract_text() or ""
            tables = page.extract_tables() or []
            # Normalize table cells to strings.
            norm_tables: List[List[List[str]]] = []
            for t in tables:
                norm_tables.append(
                    [[("" if c is None else str(c).strip()) for c in row] for row in t]
                )
            if text.strip() == "" and not norm_tables:
                doc.ocr_required = True
            doc.pages.append(PdfPage(page_number=i, text=text, tables=norm_tables))
    if not doc.pages:
        doc.ocr_required = True
    return doc
