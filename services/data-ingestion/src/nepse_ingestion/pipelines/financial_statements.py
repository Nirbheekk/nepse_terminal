"""Financial statement pipelines (Batch 4.6)."""

from __future__ import annotations

from datetime import datetime, timezone
from decimal import Decimal
from typing import Any, Dict, List

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models.ingestion import SourceDocument, IngestionJob, IngestionRun
from ..models.financial import FinancialReportRecord, FinancialFactRecord, FinancialRatioRecord
from ..normalizers.ratios import eps, book_value_per_share, pe_ratio, pb_ratio, roe, roa, net_profit_margin, debt_to_equity, current_ratio
from ..normalizers.financial import map_label


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


def run_financial_report_pipeline(session, raw, records):
    sd = _source_doc(session, raw)
    job, run = _run(session, raw.source_slug, "financial_reports")
    read = written = unchanged = rejected = 0
    for rec in records:
        read += 1
        if not rec.get("_valid", True):
            rejected += 1
            continue
        symbol = rec.get("symbol", "")
        fiscal_year = rec.get("fiscal_year", "")
        period_type = rec.get("period_type", "Q1")
        period_start = rec.get("period_start")
        period_end = rec.get("period_end")
        filing_date = rec.get("filing_date")
        if not (symbol and fiscal_year and period_start and period_end and filing_date):
            rejected += 1
            continue
        existing = session.execute(select(FinancialReportRecord).where(
            FinancialReportRecord.symbol == symbol, FinancialReportRecord.fiscal_year == fiscal_year,
            FinancialReportRecord.period_type == period_type, FinancialReportRecord.audit_status == rec.get("audit_status", "unaudited"),
            FinancialReportRecord.scope == rec.get("scope", "standalone"))).scalar_one_or_none()
        if existing is None:
            fr = FinancialReportRecord(symbol=symbol, report_type=rec.get("report_type", "quarterly"),
                                     fiscal_year=fiscal_year, period_type=period_type,
                                     period_start=period_start, period_end=period_end, filing_date=filing_date,
                                     publication_date=rec.get("publication_date"),
                                     audit_status=rec.get("audit_status", "unaudited"),
                                     scope=rec.get("scope", "standalone"), currency=rec.get("currency", "NPR"),
                                     original_units=rec.get("original_units", "NPR"),
                                     normalized_units=rec.get("normalized_units", "NPR"),
                                     source_document_id=sd.id, status="PROVISIONAL", revision_number=1,
                                     parser_version="1")
            session.add(fr)
            session.flush()
            # facts
            for f in rec.get("facts", []):
                session.add(FinancialFactRecord(financial_report_id=fr.id,
                                              statement_type=f.get("statement_type", "income_statement"),
                                              line_code=f.get("canonical") or f.get("label", ""),
                                              line_name=f.get("label", ""),
                                              original_label=f.get("label", ""),
                                              amount=f.get("amount"),
                                              unit=f.get("unit", "NPR"),
                                              normalized_amount=f.get("amount"),
                                              normalized_unit="NPR",
                                              is_calculated=False))
            written += 1
        else:
            unchanged += 1
    sd.status = "validated"
    run.status = "success"; job.status = "success"
    run.rows_read = read; run.rows_written = written; run.rows_unchanged = unchanged; run.rows_rejected = rejected
    run.finished_at = datetime.now(timezone.utc); job.finished_at = datetime.now(timezone.utc)
    session.flush()
    return {"source_document_id": str(sd.id), "run_id": str(run.id), "rows_read": read, "rows_written": written, "rows_unchanged": unchanged, "rows_rejected": rejected}
