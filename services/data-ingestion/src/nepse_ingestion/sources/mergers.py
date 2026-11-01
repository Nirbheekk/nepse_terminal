"""Merger disclosure adapter (fixture-backed)."""

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


class MergerDisclosureAdapter(SourceAdapter):
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
        return RawDocument(source_slug=self.descriptor.slug, document_type="json", title="Merger disclosure (fixture)",
                           source_url=self.descriptor.url, file_name=p.name, mime_type="application/json", content=c,
                           sha256=hashlib.sha256(c).hexdigest(), fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw):
        import json
        data = json.loads(raw.content.decode("utf-8"))
        if isinstance(data, dict) and "mergers" in data:
            return data["mergers"]
        return data if isinstance(data, list) else [data]

    def normalize(self, records):
        out = []
        for r in records:
            r = dict(r)
            r["event_name"] = clean_text(r.get("event_name", ""))
            r["event_type"] = r.get("event_type", "merger")
            r["announcement_date"] = parse_date(r.get("announcement_date"))
            r["effective_date"] = parse_date(r.get("effective_date"))
            r["status"] = r.get("status", "announced")
            r["surviving_symbol"] = clean_symbol(r.get("surviving_symbol", ""))
            r["swap_ratio"] = parse_decimal(r.get("swap_ratio"))
            r["participants"] = r.get("participants", [])
            r["_valid"] = bool(r["event_name"])
            out.append(r)
        return out
