"""Public issue ORM model."""

from __future__ import annotations

import uuid

from datetime import date, datetime
from typing import Optional

from sqlalchemy import Date, DateTime, Numeric, String, Uuid, func
from sqlalchemy.orm import Mapped, mapped_column

from ..db.base import Base


class PublicIssue(Base):
    __tablename__ = "public_issues"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    company_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    symbol: Mapped[str] = mapped_column(String, nullable=False)
    issue_type: Mapped[str] = mapped_column(String, nullable=False)
    issue_name: Mapped[str] = mapped_column(String, default="")
    registrar: Mapped[str] = mapped_column(String, default="")
    issue_manager: Mapped[str] = mapped_column(String, default="")
    open_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    close_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    allotment_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    listing_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    issue_size: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    issue_price: Mapped[Optional[float]] = mapped_column(Numeric(18, 4), nullable=True)
    units_offered: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    minimum_units: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    status: Mapped[str] = mapped_column(String, default="announced")
    source_document_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    source_url: Mapped[str] = mapped_column(String, default="")
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
