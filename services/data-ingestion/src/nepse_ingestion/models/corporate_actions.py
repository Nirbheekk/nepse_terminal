"""Corporate action ORM models (Batch 4.5)."""

from __future__ import annotations

import uuid
from datetime import date, datetime
from typing import Optional

from sqlalchemy import Date, DateTime, Numeric, String, Text, Uuid, func
from sqlalchemy.orm import Mapped, mapped_column

from ..db.base import Base


class CorporateActionRecord(Base):
    __tablename__ = "corporate_actions"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    symbol: Mapped[str] = mapped_column(String, nullable=False)
    action_type: Mapped[str] = mapped_column(String, nullable=False)
    fiscal_year: Mapped[str] = mapped_column(String, nullable=False)
    announcement_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    book_closure_date: Mapped[date] = mapped_column(Date, nullable=False)
    agm_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    effective_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    cash_percent: Mapped[float] = mapped_column(Numeric(10, 4), default=0)
    cash_amount: Mapped[float] = mapped_column(Numeric(18, 4), default=0)
    bonus_percent: Mapped[float] = mapped_column(Numeric(10, 4), default=0)
    rights_ratio: Mapped[float] = mapped_column(Numeric(14, 6), default=0)
    rights_price: Mapped[float] = mapped_column(Numeric(18, 4), default=0)
    split_from: Mapped[float] = mapped_column(Numeric(14, 6), default=0)
    split_to: Mapped[float] = mapped_column(Numeric(14, 6), default=0)
    status: Mapped[str] = mapped_column(String, default="announced")
    notes: Mapped[str] = mapped_column(Text, default="")
    source_document_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class CompanyMeetingRecord(Base):
    __tablename__ = "company_meetings"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    symbol: Mapped[str] = mapped_column(String, nullable=False)
    meeting_type: Mapped[str] = mapped_column(String, nullable=False)
    meeting_date: Mapped[date] = mapped_column(Date, nullable=False)
    book_closure_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    agenda: Mapped[str] = mapped_column(Text, default="")
    venue: Mapped[str] = mapped_column(String, default="")
    status: Mapped[str] = mapped_column(String, default="announced")
    source_document_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class MergerEventRecord(Base):
    __tablename__ = "merger_events"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    event_name: Mapped[str] = mapped_column(String, nullable=False)
    event_type: Mapped[str] = mapped_column(String, nullable=False)
    announcement_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    effective_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    status: Mapped[str] = mapped_column(String, default="announced")
    surviving_symbol: Mapped[str] = mapped_column(String, default="")
    swap_ratio: Mapped[Optional[float]] = mapped_column(Numeric(18, 6), nullable=True)
    source_document_id: Mapped[Optional[str]] = mapped_column(Uuid(as_uuid=False), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())


class MergerParticipantRecord(Base):
    __tablename__ = "merger_participants"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=lambda: str(uuid.uuid4()))
    merger_event_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), nullable=False)
    symbol: Mapped[str] = mapped_column(String, nullable=False)
    role: Mapped[str] = mapped_column(String, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
