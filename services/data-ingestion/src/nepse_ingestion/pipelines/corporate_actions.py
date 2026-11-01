"""Dividend/capital-action and meeting pipelines (Batch 4.5)."""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Dict, List

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models.ingestion import SourceDocument, IngestionJob, IngestionRun
from ..models.corporate_actions import CorporateActionRecord, CompanyMeetingRecord


def _source_doc(session, raw):
    sd = SourceDocument(source_slug=raw.source_slug, document_type=raw.document_type, title=raw.title,
                        source_url=raw.source_url, file_name=raw.file_name, mime_type=raw.mime_type,
                        sha256=raw.sha256, size_bytes=len(raw.content), license_class="public_unverified",
                        redistribution="prohibited", status="captured", meta=dict(raw.meta or {}))
    session.add(sd)
    session.flush()
    return sd


def _run(session, source_slug, stream):
    job = IngestionJob(job_name=f"{stream}-import", source_slug=source_slug, status="running")
    session.add(job)
    session.flush()
    run = IngestionRun(job_id=job.id, source_slug=source_slug, stream=stream, status="partial")
    session.add(run)
    session.flush()
    return job, run


def run_dividend_capital_pipeline(session, raw, records):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "corporate_actions")
    read = written = unchanged = rejected = 0
    for rec in records:
        read += 1
        if not rec.get("_valid", True):
            rejected += 1
            continue
        symbol = rec.get("symbol", "")
        action_type = rec.get("action_type", "cash_dividend")
        fiscal_year = rec.get("fiscal_year", "")
        bcd = rec.get("book_closure_date")
        if not (symbol and action_type and fiscal_year and bcd):
            rejected += 1
            continue
        existing = session.execute(select(CorporateActionRecord).where(
            CorporateActionRecord.symbol == symbol, CorporateActionRecord.action_type == action_type,
            CorporateActionRecord.fiscal_year == fiscal_year, CorporateActionRecord.book_closure_date == bcd)).scalar_one_or_none()
        payload = dict(symbol=symbol, action_type=action_type, fiscal_year=fiscal_year,
                       announcement_date=rec.get("announcement_date"), book_closure_date=bcd,
                       agm_date=rec.get("agm_date"), effective_date=rec.get("effective_date"),
                       cash_percent=rec.get("cash_percent", 0) or 0, cash_amount=rec.get("cash_amount", 0) or 0,
                       bonus_percent=rec.get("bonus_percent", 0) or 0, rights_ratio=rec.get("rights_ratio", 0) or 0,
                       rights_price=rec.get("rights_price", 0) or 0, split_from=rec.get("split_from", 0) or 0,
                       split_to=rec.get("split_to", 0) or 0, status=rec.get("status", "announced"),
                       notes=rec.get("notes", ""), source_document_id=sd.id)
        if existing is None:
            session.add(CorporateActionRecord(**payload)); written += 1
        else:
            changed = any(getattr(existing, k) != v for k, v in payload.items() if k != "source_document_id")
            if changed:
                for k, v in payload.items():
                    setattr(existing, k, v)
                written += 1
            else:
                unchanged += 1
    sd.status = "validated"
    run.status = "success"; job.status = "success"
    run.rows_read = read; run.rows_written = written; run.rows_unchanged = unchanged; run.rows_rejected = rejected
    run.finished_at = datetime.now(timezone.utc); job.finished_at = datetime.now(timezone.utc)
    session.flush()
    return {"source_document_id": str(sd.id), "run_id": str(run.id), "rows_read": read, "rows_written": written, "rows_unchanged": unchanged, "rows_rejected": rejected}


def run_meeting_pipeline(session, raw, records):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "company_meetings")
    read = written = unchanged = rejected = 0
    for rec in records:
        read += 1
        if not rec.get("_valid", True):
            rejected += 1
            continue
        symbol = rec.get("symbol", "")
        mtype = rec.get("meeting_type", "AGM")
        mdate = rec.get("meeting_date")
        if not (symbol and mtype and mdate):
            rejected += 1
            continue
        existing = session.execute(select(CompanyMeetingRecord).where(
            CompanyMeetingRecord.symbol == symbol, CompanyMeetingRecord.meeting_type == mtype,
            CompanyMeetingRecord.meeting_date == mdate)).scalar_one_or_none()
        payload = dict(symbol=symbol, meeting_type=mtype, meeting_date=mdate,
                       book_closure_date=rec.get("book_closure_date"), agenda=rec.get("agenda", ""),
                       venue=rec.get("venue", ""), status=rec.get("status", "announced"), source_document_id=sd.id)
        if existing is None:
            session.add(CompanyMeetingRecord(**payload)); written += 1
        else:
            changed = any(getattr(existing, k) != v for k, v in payload.items() if k != "source_document_id")
            if changed:
                for k, v in payload.items():
                    setattr(existing, k, v)
                written += 1
            else:
                unchanged += 1
    sd.status = "validated"
    run.status = "success"; job.status = "success"
    run.rows_read = read; run.rows_written = written; run.rows_unchanged = unchanged; run.rows_rejected = rejected
    run.finished_at = datetime.now(timezone.utc); job.finished_at = datetime.now(timezone.utc)
    session.flush()
    return {"source_document_id": str(sd.id), "run_id": str(run.id), "rows_read": read, "rows_written": written, "rows_unchanged": unchanged, "rows_rejected": rejected}
