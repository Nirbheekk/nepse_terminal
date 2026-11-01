"""Daily OHLCV adapter (fixture-backed)."""

from __future__ import annotations

import uuid

import hashlib
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

from ..config import settings
from ..sources.base import RawDocument, SourceAdapter, SourceDescriptor
from ..sources.registry import get_source
from ..normalizers.numbers import parse_decimal, clean_symbol
from ..normalizers.dates import parse_date


class OhlcvAdapter(SourceAdapter):
    def __init__(self, fixture_path: str | None = None):
        d = get_source("nepe-official-market")
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
        return RawDocument(source_slug=self.descriptor.slug, document_type="json", title="OHLCV (fixture)",
                           source_url=self.descriptor.url, file_name=p.name, mime_type="application/json", content=c,
                           sha256=hashlib.sha256(c).hexdigest(), fetched_at=datetime.now(timezone.utc).isoformat(), meta={"fixture": True})

    def parse(self, raw):
        import json
        data = json.loads(raw.content.decode("utf-8"))
        if isinstance(data, dict) and "bars" in data:
            return data["bars"]
        return data if isinstance(data, list) else [data]

    def normalize(self, records):
        out = []
        for r in records:
            r = dict(r)
            r["symbol"] = clean_symbol(r.get("symbol", ""))
            r["trading_date"] = parse_date(r.get("trading_date"))
            r["open"] = parse_decimal(r.get("open"))
            r["high"] = parse_decimal(r.get("high"))
            r["low"] = parse_decimal(r.get("low"))
            r["close"] = parse_decimal(r.get("close"))
            r["volume"] = parse_decimal(r.get("volume"))
            r["turnover"] = parse_decimal(r.get("turnover"))
            r["security_id"] = str(uuid.uuid5(uuid.NAMESPACE_DNS, r["symbol"]))
            # validation
            errs = []
            o, h, l, c = r["open"], r["high"], r["low"], r["close"]
            if h is not None and o is not None and h < o: errs.append("high < open")
            if h is not None and c is not None and h < c: errs.append("high < close")
            if l is not None and o is not None and l > o: errs.append("low > open")
            if l is not None and c is not None and l > c: errs.append("low > close")
            if h is not None and l is not None and h < l: errs.append("high < low")
            if r["volume"] is not None and r["volume"] < 0: errs.append("negative volume")
            r["_errors"] = errs
            r["_valid"] = len(errs) == 0 and bool(r["symbol"]) and r["trading_date"] is not None
            out.append(r)
        return out
