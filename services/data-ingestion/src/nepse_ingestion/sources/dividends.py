"""Dividend and capital action adapter (fixture-backed)."""

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


def _base_desc():
    d = get_source("issuer-investor-relations")
    assert d is not None
    return d


class DividendCapitalActionAdapter(SourceAdapter):
    def __init__(self, fixture_path: str | None = None):
        self.descriptor = _base_desc()
        self.fixture_path = fixture_path

    def discover(self): return [{"source": self.descriptor.slug}]

    def fetch(self) -> RawDocument:
        if not (self.descriptor.live_fetch_enabled or settings.live_fetch_enabled):
            raise RuntimeError("Live fetch disabled")
        raise NotImplementedError

    def fetch_fixture(self, path):
        p = Path(path); c = p.read_bytes()
        return RawDocument(source_slug=self.descriptor.slug, document_type="json", title="Dividend/capital action (fixture)",
                           source_url=self.descriptor.url, file_name=p.name, mime_type="application/json", content=c,
                           sha256=hashlib.sha256(c).hexdigest(), fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw):
        import json
        data = json.loads(raw.content.decode("utf-8"))
        if isinstance(data, dict) and "actions" in data:
            return data["actions"]
        return data if isinstance(data, list) else [data]

    def normalize(self, records):
        out = []
        for r in records:
            r = dict(r)
            r["symbol"] = clean_symbol(r.get("symbol", ""))
            r["action_type"] = r.get("action_type", "cash_dividend")
            r["fiscal_year"] = clean_text(r.get("fiscal_year", ""))
            r["announcement_date"] = parse_date(r.get("announcement_date"))
            r["book_closure_date"] = parse_date(r.get("book_closure_date")) or parse_date(r.get("record_date"))
            r["agm_date"] = parse_date(r.get("agm_date"))
            r["effective_date"] = parse_date(r.get("effective_date"))
            r["cash_percent"] = parse_decimal(r.get("cash_percent")) or 0
            r["cash_amount"] = parse_decimal(r.get("cash_amount")) or 0
            r["bonus_percent"] = parse_decimal(r.get("bonus_percent")) or 0
            r["rights_ratio"] = parse_decimal(r.get("rights_ratio")) or 0
            r["rights_price"] = parse_decimal(r.get("rights_price")) or 0
            r["split_from"] = parse_decimal(r.get("split_from")) or 0
            r["split_to"] = parse_decimal(r.get("split_to")) or 0
            r["status"] = r.get("status", "announced")
            r["notes"] = clean_text(r.get("notes", ""))
            r["_valid"] = bool(r["symbol"] and r["action_type"] and r["fiscal_year"] and r["book_closure_date"])
            out.append(r)
        return out
