"""Structured data parsers for CSV, Excel and JSON."""

from __future__ import annotations

import csv
import json
from pathlib import Path
from typing import Any, List

import pandas as pd


def parse_csv(path: str | Path) -> List[dict]:
    with open(path, newline="", encoding="utf-8-sig") as f:
        reader = csv.DictReader(f)
        return [dict(r) for r in reader]


def parse_excel(path: str | Path, sheet: str | int | None = None) -> List[dict]:
    df = pd.read_excel(path, sheet_name=sheet if sheet is not None else 0)
    if isinstance(df, dict):
        # multiple sheets
        out: List[dict] = []
        for k, v in df.items():
            for r in v.to_dict(orient="records"):
                r["_sheet"] = k
                out.append(r)
        return out
    return df.to_dict(orient="records")


def parse_json(path: str | Path) -> Any:
    with open(path, encoding="utf-8") as f:
        return json.load(f)
