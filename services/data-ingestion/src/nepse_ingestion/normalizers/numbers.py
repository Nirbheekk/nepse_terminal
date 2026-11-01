"""Normalization helpers for Nepali financial data."""

from __future__ import annotations

import re
from datetime import date
from decimal import Decimal, InvalidOperation
from typing import Optional


def parse_decimal(value: object) -> Optional[Decimal]:
    """Parse a financial number without converting missing to zero.

    Returns None for missing/blank/invalid values.
    """
    if value is None:
        return None
    if isinstance(value, Decimal):
        return value
    if isinstance(value, (int, float)):
        # Avoid float-derived precision loss by stringifying first.
        try:
            return Decimal(str(value))
        except (InvalidOperation, ValueError):
            return None
    text = str(value).strip()
    if text == "" or text.lower() in {"-", "n/a", "na", "nil", "none"}:
        return None
    # Remove thousands separators and currency symbols.
    text = text.replace(",", "").replace("Rs.", "").replace("Rs", "").strip()
    text = text.replace("रू", "").strip()
    # Parentheses denote negatives.
    negative = False
    if text.startswith("(") and text.endswith(")"):
        negative = True
        text = text[1:-1].strip()
    try:
        d = Decimal(text)
    except (InvalidOperation, ValueError):
        return None
    return -d if negative else d


def normalize_units(amount: Optional[Decimal], from_unit: str, to_unit: str = "NPR") -> Optional[Decimal]:
    """Convert between common Nepali reporting units. Returns None if unknown."""
    if amount is None:
        return None
    if from_unit == to_unit:
        return amount
    factors = {
        ("thousands", "NPR"): Decimal("1000"),
        ("lakhs", "NPR"): Decimal("100000"),
        ("crores", "NPR"): Decimal("10000000"),
        ("millions", "NPR"): Decimal("1000000"),
    }
    factor = factors.get((from_unit.lower(), to_unit.upper()))
    if factor is None:
        return None
    return amount * factor


_FISCAL_PERIOD_MAP = {
    "q1": "Q1",
    "first quarter": "Q1",
    "q2": "Q2",
    "second quarter": "Q2",
    "q3": "Q3",
    "third quarter": "Q3",
    "q4": "Q4",
    "fourth quarter": "Q4",
    "annual": "ANNUAL",
    "yearly": "ANNUAL",
    "fy": "ANNUAL",
}


def map_period_type(label: str) -> Optional[str]:
    if not label:
        return None
    key = label.strip().lower()
    if key in _FISCAL_PERIOD_MAP:
        return _FISCAL_PERIOD_MAP[key]
    for k, v in _FISCAL_PERIOD_MAP.items():
        if k in key:
            return v
    return None


def fiscal_year_from_dates(start: date, end: date) -> str:
    """Derive a fiscal year label from a period. Nepal fiscal year runs Shrawan-Asadh (mid-July to mid-July)."""
    # Nepal FY ends mid-July. If period end month >= 7, FY label is end_year/end_year+1.
    if end.month >= 7:
        return f"{end.year}/{end.year + 1}"
    return f"{end.year - 1}/{end.year}"


def clean_symbol(value: object) -> str:
    if value is None:
        return ""
    return str(value).strip().upper()


def clean_text(value: object) -> str:
    if value is None:
        return ""
    return " ".join(str(value).split())
