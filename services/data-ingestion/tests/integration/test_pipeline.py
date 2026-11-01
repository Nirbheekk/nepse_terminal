"""Integration test for CDSC IPO pipeline idempotency (SQLite)."""

import os
from pathlib import Path

from nepse_ingestion.db.base import make_session_factory, Base
from nepse_ingestion.sources.cdsc import CdscIpoAdapter
from nepse_ingestion.pipelines.cdsc_ipo import run_cdsc_ipo_pipeline


def test_pipeline_idempotent(tmp_path):
    db_url = f"sqlite:///{tmp_path}/test.db"
    sf, engine = make_session_factory(db_url)
    Base.metadata.create_all(engine)
    adapter = CdscIpoAdapter()
    raw = adapter.fetch_fixture("tests/fixtures/cdsc_ipolist_sample.html")
    session = sf()
    r1 = run_cdsc_ipo_pipeline(session, raw)
    session.commit()
    assert r1.rows_written == 3
    session.close()

    session = sf()
    r2 = run_cdsc_ipo_pipeline(session, raw)
    session.commit()
    # duplicate document => skipped, 0 rows
    assert r2.rows_read == 0
    session.close()
