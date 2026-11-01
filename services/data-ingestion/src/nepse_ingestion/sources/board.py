"""Board member disclosure adapter (fixture-backed)."""

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
from ..normalizers.entity_resolution import resolve_symbol


class BoardDisclosureAdapter(SourceAdapter):
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
        return RawDocument(source_slug=self.descriptor.slug, document_type="json", title="Board disclosure (fixture)",
                           source_url=self.descriptor.url, file_name=p.name, mime_type="application/json",
                           content=content, sha256=hashlib.sha256(content).hexdigest(),
                           fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw: RawDocument) -> List[Dict[str, Any]]:
        import json
        data = json.loads(raw.content.decode("utf-8"))
        if isinstance(data, dict) and "board" in data:
            return data["board"]
        return data if isinstance(data, list) else [data]

    def normalize(self, records: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        out = []
        for r in records:
            r = dict(r)
            r["symbol"] = clean_symbol(r.get("symbol", ""))
            r["full_name"] = clean_text(r.get("full_name") or r.get("name", ""))
            r["role"] = clean_text(r.get("role") or r.get("position", ""))
            r["designation"] = clean_text(r.get("designation", ""))
            r["appointment_date"] = parse_date(r.get("appointment_date"))
            r["cessation_date"] = parse_date(r.get("cessation_date"))
            r["is_independent"] = r.get("is_independent")
            res = resolve_symbol(symbol=r["symbol"], company_name=r.get("company_name", ""))
            r["_resolution"] = res
            r["_valid"] = bool(r["symbol"] and r["full_name"])
            out.append(r)
        return out
