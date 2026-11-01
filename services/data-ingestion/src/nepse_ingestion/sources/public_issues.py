"""Public issue adapter (fixture-backed)."""

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


class PublicIssueAdapter(SourceAdapter):
    def __init__(self, fixture_path: str | None = None):
        d = get_source("sebon-public-issues")
        if d is None:
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
        return RawDocument(source_slug=self.descriptor.slug, document_type="json", title="Public issue (fixture)",
                           source_url=self.descriptor.url, file_name=p.name, mime_type="application/json", content=c,
                           sha256=hashlib.sha256(c).hexdigest(), fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw):
        import json
        data = json.loads(raw.content.decode("utf-8"))
        if isinstance(data, dict) and "issues" in data:
            return data["issues"]
        return data if isinstance(data, list) else [data]

    def normalize(self, records):
        out = []
        for r in records:
            r = dict(r)
            r["symbol"] = clean_symbol(r.get("symbol", ""))
            r["issue_type"] = r.get("issue_type", "IPO")
            r["issue_name"] = clean_text(r.get("issue_name", ""))
            r["open_date"] = parse_date(r.get("open_date"))
            r["close_date"] = parse_date(r.get("close_date"))
            r["allotment_date"] = parse_date(r.get("allotment_date"))
            r["listing_date"] = parse_date(r.get("listing_date"))
            r["issue_price"] = parse_decimal(r.get("issue_price"))
            r["units_offered"] = parse_decimal(r.get("units_offered"))
            r["issue_size"] = parse_decimal(r.get("issue_size"))
            r["status"] = r.get("status", "announced")
            r["_valid"] = bool(r["symbol"] and r["issue_type"])
            out.append(r)
        return out
