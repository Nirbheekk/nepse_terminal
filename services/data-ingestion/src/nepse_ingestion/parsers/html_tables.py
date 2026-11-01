"""HTML table parsing utilities."""

from __future__ import annotations

from dataclasses import dataclass
from typing import List, Optional

from bs4 import BeautifulSoup


@dataclass
class Table:
    headers: List[str]
    rows: List[List[str]]
    caption: Optional[str] = None


def parse_html_tables(html: str) -> List[Table]:
    soup = BeautifulSoup(html, "lxml")
    tables: List[Table] = []
    for t in soup.find_all("table"):
        caption = None
        cap = t.find("caption")
        if cap is not None:
            caption = cap.get_text(strip=True)
        headers: List[str] = []
        thead = t.find("thead")
        if thead is not None:
            header_row = thead.find("tr")
            if header_row is not None:
                headers = [c.get_text(strip=True) for c in header_row.find_all(["th", "td"])]
        if not headers:
            first_row = t.find("tr")
            if first_row is not None and first_row.find("th") is not None:
                headers = [c.get_text(strip=True) for c in first_row.find_all(["th", "td"])]
        rows: List[List[str]] = []
        tbody = t.find("tbody") or t
        for tr in tbody.find_all("tr"):
            if tr.find("th") is not None and not tr.find("td"):
                continue
            cells = [c.get_text(strip=True) for c in tr.find_all(["td", "th"])]
            if not cells:
                continue
            if headers and cells == headers:
                continue
            rows.append(cells)
        tables.append(Table(headers=headers, rows=rows, caption=caption))
    return tables


def table_to_dicts(table: Table) -> List[dict]:
    if not table.headers:
        return [{"_row": i, "values": r} for i, r in enumerate(table.rows)]
    result = []
    for r in table.rows:
        d = {}
        for i, h in enumerate(table.headers):
            key = h if h else f"col_{i}"
            d[key] = r[i] if i < len(r) else ""
        result.append(d)
    return result
