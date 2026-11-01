"""Financial report ORM models (Batch 4.6)."""

from __future__ import annotations

import uuid
from datetime import date, datetime
from typing import Optional

from sqlalchemy import Date, DateTime, Numeric, String, Text, Uuid, func
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from ..db.base import Base


class FinancialReportRecord(Base):
    __tablename__ = "financial_reports"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    symbol: Mapped[str] = mapped_column(String, nullable=False)
    report_type: Mapped[str] = mapped_column(String, nullable=False)
    fiscal_year: Mapped[str] = mapped_column(String, nullable=False)
    period_type: Mapped[str] = mapped_column(String, nullable=False)
    period_start: Mapped[date] = mapped_column(Date, nullable=False)
    period_end: Mapped[date] = mapped_column(Date, nullable=False)
    filing_date: Mapped[date] = mapped_column(Date, nullable=False)
    publication_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    audit_status: Mapped[str] = mapped_column(String, default="unaudited")
    scope: Mapped[str] = mapped_column(String, default="standalone")
    currency: Mapped[str] = mapped_column(String, default="NPR")
    original_units: Mapped[str] = mapped_column(String, default="NPR")
    normalized_units: Mapped[str] = mapped_column(String, default="NPR")
    source_document_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    status: Mapped[str] = mapped_column(String, default="PROVISIONAL")
    revision_number: Mapped[int] = mapped_column(default=1)
    parser_version: Mapped[str] = mapped_column(String, default="1")
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class FinancialFactRecord(Base):
    __tablename__ = "financial_statement_facts"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    financial_report_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), nullable=False)
    statement_type: Mapped[str] = mapped_column(String, nullable=False)
    line_code: Mapped[str] = mapped_column(String, nullable=False)
    line_name: Mapped[str] = mapped_column(String, nullable=False)
    original_label: Mapped[str] = mapped_column(String, default="")
    amount: Mapped[Optional[float]] = mapped_column(Numeric(28, 6), nullable=True)
    unit: Mapped[str] = mapped_column(String, default="NPR")
    normalized_amount: Mapped[Optional[float]] = mapped_column(Numeric(28, 6), nullable=True)
    normalized_unit: Mapped[str] = mapped_column(String, default="NPR")
    is_calculated: Mapped[bool] = mapped_column(default=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class FinancialRatioRecord(Base):
    __tablename__ = "financial_ratio_observations"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    financial_report_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    symbol: Mapped[str] = mapped_column(String, nullable=False)
    metric_code: Mapped[str] = mapped_column(String, nullable=False)
    as_of_date: Mapped[date] = mapped_column(Date, nullable=False)
    value: Mapped[Optional[float]] = mapped_column(Numeric(20, 6), nullable=True)
    currency: Mapped[str] = mapped_column(String, default="NPR")
    methodology: Mapped[str] = mapped_column(String, default="")
    input_lineage: Mapped[dict] = mapped_column(JSONB, default=dict)
    is_reported: Mapped[bool] = mapped_column(default=False)
    is_calculated: Mapped[bool] = mapped_column(default=False)
    status: Mapped[str] = mapped_column(String, default="PROVISIONAL")
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
