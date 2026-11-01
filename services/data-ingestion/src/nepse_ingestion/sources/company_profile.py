"""Company profile adapter (fixture-backed)."""

from __future__ import annotations

import hashlib
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

from ..config import settings
from ..sources.base import RawDocument, SourceAdapter, SourceDescriptor, SourceStatus
from ..sources.registry import get_source
from ..normalizers.numbers import parse_decimal, clean_symbol, clean_text
from ..normalizers.dates import parse_date
from ..normalizers.entity_resolution import resolve_symbol
from ..validators.records import validate_public_issue


class CompanyProfileAdapter(SourceAdapter):
    def __init__(self, source_slug: str = "issuer-investor-relations", fixture_path: str | None = None):
        desc = get_source(source_slug)
        if desc is None:
            desc = get_source("issuer-investor-relations")
        assert desc is not None
        self.descriptor = desc
        self.fixture_path = fixture_path

    def discover(self) -> List[Dict[str, Any]]:
        return [{"source": self.descriptor.slug, "status": self.descriptor.status.value}]

    def fetch(self) -> RawDocument:
        if not (self.descriptor.live_fetch_enabled or settings.live_fetch_enabled):
            raise RuntimeError("Live fetch disabled")
        raise NotImplementedError

    def fetch_fixture(self, path: str | Path) -> RawDocument:
        p = Path(path)
        content = p.read_bytes()
        return RawDocument(
            source_slug=self.descriptor.slug,
            document_type="html",
            title="Company profile (fixture)",
            source_url=self.descriptor.url,
            file_name=p.name,
            mime_type="text/html",
            content=content,
            sha256=hashlib.sha256(content).hexdigest(),
            fetched_at=datetime.now(timezone.utc).isoformat(),
            meta={"fixture": True},
        )

    def parse(self, raw: RawDocument) -> List[Dict[str, Any]]:
        import json
        text = raw.content.decode("utf-8", errors="replace")
        try:
            data = json.loads(text)
            if isinstance(data, list):
                return data
            if isinstance(data, dict) and "profiles" in data:
                return data["profiles"]
            return [data]
        except Exception:
            from ..parsers.html_tables import parse_html_tables, table_to_dicts
            tables = parse_html_tables(text)
            out = []
            for t in tables:
                out.extend(table_to_dicts(t))
            return out


    def normalize(self, records: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        out = []
        for r in records:
            r = dict(r)
            r["symbol"] = clean_symbol(r.get("symbol", ""))
            r["company_name"] = clean_text(r.get("company_name") or r.get("name", ""))
            r["sector"] = clean_text(r.get("sector", ""))
            r["website"] = clean_text(r.get("website", ""))
            r["paid_up_capital"] = r.get("paid_up_capital")
            r["listed_shares"] = r.get("listed_shares")
            r["incorporation_date"] = parse_date(r.get("incorporation_date"))
            r["listed_date"] = parse_date(r.get("listed_date"))
            r["fiscal_year_end"] = clean_text(r.get("fiscal_year_end", ""))
            res = resolve_symbol(symbol=r["symbol"], company_name=r["company_name"])
            r["_resolution"] = res
            r["_valid"] = bool(r["symbol"])
            out.append(r)
        return out
