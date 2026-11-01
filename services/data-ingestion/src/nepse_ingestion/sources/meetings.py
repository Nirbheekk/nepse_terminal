"""Company meeting (AGM/SGM) adapter (fixture-backed)."""

from __future__ import annotations

import hashlib
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

from ..config import settings
from ..sources.base import RawDocument, SourceAdapter, SourceDescriptor
from ..sources.registry import get_source
from ..normalizers.numbers import clean_symbol, clean_text
from ..normalizers.dates import parse_date


class CompanyMeetingAdapter(SourceAdapter):
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
        return RawDocument(source_slug=self.descriptor.slug, document_type="json", title="Meeting notice (fixture)",
                           source_url=self.descriptor.url, file_name=p.name, mime_type="application/json", content=c,
                           sha256=hashlib.sha256(c).hexdigest(), fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw):
        import json
        data = json.loads(raw.content.decode("utf-8"))
        if isinstance(data, dict) and "meetings" in data:
            return data["meetings"]
        return data if isinstance(data, list) else [data]

    def normalize(self, records):
        out = []
        for r in records:
            r = dict(r)
            r["symbol"] = clean_symbol(r.get("symbol", ""))
            r["meeting_type"] = r.get("meeting_type", "AGM")
            r["meeting_date"] = parse_date(r.get("meeting_date"))
            r["book_closure_date"] = parse_date(r.get("book_closure_date"))
            r["agenda"] = clean_text(r.get("agenda", ""))
            r["venue"] = clean_text(r.get("venue", ""))
            r["status"] = r.get("status", "announced")
            r["_valid"] = bool(r["symbol"] and r["meeting_type"] and r["meeting_date"])
            out.append(r)
        return out
