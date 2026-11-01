"""Fundamental ingestion pipelines (Batch 4.4)."""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Dict, List

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models.fundamentals import CompanyProfileRecord, CompanyDirectorRecord
from ..models.ingestion import SourceDocument, IngestionJob, IngestionRun


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


def run_company_profile_pipeline(session: Session, raw, records: List[Dict[str, Any]]):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "company_profiles")
    read = written = unchanged = rejected = 0
    for rec in records:
        read += 1
        if not rec.get("_valid", True):
            rejected += 1
            continue
        symbol = rec.get("symbol", "")
        if not symbol:
            rejected += 1
            continue
        existing = session.execute(select(CompanyProfileRecord).where(CompanyProfileRecord.symbol == symbol)).scalar_one_or_none()
        payload = dict(symbol=symbol, legal_name=rec.get("company_name") or rec.get("legal_name", ""),
                       sector=rec.get("sector", ""), sub_sector=rec.get("sub_sector", ""),
                       website=rec.get("website", ""), paid_up_capital=rec.get("paid_up_capital"),
                       listed_shares=rec.get("listed_shares"), fiscal_year_end=rec.get("fiscal_year_end", ""),
                       status=rec.get("status", "active"), source_document_id=sd.id)
        if existing is None:
            session.add(CompanyProfileRecord(**payload)); written += 1
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


def run_board_pipeline(session: Session, raw, records: List[Dict[str, Any]]):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "company_directors")
    read = written = unchanged = rejected = 0
    for rec in records:
        read += 1
        if not rec.get("_valid", True):
            rejected += 1
            continue
        symbol = rec.get("symbol", "")
        name = rec.get("full_name", "")
        if not (symbol and name):
            rejected += 1
            continue
        existing = session.execute(select(CompanyDirectorRecord).where(
            CompanyDirectorRecord.symbol == symbol, CompanyDirectorRecord.full_name == name,
            CompanyDirectorRecord.role == rec.get("role", ""))).scalar_one_or_none()
        payload = dict(symbol=symbol, full_name=name, role=rec.get("role", ""),
                       designation=rec.get("designation", ""), appointment_date=rec.get("appointment_date"),
                       cessation_date=rec.get("cessation_date"), is_independent=rec.get("is_independent"),
                       source_document_id=sd.id)
        if existing is None:
            session.add(CompanyDirectorRecord(**payload)); written += 1
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
from ..models.fundamentals import OwnershipSnapshotRecord, FinancialReportDocumentRecord


def run_ownership_pipeline(session: Session, raw, records: List[Dict[str, Any]]):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "ownership_snapshots")
    read = written = unchanged = rejected = 0
    for rec in records:
        read += 1
        if not rec.get("_valid", True):
            rejected += 1
            continue
        symbol = rec.get("symbol", "")
        eff = rec.get("effective_date")
        if not (symbol and eff):
            rejected += 1
            continue
        existing = session.execute(select(OwnershipSnapshotRecord).where(
            OwnershipSnapshotRecord.symbol == symbol, OwnershipSnapshotRecord.effective_date == eff)).scalar_one_or_none()
        payload = dict(symbol=symbol, effective_date=eff,
                       promoter_percent=rec.get("promoter_percent"), public_percent=rec.get("public_percent"),
                       government_percent=rec.get("government_percent"),
                       institutional_percent=rec.get("institutional_percent"),
                       foreign_percent=rec.get("foreign_percent"), other_percent=rec.get("other_percent"),
                       status="PROVISIONAL", source_document_id=sd.id)
        if existing is None:
            session.add(OwnershipSnapshotRecord(**payload)); written += 1
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


def run_financial_document_pipeline(session: Session, raw, records: List[Dict[str, Any]]):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "financial_report_documents")
    read = written = 0
    for rec in records:
        read += 1
        session.add(FinancialReportDocumentRecord(
            file_name=rec.get("file_name", raw.file_name), file_url=rec.get("file_url", ""),
            mime_type=rec.get("mime_type", raw.mime_type), sha256=rec.get("sha256", raw.sha256),
            page_count=rec.get("page_count"), ocr_required=rec.get("ocr_required", False)))
        written += 1
    sd.status = "validated"
    run.status = "success"; job.status = "success"
    run.rows_read = read; run.rows_written = written
    run.finished_at = datetime.now(timezone.utc); job.finished_at = datetime.now(timezone.utc)
    session.flush()
    return {"source_document_id": str(sd.id), "run_id": str(run.id), "rows_read": read, "rows_written": written, "rows_unchanged": 0, "rows_rejected": 0}

