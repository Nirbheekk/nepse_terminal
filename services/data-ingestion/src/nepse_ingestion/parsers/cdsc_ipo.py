"""CDSC IPO list HTML parser (fixture-backed)."""

from __future__ import annotations

from datetime import date
from typing import List, Dict, Any, Optional

from .html_tables import parse_html_tables, table_to_dicts
from ..normalizers.numbers import parse_decimal, clean_symbol, clean_text


_COLUMN_MAP = {
    "symbol": ["symbol", "scrip", "company symbol"],
    "company": ["company", "company name", "issuer"],
    "issue_type": ["issue type", "type"],
    "open_date": ["open date", "opening date", "issue open"],
    "close_date": ["close date", "closing date", "issue close"],
    "allotment_date": ["allotment date", "allotment"],
    "listing_date": ["listing date", "listing"],
    "issue_price": ["issue price", "price", "par value"],
    "units_offered": ["units", "units offered", "quantity", "shares"],
    "issue_size": ["issue size", "amount", "total amount"],
}


def _find_col(headers: List[str], candidates: List[str]) -> Optional[int]:
    low = [h.lower().strip() for h in headers]
    for cand in candidates:
        for i, h in enumerate(low):
            if cand in h:
                return i
    return None


def parse_cdsc_ipolist_html(html: str) -> List[Dict[str, Any]]:
    tables = parse_html_tables(html)
    records: List[Dict[str, Any]] = []
    for t in tables:
        if not t.rows:
            continue
        # find header row index
        headers = t.headers
        if not headers and t.rows:
            headers = t.rows[0]
            data_rows = t.rows[1:]
        else:
            data_rows = t.rows
        # skip tables without a symbol/company column
        sym_idx = _find_col(headers, _COLUMN_MAP["symbol"]) 
        comp_idx = _find_col(headers, _COLUMN_MAP["company"])
        if sym_idx is None and comp_idx is None:
            continue
        # column indices
        idx = {k: _find_col(headers, v) for k, v in _COLUMN_MAP.items()}
        for r in data_rows:
            def cell(key):
                i = idx.get(key)
                if i is None or i >= len(r):
                    return ""
                return r[i].strip()
            symbol = clean_symbol(cell("symbol"))
            company = clean_text(cell("company"))
            # If no symbol but company present, derive nothing; keep blank for review.
            issue_type_raw = clean_text(cell("issue_type")).upper()
            issue_type = issue_type_raw if issue_type_raw in {"IPO", "FPO", "RIGHTS", "BONUS"} else "IPO"
            rec = {
                "source": "cdsc-ipolist",
                "symbol": symbol,
                "company_name": company,
                "issue_type": issue_type,
                "open_date": _parse_date(cell("open_date")),
                "close_date": _parse_date(cell("close_date")),
                "allotment_date": _parse_date(cell("allotment_date")),
                "listing_date": _parse_date(cell("listing_date")),
                "issue_price": parse_decimal(cell("issue_price")),
                "units_offered": parse_decimal(cell("units_offered")),
                "issue_size": parse_decimal(cell("issue_size")),
                "issue_name": company,
                "status": "announced",
            }
            if not symbol and not company:
                continue
            records.append(rec)
    return records


def _parse_date(value: str) -> Optional[date]:
    if not value:
        return None
    value = value.strip()
    for fmt in ("%Y-%m-%d", "%d-%m-%Y", "%d/%m/%Y", "%m/%d/%Y", "%Y/%m/%d"):
        try:
            from datetime import datetime as _dt
            return _dt.strptime(value, fmt).date()
        except ValueError:
            continue
    return None
