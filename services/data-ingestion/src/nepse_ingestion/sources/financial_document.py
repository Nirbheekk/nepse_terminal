"""Issuer financial report document registration adapter (fixture-backed)."""

from __future__ import annotations

import hashlib
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

from ..config import settings
from ..sources.base import RawDocument, SourceAdapter, SourceDescriptor
from ..sources.registry import get_source
from ..normalizers.numbers import clean_symbol, clean_text
from ..parsers.pdf import extract_pdf


class IssuerFinancialReportAdapter(SourceAdapter):
    def __init__(self, fixture_path: str | None = None):
        desc = get_source("issuer-investor-relations")
        assert desc is not None
        self.descriptor = desc
        self.fixture_path = fixture_path

    def discover(self) -> List[Dict[str, Any]]:
        return [{"source": self.descriptor.slug}]

    def fetch(self) -> RawDocument:
        if not (self.descriptor.live_fetch_enabled or settings.live_fetch_enabled):
            raise RuntimeError("Live fetch disabled")
        raise NotImplementedError

    def fetch_fixture(self, path: str | Path) -> RawDocument:
        p = Path(path)
        content = p.read_bytes()
        mime = "application/pdf" if p.suffix.lower() == ".pdf" else "application/octet-stream"
        return RawDocument(source_slug=self.descriptor.slug, document_type="financial_report",
                           title="Financial report (fixture)", source_url=self.descriptor.url,
                           file_name=p.name, mime_type=mime, content=content,
                           sha256=hashlib.sha256(content).hexdigest(),
                           fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw: RawDocument) -> List[Dict[str, Any]]:
        # Registration metadata only; PDF text extraction records page count / OCR flag.
        meta = {"file_name": raw.file_name, "sha256": raw.sha256, "mime_type": raw.mime_type}
        if raw.file_name.lower().endswith(".pdf"):
            import tempfile, os
            tmp = Path(tempfile.gettempdir()) / raw.file_name
            tmp.write_bytes(raw.content)
            doc = extract_pdf(tmp)
            meta["page_count"] = len(doc.pages)
            meta["ocr_required"] = doc.ocr_required
            try:
                tmp.unlink()
            except OSError:
                pass
        return [meta]

    def normalize(self, records: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        out = []
        for r in records:
            r = dict(r)
            r["symbol"] = clean_symbol(r.get("symbol", ""))
            r["fiscal_year"] = clean_text(r.get("fiscal_year", ""))
            r["report_type"] = r.get("report_type", "quarterly")
            r["audit_status"] = r.get("audit_status", "unaudited")
            r["_valid"] = True
            out.append(r)
        return out
