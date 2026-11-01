"""Mutual fund performance calculations (Decimal)."""

from __future__ import annotations

from decimal import Decimal
from typing import Optional

from .ratios import _d


def nav_return(begin_nav, end_nav) -> Optional[Decimal]:
    b = _d(begin_nav); e = _d(end_nav)
    if b is None or e is None or b == 0:
        return None
    return e / b - Decimal("1")


def premium_discount(market_price, nav) -> Optional[Decimal]:
    p = _d(market_price); n = _d(nav)
    if p is None or n is None or n == 0:
        return None
    return (p - n) / n * Decimal("100")


def annualized_return(total_return, days) -> Optional[Decimal]:
    tr = _d(total_return)
    if tr is None or days is None or days <= 0:
        return None
    if days < 30:
        return None  # too short, warn
    years = Decimal(days) / Decimal("365")
    try:
        return (Decimal("1") + tr) ** (Decimal("1") / years) - Decimal("1")
    except Exception:
        return None


def concentration_ratio(weights) -> Optional[Decimal]:
    vals = [_d(w) for w in (weights or [])]
    vals = [v for v in vals if v is not None]
    if not vals:
        return None
    return max(vals)
