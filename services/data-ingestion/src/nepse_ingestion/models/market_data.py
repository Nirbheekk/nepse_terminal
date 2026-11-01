"""Market data ORM models (Batch 4.8)."""

from __future__ import annotations

import uuid
from datetime import date, datetime
from typing import Optional

from sqlalchemy import Date, DateTime, Numeric, String, Uuid, func
from sqlalchemy.orm import Mapped, mapped_column

from ..db.base import Base


class DailyOhlcvRecord(Base):
    __tablename__ = "daily_ohlcv"

    security_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True)
    trading_date: Mapped[date] = mapped_column(Date, primary_key=True)
    open: Mapped[Optional[float]] = mapped_column(Numeric(18, 4), nullable=True)
    high: Mapped[Optional[float]] = mapped_column(Numeric(18, 4), nullable=True)
    low: Mapped[Optional[float]] = mapped_column(Numeric(18, 4), nullable=True)
    close: Mapped[Optional[float]] = mapped_column(Numeric(18, 4), nullable=True)
    volume: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    turnover: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    transactions: Mapped[Optional[int]] = mapped_column(nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
