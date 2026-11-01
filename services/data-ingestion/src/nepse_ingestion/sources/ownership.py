"""Ownership disclosure adapter (fixture-backed)."""

from __future__ import annotations

import hashlib
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

from ..config import settings
from ..sources.base import RawDocument, SourceAdapter, SourceDescriptor
from ..sources.registry import get_source
from ..normalizers.numbers import parse_decimal, clean_symbol
from ..normalizers.dates import parse_date


class OwnershipDisclosureAdapter(SourceAdapter):
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
        return RawDocument(source_slug=self.descriptor.slug, document_type="json", title="Ownership disclosure (fixture)",
                           source_url=self.descriptor.url, file_name=p.name, mime_type="application/json",
                           content=content, sha256=hashlib.sha256(content).hexdigest(),
                           fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw: RawDocument) -> List[Dict[str, Any]]:
        import json
        data = json.loads(raw.content.decode("utf-8"))
        if isinstance(data, dict) and "ownership" in data:
            return data["ownership"]
        return data if isinstance(data, list) else [data]

    def normalize(self, records: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        out = []
        for r in records:
            r = dict(r)
            r["symbol"] = clean_symbol(r.get("symbol", ""))
            r["effective_date"] = parse_date(r.get("effective_date"))
            r["promoter_percent"] = parse_decimal(r.get("promoter_percent"))
            r["public_percent"] = parse_decimal(r.get("public_percent"))
            r["government_percent"] = parse_decimal(r.get("government_percent"))
            r["institutional_percent"] = parse_decimal(r.get("institutional_percent"))
            r["foreign_percent"] = parse_decimal(r.get("foreign_percent"))
            r["other_percent"] = parse_decimal(r.get("other_percent"))
            # validation
            errs = []
            for k in ["promoter_percent", "public_percent", "government_percent", "institutional_percent", "foreign_percent", "other_percent"]:
                v = r.get(k)
                if v is not None and (v < 0 or v > 100):
                    errs.append(f"{k} out of range")
            r["_errors"] = errs
            r["_valid"] = len(errs) == 0 and bool(r["symbol"])
            out.append(r)
        return out
