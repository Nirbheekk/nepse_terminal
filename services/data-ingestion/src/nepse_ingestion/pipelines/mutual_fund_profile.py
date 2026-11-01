"""Mutual fund profile pipeline (Batch 4.7)."""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Dict, List

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models.ingestion import SourceDocument, IngestionJob, IngestionRun
from ..models.mutual_fund import MutualFundRecord


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


def run_mf_profile_pipeline(session, raw, records):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "mutual_funds")
    read = written = unchanged = rejected = 0
    for rec in records:
        read += 1
        if not rec.get("_valid", True):
            rejected += 1
            continue
        code = rec.get("fund_code", "")
        if not code:
            rejected += 1
            continue
        existing = session.execute(select(MutualFundRecord).where(MutualFundRecord.fund_code == code)).scalar_one_or_none()
        payload = dict(fund_code=code, fund_name=rec.get("fund_name", code), manager=rec.get("manager", ""),
                       fund_type=rec.get("fund_type", "open_ended"), currency=rec.get("currency", "NPR"),
                       status=rec.get("status", "active"))
        if existing is None:
            session.add(MutualFundRecord(**payload)); written += 1
        else:
            changed = any(getattr(existing, k) != v for k, v in payload.items())
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
