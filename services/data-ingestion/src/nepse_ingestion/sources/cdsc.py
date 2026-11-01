"""CDSC IPO source adapter (fixture-backed)."""

from __future__ import annotations

import hashlib
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

from ..config import settings
from ..fetchers.http import HttpFetcher
from ..sources.base import RawDocument, SourceAdapter, SourceDescriptor, SourceStatus
from ..sources.registry import get_source
from ..parsers.cdsc_ipo import parse_cdsc_ipolist_html
from ..normalizers.numbers import clean_symbol
from ..validators.records import validate_public_issue


class CdscIpoAdapter(SourceAdapter):
    def __init__(self, fixture_path: str | None = None):
        desc = get_source("cdsc-ipolist")
        assert desc is not None
        self.descriptor = desc
        self.fixture_path = fixture_path

    def discover(self) -> List[Dict[str, Any]]:
        return [{
            "source": self.descriptor.slug,
            "url": self.descriptor.url,
            "status": self.descriptor.status.value,
            "live_fetch_enabled": self.descriptor.live_fetch_enabled or settings.live_fetch_enabled,
        }]

    def fetch(self) -> RawDocument:
        if not (self.descriptor.live_fetch_enabled or settings.live_fetch_enabled):
            raise RuntimeError("Live fetch disabled for cdsc-ipolist (permission unknown)")
        fetcher = HttpFetcher()
        res = fetcher.fetch(self.descriptor.url)
        return RawDocument(
            source_slug=self.descriptor.slug,
            document_type="html",
            title="CDSC IPO list",
            source_url=self.descriptor.url,
            file_name="cdsc_ipolist.html",
            mime_type=res.content_type or "text/html",
            content=res.content,
            sha256=res.sha256,
            fetched_at=datetime.now(timezone.utc).isoformat(),
        )

    def fetch_fixture(self, path: str | Path) -> RawDocument:
        p = Path(path)
        content = p.read_bytes()
        return RawDocument(
            source_slug=self.descriptor.slug,
            document_type="html",
            title="CDSC IPO list (fixture)",
            source_url=self.descriptor.url,
            file_name=p.name,
            mime_type="text/html",
            content=content,
            sha256=hashlib.sha256(content).hexdigest(),
            fetched_at=datetime.now(timezone.utc).isoformat(),
            meta={"fixture": True},
        )

    def parse(self, raw: RawDocument) -> List[Dict[str, Any]]:
        html = raw.content.decode("utf-8", errors="replace")
        return parse_cdsc_ipolist_html(html)

    def normalize(self, records: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        out = []
        for r in records:
            r = dict(r)
            r["symbol"] = clean_symbol(r.get("symbol", ""))
            # validate
            ok, errs = validate_public_issue(r)
            r["_valid"] = ok
            r["_errors"] = errs
            out.append(r)
        return out
