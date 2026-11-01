"""Mutual fund holdings pipeline (Batch 4.7)."""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Dict, List

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models.ingestion import SourceDocument, IngestionJob, IngestionRun
from ..models.mutual_fund import MutualFundRecord, MutualFundHoldingRecord


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


def _get_or_create_fund(session, rec):
    code = rec.get("fund_code", "")
    existing = session.execute(select(MutualFundRecord).where(MutualFundRecord.fund_code == code)).scalar_one_or_none()
    if existing is not None:
        return existing
    mf = MutualFundRecord(fund_code=code, fund_name=rec.get("fund_name", code),
                          manager=rec.get("manager", ""), fund_type=rec.get("fund_type", "open_ended"),
                          currency=rec.get("currency", "NPR"), status="active")
    session.add(mf)
    session.flush()
    return mf


def run_mf_holdings_pipeline(session, raw, records):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "mutual_fund_portfolio_disclosures")
    read = written = rejected = 0
    for rec in records:
        read += 1
        if not rec.get("_valid", True):
            rejected += 1
            continue
        code = rec.get("fund_code", "")
        ad = rec.get("as_of_date")
        if not (code and ad):
            rejected += 1
            continue
        mf = _get_or_create_fund(session, rec)
        session.add(MutualFundHoldingRecord(mutual_fund_id=mf.id, as_of_date=ad,
                                           holding_symbol=rec.get("holding_symbol", ""),
                                           holding_name=rec.get("holding_name", ""),
                                           units=rec.get("units"), market_value=rec.get("market_value"),
                                           weight_percent=rec.get("weight_percent"), source_document_id=sd.id))
        written += 1
    sd.status = "validated"
    run.status = "success"; job.status = "success"
    run.rows_read = read; run.rows_written = written; run.rows_rejected = rejected
    run.finished_at = datetime.now(timezone.utc); job.finished_at = datetime.now(timezone.utc)
    session.flush()
    return {"source_document_id": str(sd.id), "run_id": str(run.id), "rows_read": read, "rows_written": written, "rows_unchanged": 0, "rows_rejected": rejected}
