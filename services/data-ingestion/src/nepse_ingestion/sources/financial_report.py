"""Financial report adapter (fixture-backed)."""

from __future__ import annotations

import hashlib
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

from ..config import settings
from ..sources.base import RawDocument, SourceAdapter, SourceDescriptor
from ..sources.registry import get_source
from ..normalizers.numbers import clean_symbol, clean_text, parse_decimal
from ..normalizers.dates import parse_date
from ..normalizers.financial import map_label


class FinancialReportAdapter(SourceAdapter):
    def __init__(self, fixture_path: str | None = None):
        d = get_source("issuer-investor-relations")
        assert d is not None
        self.descriptor = d
        self.fixture_path = fixture_path

    def discover(self): return [{"source": self.descriptor.slug}]

    def fetch(self) -> RawDocument:
        if not (self.descriptor.live_fetch_enabled or settings.live_fetch_enabled):
            raise RuntimeError("Live fetch disabled")
        raise NotImplementedError

    def fetch_fixture(self, path):
        p = Path(path); c = p.read_bytes()
        return RawDocument(source_slug=self.descriptor.slug, document_type="financial_report",
                           title="Financial report (fixture)", source_url=self.descriptor.url,
                           file_name=p.name, mime_type="application/json", content=c,
                           sha256=hashlib.sha256(c).hexdigest(), fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw):
        import json
        data = json.loads(raw.content.decode("utf-8"))
        if isinstance(data, dict) and "reports" in data:
            return data["reports"]
        return data if isinstance(data, list) else [data]

    def normalize(self, records):
        out = []
        for r in records:
            r = dict(r)
            r["symbol"] = clean_symbol(r.get("symbol", ""))
            r["report_type"] = r.get("report_type", "quarterly")
            r["fiscal_year"] = clean_text(r.get("fiscal_year", ""))
            r["period_type"] = r.get("period_type", "Q1")
            r["period_start"] = parse_date(r.get("period_start"))
            r["period_end"] = parse_date(r.get("period_end"))
            r["filing_date"] = parse_date(r.get("filing_date"))
            r["publication_date"] = parse_date(r.get("publication_date"))
            r["audit_status"] = r.get("audit_status", "unaudited")
            r["scope"] = r.get("scope", "standalone")
            r["currency"] = r.get("currency", "NPR")
            r["original_units"] = r.get("original_units", "NPR")
            r["normalized_units"] = r.get("normalized_units", "NPR")
            # facts
            facts = r.get("facts", [])
            norm_facts = []
            for f in facts:
                f = dict(f)
                f["canonical"] = map_label(f.get("label", ""))
                f["amount"] = parse_decimal(f.get("amount"))
                norm_facts.append(f)
            r["facts"] = norm_facts
            r["_valid"] = bool(r["symbol"] and r["fiscal_year"] and r["period_start"] and r["period_end"] and r["filing_date"])
            out.append(r)
        return out
