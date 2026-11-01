"""Integration tests for Batch 4.4 pipelines (SQLite)."""

from nepse_ingestion.db.base import make_session_factory, Base
from nepse_ingestion.sources.company_profile import CompanyProfileAdapter
from nepse_ingestion.sources.board import BoardDisclosureAdapter
from nepse_ingestion.sources.ownership import OwnershipDisclosureAdapter
from nepse_ingestion.sources.financial_document import IssuerFinancialReportAdapter
from nepse_ingestion.pipelines.fundamentals import (
    run_company_profile_pipeline, run_board_pipeline, run_ownership_pipeline, run_financial_document_pipeline,
)


def test_profile_pipeline_idempotent(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    adapter = CompanyProfileAdapter()
    raw = adapter.fetch_fixture("tests/fixtures/company_profiles_sample.json")
    recs = adapter.normalize(adapter.parse(raw))
    s = sf()
    r1 = run_company_profile_pipeline(s, raw, recs)
    s.commit()
    assert r1["rows_written"] == 2
    s.close()
    s = sf()
    r2 = run_company_profile_pipeline(s, raw, recs)
    s.commit()
    assert r2["rows_written"] == 0
    assert r2["rows_unchanged"] == 2
    s.close()


def test_board_pipeline(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    adapter = BoardDisclosureAdapter()
    raw = adapter.fetch_fixture("tests/fixtures/board_sample.json")
    recs = adapter.normalize(adapter.parse(raw))
    s = sf()
    r = run_board_pipeline(s, raw, recs)
    s.commit()
    assert r["rows_written"] == 2
    s.close()


def test_ownership_pipeline(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    adapter = OwnershipDisclosureAdapter()
    raw = adapter.fetch_fixture("tests/fixtures/ownership_sample.json")
    recs = adapter.normalize(adapter.parse(raw))
    s = sf()
    r = run_ownership_pipeline(s, raw, recs)
    s.commit()
    assert r["rows_written"] == 2
    s.close()


def test_financial_document_pipeline(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    adapter = IssuerFinancialReportAdapter()
    raw = adapter.fetch_fixture("tests/fixtures/financial_report_sample.bin")
    recs = adapter.normalize(adapter.parse(raw))
    s = sf()
    r = run_financial_document_pipeline(s, raw, recs)
    s.commit()
    assert r["rows_written"] == 1
    s.close()
