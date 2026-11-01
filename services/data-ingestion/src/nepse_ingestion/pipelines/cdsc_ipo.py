"""CDSC IPO ingestion vertical slice."""

from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Dict, List

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models.company import Company
from ..models.ingestion import IngestionJob, IngestionRun
from ..models.public_issue import PublicIssue
from ..models.ingestion import SourceDocument, IngestionJob, IngestionRun
from ..normalizers.numbers import parse_decimal
from ..sources.cdsc import CdscIpoAdapter


@dataclass
class PipelineResult:
    source_document_id: str
    run_id: str
    rows_read: int = 0
    rows_written: int = 0
    rows_unchanged: int = 0
    rows_rejected: int = 0
    rejected: List[Dict[str, Any]] = field(default_factory=list)
    errors: List[str] = field(default_factory=list)


def run_cdsc_ipo_pipeline(session: Session, raw_doc, *, job_name: str = "cdsc-ipo-import") -> PipelineResult:
    adapter = CdscIpoAdapter()
    # 1. source document metadata
    sd = SourceDocument(
        source_slug=raw_doc.source_slug,
        document_type=raw_doc.document_type,
        title=raw_doc.title,
        source_url=raw_doc.source_url,
        file_name=raw_doc.file_name,
        mime_type=raw_doc.mime_type,
        sha256=raw_doc.sha256,
        size_bytes=len(raw_doc.content),
        fetched_at=datetime.fromisoformat(raw_doc.fetched_at.replace("Z", "+00:00")) if raw_doc.fetched_at else None,
        license_class="public_unverified",
        redistribution="prohibited",
        status="captured",
        meta=dict(raw_doc.meta or {}),
    )
    session.add(sd)
    session.flush()

    # idempotency: if same sha256 already processed, skip
    existing = session.execute(
        select(SourceDocument).where(
            SourceDocument.source_slug == raw_doc.source_slug,
            SourceDocument.sha256 == raw_doc.sha256,
            SourceDocument.id != sd.id,
        )
    ).scalar_one_or_none()
    if existing is not None:
        # duplicate document; remove the just-added and report no-op
        session.delete(sd)
        session.flush()
        run = IngestionRun(
            source_slug=raw_doc.source_slug,
            stream="public_issues",
            status="skipped",
            rows_read=0,
            rows_written=0,
            error="duplicate document sha256",
        )
        session.add(run)
        session.flush()
        return PipelineResult(source_document_id=str(existing.id), run_id=str(run.id))

    job = IngestionJob(job_name=job_name, source_slug=raw_doc.source_slug, status="running")
    session.add(job)
    session.flush()

    run = IngestionRun(job_id=job.id, source_slug=raw_doc.source_slug, stream="public_issues", status="partial")
    session.add(run)
    session.flush()

    result = PipelineResult(source_document_id=str(sd.id), run_id=str(run.id))

    try:
        records = adapter.parse(raw_doc)
        normalized = adapter.normalize(records)
        result.rows_read = len(normalized)

        for rec in normalized:
            if not rec.get("_valid", True):
                result.rows_rejected += 1
                result.rejected.append({"record": rec, "errors": rec.get("_errors", [])})
                continue
            symbol = rec.get("symbol") or ""
            issue_type = rec.get("issue_type") or "IPO"
            open_date = rec.get("open_date")

            # find existing
            stmt = select(PublicIssue).where(
                PublicIssue.symbol == symbol,
                PublicIssue.issue_type == issue_type,
                PublicIssue.open_date == open_date,
            )
            existing_issue = session.execute(stmt).scalar_one_or_none()

            payload = dict(
                symbol=symbol,
                issue_type=issue_type,
                issue_name=rec.get("issue_name") or rec.get("company_name") or "",
                open_date=open_date,
                close_date=rec.get("close_date"),
                allotment_date=rec.get("allotment_date"),
                listing_date=rec.get("listing_date"),
                issue_price=rec.get("issue_price"),
                units_offered=rec.get("units_offered"),
                issue_size=rec.get("issue_size"),
                status=rec.get("status", "announced"),
                source_document_id=sd.id,
                source_url=raw_doc.source_url,
            )

            if existing_issue is None:
                session.add(PublicIssue(**payload))
                result.rows_written += 1
            else:
                changed = False
                for k, v in payload.items():
                    if getattr(existing_issue, k) != v:
                        setattr(existing_issue, k, v)
                        changed = True
                if changed:
                    result.rows_written += 1
                else:
                    result.rows_unchanged += 1

        sd.status = "validated"
        run.status = "success"
        job.status = "success"
    except Exception as exc:  # pragma: no cover - defensive
        run.status = "failed"
        run.error = str(exc)
        job.status = "failed"
        result.errors.append(str(exc))
        raise
    finally:
        run.rows_read = result.rows_read
        run.rows_written = result.rows_written
        run.rows_unchanged = result.rows_unchanged
        run.rows_rejected = result.rows_rejected
        run.finished_at = datetime.now(timezone.utc)
        job.finished_at = datetime.now(timezone.utc)

    session.flush()
    return result
