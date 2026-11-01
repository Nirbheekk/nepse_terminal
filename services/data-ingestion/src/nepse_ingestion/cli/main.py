"""CLI for NEPSE ingestion."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from ..config import settings
from ..sources.registry import SOURCES, get_source
from ..fetchers.http import HttpFetcher
from ..pipelines.cdsc_ipo import run_cdsc_ipo_pipeline
from ..sources.cdsc import CdscIpoAdapter
from ..db.base import make_session_factory


def cmd_list_sources(args):
    for s in SOURCES:
        print(f"{s.slug:32} {s.status.value:20} {s.organization}")


def cmd_check_source(args):
    src = get_source(args.slug)
    if src is None:
        print("unknown source", file=sys.stderr)
        return 1
    print(json.dumps({
        "slug": src.slug,
        "status": src.status.value,
        "live_fetch_enabled": src.live_fetch_enabled or settings.live_fetch_enabled,
        "url": src.url,
    }, indent=2))
    return 0


def cmd_import_document(args):
    src_slug = args.source
    path = Path(args.path)
    if src_slug == "cdsc-ipolist":
        adapter = CdscIpoAdapter()
        raw = adapter.fetch_fixture(path)
    else:
        print("import not implemented for source", src_slug, file=sys.stderr)
        return 1
    sf, engine = make_session_factory(settings.test_database_url if args.use_test_db else settings.database_url)
    # create tables if sqlite
    from ..db.base import Base
    Base.metadata.create_all(engine)
    session = sf()
    try:
        result = run_cdsc_ipo_pipeline(session, raw)
        session.commit()
        print(json.dumps({
            "source_document_id": result.source_document_id,
            "run_id": result.run_id,
            "rows_read": result.rows_read,
            "rows_written": result.rows_written,
            "rows_unchanged": result.rows_unchanged,
            "rows_rejected": result.rows_rejected,
            "errors": result.errors,
        }, indent=2))
    except Exception as exc:
        session.rollback()
        print("FAILED:", exc, file=sys.stderr)
        return 1
    finally:
        session.close()
    return 0


def cmd_run_source(args):
    src = get_source(args.slug)
    if src is None:
        print("unknown source", file=sys.stderr)
        return 1
    if not (src.live_fetch_enabled or settings.live_fetch_enabled):
        print("live fetch disabled; use import-document with a fixture", file=sys.stderr)
        return 2
    print("live run not enabled in this build")
    return 0


def cmd_validate_records(args):
    print("validate-records: use import-document (validation runs during import)")
    return 0


def cmd_show_ingestion_status(args):
    sf, engine = make_session_factory(settings.test_database_url if args.use_test_db else settings.database_url)
    from sqlalchemy import select
    from ..models.ingestion import IngestionRun
    session = sf()
    try:
        from ..db.base import Base
        Base.metadata.create_all(engine)
        runs = session.execute(select(IngestionRun).order_by(IngestionRun.started_at.desc()).limit(20)).scalars().all()
        for r in runs:
            print(f"{r.id} {r.source_slug} {r.stream} {r.status} read={r.rows_read} written={r.rows_written}")
    finally:
        session.close()
    return 0


def cmd_retry_failed(args):
    print("retry-failed: re-run import-document with the same fixture")
    return 0


def cmd_report_coverage(args):
    sf, engine = make_session_factory(settings.test_database_url if args.use_test_db else settings.database_url)
    from sqlalchemy import select, func
    from ..models.public_issue import PublicIssue
    session = sf()
    try:
        from ..db.base import Base
        Base.metadata.create_all(engine)
        count = session.execute(select(func.count()).select_from(PublicIssue)).scalar_one()
        print(f"public_issues: {count}")
    finally:
        session.close()
    return 0


def main(argv=None):
    parser = argparse.ArgumentParser(prog="nepse-ingest")
    sub = parser.add_subparsers(dest="command", required=True)

    p = sub.add_parser("list-sources")
    p.set_defaults(func=cmd_list_sources)

    p = sub.add_parser("check-source")
    p.add_argument("slug")
    p.set_defaults(func=cmd_check_source)

    p = sub.add_parser("import-document")
    p.add_argument("--source", required=True)
    p.add_argument("--path", required=True)
    p.add_argument("--use-test-db", action="store_true")
    p.set_defaults(func=cmd_import_document)

    p = sub.add_parser("run-source")
    p.add_argument("slug")
    p.set_defaults(func=cmd_run_source)

    p = sub.add_parser("validate-records")
    p.set_defaults(func=cmd_validate_records)

    p = sub.add_parser("show-ingestion-status")
    p.add_argument("--use-test-db", action="store_true")
    p.set_defaults(func=cmd_show_ingestion_status)

    p = sub.add_parser("retry-failed")
    p.set_defaults(func=cmd_retry_failed)

    p = sub.add_parser("report-coverage")
    p.add_argument("--use-test-db", action="store_true")
    p.set_defaults(func=cmd_report_coverage)

    args = parser.parse_args(argv)
    rc = args.func(args)
    raise SystemExit(rc if rc is not None else 0)


if __name__ == "__main__":
    main()
