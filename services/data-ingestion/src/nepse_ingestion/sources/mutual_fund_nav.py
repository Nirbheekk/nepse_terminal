"""Mutual fund NAV adapter (fixture-backed)."""

from __future__ import annotations

import hashlib
from datetime import datetime, timezone
from decimal import Decimal
from pathlib import Path
from typing import Any, Dict, List

from ..config import settings
from ..sources.base import RawDocument, SourceAdapter, SourceDescriptor
from ..sources.registry import get_source
from ..normalizers.numbers import clean_text, parse_decimal
from ..normalizers.dates import parse_date


class MutualFundNavAdapter(SourceAdapter):
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
        return RawDocument(source_slug=self.descriptor.slug, document_type="json", title="MF NAV (fixture)",
                           source_url=self.descriptor.url, file_name=p.name, mime_type="application/json", content=c,
                           sha256=hashlib.sha256(c).hexdigest(), fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw):
        import json
        data = json.loads(raw.content.decode("utf-8"))
        if isinstance(data, dict) and "nav" in data:
            return data["nav"]
        return data if isinstance(data, list) else [data]

    def normalize(self, records):
        out = []
        for r in records:
            r = dict(r)
            r["fund_code"] = clean_text(r.get("fund_code", ""))
            r["valuation_date"] = parse_date(r.get("valuation_date"))
            r["publication_date"] = parse_date(r.get("publication_date"))
            r["nav_per_unit"] = parse_decimal(r.get("nav_per_unit"))
            r["total_nav"] = parse_decimal(r.get("total_nav"))
            r["units_outstanding"] = parse_decimal(r.get("units_outstanding"))
            r["currency"] = r.get("currency", "NPR")
            errs = []
            if r["nav_per_unit"] is None or r["nav_per_unit"] <= 0:
                errs.append("invalid nav_per_unit")
            if r["valuation_date"] is None:
                errs.append("missing valuation_date")
            if r["total_nav"] is not None and r["units_outstanding"] is not None and r["nav_per_unit"] is not None:
                calc = r["units_outstanding"] * r["nav_per_unit"]
                if abs(calc - r["total_nav"]) > (r["total_nav"] * Decimal("0.01") if r["total_nav"] else Decimal("0")):
                    errs.append("nav/units total mismatch")
            r["_errors"] = errs
            r["_valid"] = len(errs) == 0 and bool(r["fund_code"])
            out.append(r)
        return out
