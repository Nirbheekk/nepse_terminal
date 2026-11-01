"""Market calendar helpers."""

from __future__ import annotations

from datetime import date, datetime, time, timedelta, timezone
from typing import Optional

KATHMANDU = timezone(timedelta(hours=5, minutes=45))


def is_weekend(d: date) -> bool:
    # Nepal: Saturday is weekend (single day). Sunday-Friday trading.
    return d.weekday() == 5  # Saturday


def market_status(now: datetime, trading_date: date, is_trading_day: bool, last_close: Optional[datetime] = None) -> str:
    if not is_trading_day:
        return "HOLIDAY"
    # Simple: if weekend -> closed
    if is_weekend(trading_date):
        return "MARKET_CLOSED"
    # Nepal trading hours approx 11:00-15:00 NPT
    local = now.astimezone(KATHMANDU)
    if local.hour < 11:
        return "PRE_OPEN" if local.hour >= 10 and local.minute >= 45 else "MARKET_CLOSED"
    if local.hour >= 15:
        return "MARKET_CLOSED"
    return "MARKET_OPEN"


def freshness_status(last_observation: Optional[datetime], now: datetime, expected_minutes: int = 15) -> str:
    if last_observation is None:
        return "UNAVAILABLE"
    age = (now - last_observation).total_seconds() / 60.0
    if age <= expected_minutes:
        return "CURRENT"
    if age <= expected_minutes * 4:
        return "DELAYED"
    return "STALE"
