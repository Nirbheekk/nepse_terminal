"""Validation helpers."""

from __future__ import annotations

from datetime import date
from typing import List, Tuple

from ..normalizers.numbers import parse_decimal


def validate_public_issue(rec: dict) -> Tuple[bool, List[str]]:
    errors: List[str] = []
    if not rec.get("symbol"):
        errors.append("missing symbol")
    if not rec.get("issue_type"):
        errors.append("missing issue_type")
    # date ordering
    od = rec.get("open_date")
    cd = rec.get("close_date")
    if od and cd and od > cd:
        errors.append("open_date after close_date")
    # price non-negative
    price = parse_decimal(rec.get("issue_price"))
    if price is not None and price < 0:
        errors.append("negative issue_price")
    return (len(errors) == 0, errors)


def validate_nav(rec: dict) -> Tuple[bool, List[str]]:
    errors: List[str] = []
    if not rec.get("fund_code"):
        errors.append("missing fund_code")
    nav = parse_decimal(rec.get("nav_per_unit"))
    if nav is None:
        errors.append("missing nav_per_unit")
    elif nav <= 0:
        errors.append("invalid nav_per_unit")
    vd = rec.get("valuation_date")
    if not vd:
        errors.append("missing valuation_date")
    return (len(errors) == 0, errors)
