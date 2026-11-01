"""Financial ratio engine (Decimal-based)."""

from __future__ import annotations

from decimal import Decimal, InvalidOperation
from typing import Optional


def _d(v) -> Optional[Decimal]:
    if v is None:
        return None
    if isinstance(v, Decimal):
        return v
    try:
        return Decimal(str(v))
    except (InvalidOperation, ValueError):
        return None


def eps(net_profit_attributable, shares_outstanding) -> Optional[Decimal]:
    np_ = _d(net_profit_attributable)
    sh = _d(shares_outstanding)
    if np_ is None or sh is None or sh == 0:
        return None
    return np_ / sh


def book_value_per_share(equity, shares_outstanding) -> Optional[Decimal]:
    eq = _d(equity)
    sh = _d(shares_outstanding)
    if eq is None or sh is None or sh == 0:
        return None
    return eq / sh


def pe_ratio(price, eps_value) -> Optional[Decimal]:
    p = _d(price)
    e = _d(eps_value)
    if p is None or e is None or e == 0:
        return None
    if e < 0:
        return None  # not meaningful
    return p / e


def pb_ratio(price, bvps) -> Optional[Decimal]:
    p = _d(price)
    b = _d(bvps)
    if p is None or b is None or b == 0:
        return None
    return p / b


def roe(net_profit, equity) -> Optional[Decimal]:
    np_ = _d(net_profit)
    eq = _d(equity)
    if np_ is None or eq is None or eq == 0:
        return None
    return np_ / eq


def roa(net_profit, total_assets) -> Optional[Decimal]:
    np_ = _d(net_profit)
    ta = _d(total_assets)
    if np_ is None or ta is None or ta == 0:
        return None
    return np_ / ta


def net_profit_margin(net_profit, revenue) -> Optional[Decimal]:
    np_ = _d(net_profit)
    rev = _d(revenue)
    if np_ is None or rev is None or rev == 0:
        return None
    return np_ / rev


def debt_to_equity(debt, equity) -> Optional[Decimal]:
    d = _d(debt)
    eq = _d(equity)
    if d is None or eq is None or eq == 0:
        return None
    return d / eq


def current_ratio(current_assets, current_liabilities) -> Optional[Decimal]:
    ca = _d(current_assets)
    cl = _d(current_liabilities)
    if ca is None or cl is None or cl == 0:
        return None
    return ca / cl


def dividend_payout_ratio(dividends, net_profit) -> Optional[Decimal]:
    dv = _d(dividends)
    np_ = _d(net_profit)
    if dv is None or np_ is None or np_ == 0:
        return None
    return dv / np_
