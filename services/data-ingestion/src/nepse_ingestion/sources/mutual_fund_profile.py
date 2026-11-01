"""Mutual fund profile adapter (fixture-backed)."""

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


class MutualFundProfileAdapter(SourceAdapter):
    def __init__(self, fixture_path: str | None = None):
        d = get_source("mutual-fund-managers")
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
        return RawDocument(source_slug=self.descriptor.slug, document_type="json", title="MF profile (fixture)",
                           source_url=self.descriptor.url, file_name=p.name, mime_type="application/json", content=c,
                           sha256=hashlib.sha256(c).hexdigest(), fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw):
        import json
        data = json.loads(raw.content.decode("utf-8"))
        if isinstance(data, dict) and "funds" in data:
            return data["funds"]
        return data if isinstance(data, list) else [data]

    def normalize(self, records):
        out = []
        for r in records:
            r = dict(r)
            r["fund_code"] = clean_text(r.get("fund_code", ""))
            r["fund_name"] = clean_text(r.get("fund_name", ""))
            r["manager"] = clean_text(r.get("manager", ""))
            r["fund_type"] = r.get("fund_type", "open_ended")
            if r["fund_type"] == "close_ended":
                r["fund_type"] = "closed_ended"
            r["launch_date"] = parse_date(r.get("launch_date"))
            r["maturity_date"] = parse_date(r.get("maturity_date"))
            r["outstanding_units"] = parse_decimal(r.get("outstanding_units"))
            r["benchmark"] = clean_text(r.get("benchmark", ""))
            r["currency"] = r.get("currency", "NPR")
            r["status"] = r.get("status", "active")
            r["_valid"] = bool(r["fund_code"] and r["fund_name"])
            out.append(r)
        return out
