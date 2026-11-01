"""Merger and public issue pipelines (Batch 4.5)."""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Dict, List

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models.ingestion import SourceDocument, IngestionJob, IngestionRun
from ..models.corporate_actions import MergerEventRecord, MergerParticipantRecord
from ..models.public_issue import PublicIssue


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


def run_merger_pipeline(session, raw, records):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "merger_events")
    read = written = unchanged = rejected = 0
    for rec in records:
        read += 1
        if not rec.get("_valid", True):
            rejected += 1
            continue
        event_name = rec.get("event_name", "")
        if not event_name:
            rejected += 1
            continue
        existing = session.execute(select(MergerEventRecord).where(MergerEventRecord.event_name == event_name)).scalar_one_or_none()
        if existing is None:
            me = MergerEventRecord(event_name=event_name, event_type=rec.get("event_type", "merger"),
                                   announcement_date=rec.get("announcement_date"), effective_date=rec.get("effective_date"),
                                   status=rec.get("status", "announced"), surviving_symbol=rec.get("surviving_symbol", ""),
                                   swap_ratio=rec.get("swap_ratio"), source_document_id=sd.id)
            session.add(me)
            session.flush()
            for part in rec.get("participants", []):
                session.add(MergerParticipantRecord(merger_event_id=me.id, symbol=part.get("symbol", ""), role=part.get("role", "target")))
            written += 1
        else:
            unchanged += 1
    sd.status = "validated"
    run.status = "success"; job.status = "success"
    run.rows_read = read; run.rows_written = written; run.rows_unchanged = unchanged; run.rows_rejected = rejected
    run.finished_at = datetime.now(timezone.utc); job.finished_at = datetime.now(timezone.utc)
    session.flush()
    return {"source_document_id": str(sd.id), "run_id": str(run.id), "rows_read": read, "rows_written": written, "rows_unchanged": unchanged, "rows_rejected": rejected}


def run_public_issue_pipeline(session, raw, records):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "public_issues")
    read = written = unchanged = rejected = 0
    for rec in records:
        read += 1
        symbol = rec.get("symbol", "")
        issue_type = rec.get("issue_type", "IPO")
        open_date = rec.get("open_date")
        if not symbol:
            rejected += 1
            continue
        existing = session.execute(select(PublicIssue).where(
            PublicIssue.symbol == symbol, PublicIssue.issue_type == issue_type, PublicIssue.open_date == open_date)).scalar_one_or_none()
        payload = dict(symbol=symbol, issue_type=issue_type, issue_name=rec.get("issue_name", ""),
                       open_date=open_date, close_date=rec.get("close_date"), allotment_date=rec.get("allotment_date"),
                       listing_date=rec.get("listing_date"), issue_price=rec.get("issue_price"),
                       units_offered=rec.get("units_offered"), issue_size=rec.get("issue_size"),
                       status=rec.get("status", "announced"), source_document_id=sd.id, source_url=raw.source_url)
        if existing is None:
            session.add(PublicIssue(**payload)); written += 1
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
