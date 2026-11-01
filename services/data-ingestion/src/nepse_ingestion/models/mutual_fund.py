"""Mutual fund ORM models (Batch 4.7)."""

from __future__ import annotations

import uuid
from datetime import date, datetime
from typing import Optional

from sqlalchemy import Date, DateTime, Numeric, String, Uuid, func
from sqlalchemy.orm import Mapped, mapped_column

from ..db.base import Base


class MutualFundRecord(Base):
    __tablename__ = "mutual_funds"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    fund_code: Mapped[str] = mapped_column(String, unique=True, nullable=False)
    fund_name: Mapped[str] = mapped_column(String, nullable=False)
    manager: Mapped[str] = mapped_column(String, default="")
    fund_type: Mapped[str] = mapped_column(String, default="open_ended")
    currency: Mapped[str] = mapped_column(String, default="NPR")
    status: Mapped[str] = mapped_column(String, default="active")
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class MutualFundNavRecord(Base):
    __tablename__ = "mutual_fund_nav_history"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    mutual_fund_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), nullable=False)
    valuation_date: Mapped[date] = mapped_column(Date, nullable=False)
    publication_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    nav_per_unit: Mapped[float] = mapped_column(Numeric(18, 6), nullable=False)
    total_nav: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    units_outstanding: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    currency: Mapped[str] = mapped_column(String, default="NPR")
    status: Mapped[str] = mapped_column(String, default="PROVISIONAL")
    revision_number: Mapped[int] = mapped_column(default=1)
    source_document_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class MutualFundHoldingRecord(Base):
    __tablename__ = "mutual_fund_portfolio_disclosures"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    mutual_fund_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), nullable=False)
    as_of_date: Mapped[date] = mapped_column(Date, nullable=False)
    holding_symbol: Mapped[str] = mapped_column(String, default="")
    holding_name: Mapped[str] = mapped_column(String, default="")
    units: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    market_value: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    weight_percent: Mapped[Optional[float]] = mapped_column(Numeric(10, 4), nullable=True)
    source_document_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
