"""Fundamental ORM models (Batch 4.4)."""

from __future__ import annotations

import uuid
from datetime import date, datetime
from typing import Optional

from sqlalchemy import Boolean, Date, DateTime, Numeric, String, Text, Uuid, func
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from ..db.base import Base


class CompanyProfileRecord(Base):
    __tablename__ = "company_profiles"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    symbol: Mapped[str] = mapped_column(String, nullable=False)
    legal_name: Mapped[str] = mapped_column(String, default="")
    sector: Mapped[str] = mapped_column(String, default="")
    sub_sector: Mapped[str] = mapped_column(String, default="")
    website: Mapped[str] = mapped_column(String, default="")
    paid_up_capital: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    listed_shares: Mapped[Optional[float]] = mapped_column(Numeric(24, 4), nullable=True)
    incorporation_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    listed_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    fiscal_year_end: Mapped[str] = mapped_column(String, default="")
    status: Mapped[str] = mapped_column(String, default="active")
    effective_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    source_document_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class CompanyDirectorRecord(Base):
    __tablename__ = "company_directors"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    symbol: Mapped[str] = mapped_column(String, nullable=False)
    full_name: Mapped[str] = mapped_column(String, nullable=False)
    role: Mapped[str] = mapped_column(String, default="")
    designation: Mapped[str] = mapped_column(String, default="")
    appointment_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    cessation_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    is_independent: Mapped[Optional[bool]] = mapped_column(Boolean, nullable=True)
    source_document_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class OwnershipSnapshotRecord(Base):
    __tablename__ = "ownership_snapshots"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    symbol: Mapped[str] = mapped_column(String, nullable=False)
    effective_date: Mapped[date] = mapped_column(Date, nullable=False)
    promoter_percent: Mapped[Optional[float]] = mapped_column(Numeric(10, 4), nullable=True)
    public_percent: Mapped[Optional[float]] = mapped_column(Numeric(10, 4), nullable=True)
    government_percent: Mapped[Optional[float]] = mapped_column(Numeric(10, 4), nullable=True)
    institutional_percent: Mapped[Optional[float]] = mapped_column(Numeric(10, 4), nullable=True)
    foreign_percent: Mapped[Optional[float]] = mapped_column(Numeric(10, 4), nullable=True)
    other_percent: Mapped[Optional[float]] = mapped_column(Numeric(10, 4), nullable=True)
    status: Mapped[str] = mapped_column(String, default="PROVISIONAL")
    source_document_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class FinancialReportDocumentRecord(Base):
    __tablename__ = "financial_report_documents"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    file_name: Mapped[str] = mapped_column(String, default="")
    file_url: Mapped[str] = mapped_column(String, default="")
    mime_type: Mapped[str] = mapped_column(String, default="application/pdf")
    sha256: Mapped[Optional[str]] = mapped_column(String, nullable=True)
    page_count: Mapped[Optional[int]] = mapped_column(nullable=True)
    ocr_required: Mapped[bool] = mapped_column(Boolean, default=False)
    parser_confidence: Mapped[Optional[float]] = mapped_column(Numeric(5, 4), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
