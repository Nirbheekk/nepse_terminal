"""OHLCV pipeline (Batch 4.8)."""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Dict, List

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models.ingestion import SourceDocument, IngestionJob, IngestionRun
from ..models.market_data import DailyOhlcvRecord


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


def run_ohlcv_pipeline(session, raw, records):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "daily_ohlcv")
    read = written = unchanged = rejected = 0
    for rec in records:
        read += 1
        if not rec.get("_valid", True):
            rejected += 1
            continue
        sid = rec.get("security_id")
        td = rec.get("trading_date")
        if not (sid and td):
            rejected += 1
            continue
        existing = session.execute(select(DailyOhlcvRecord).where(
            DailyOhlcvRecord.security_id == sid, DailyOhlcvRecord.trading_date == td)).scalar_one_or_none()
        payload = dict(security_id=sid, trading_date=td, open=rec.get("open"), high=rec.get("high"),
                       low=rec.get("low"), close=rec.get("close"), volume=rec.get("volume"),
                       turnover=rec.get("turnover"), transactions=rec.get("transactions"))
        if existing is None:
            session.add(DailyOhlcvRecord(**payload)); written += 1
        else:
            changed = any(getattr(existing, k) != v for k, v in payload.items() if k not in ("security_id", "trading_date"))
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
