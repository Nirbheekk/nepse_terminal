"""Financial statement normalization helpers."""

from __future__ import annotations

from decimal import Decimal

from typing import Dict, Optional

from .numbers import parse_decimal, normalize_units

# Canonical metric dictionary: original label (lower) -> canonical code
LABEL_MAP: Dict[str, str] = {
    "revenue": "revenue",
    "total revenue": "revenue",
    "interest income": "interest_income",
    "interest expense": "interest_expense",
    "operating income": "operating_income",
    "net profit": "net_profit",
    "net profit after tax": "net_profit",
    "profit after tax": "net_profit",
    "total assets": "total_assets",
    "total liabilities": "total_liabilities",
    "total equity": "total_equity",
    "shareholders equity": "total_equity",
    "paid up capital": "paid_up_capital",
    "paid-up capital": "paid_up_capital",
    "shares outstanding": "shares_outstanding",
    "outstanding shares": "shares_outstanding",
    "retained earnings": "retained_earnings",
    "operating cash flow": "operating_cash_flow",
    "investing cash flow": "investing_cash_flow",
    "financing cash flow": "financing_cash_flow",
    "cash and cash equivalents": "cash_and_equivalents",
    "cash and bank balances": "cash_and_equivalents",
    "earnings per share": "reported_eps",
    "eps": "reported_eps",
    "book value per share": "reported_book_value_per_share",
}


def map_label(label: str) -> Optional[str]:
    if not label:
        return None
    key = " ".join(label.lower().split())
    if key in LABEL_MAP:
        return LABEL_MAP[key]
    for k, v in LABEL_MAP.items():
        if k in key:
            return v
    return None


def validate_balance_sheet(total_assets, total_liabilities, total_equity, tolerance=Decimal("0")) -> bool:
    ta = parse_decimal(total_assets)
    tl = parse_decimal(total_liabilities)
    te = parse_decimal(total_equity)
    if ta is None or tl is None or te is None:
        return False
    diff = abs(ta - (tl + te))
    return diff <= tolerance
