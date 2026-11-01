"""Integration test for financial report pipeline (SQLite)."""

from nepse_ingestion.db.base import make_session_factory, Base
from nepse_ingestion.sources.financial_report import FinancialReportAdapter
from nepse_ingestion.pipelines.financial_statements import run_financial_report_pipeline


def test_financial_report_pipeline(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    a = FinancialReportAdapter()
    raw = a.fetch_fixture("tests/fixtures/financial_report_sample.json")
    recs = a.normalize(a.parse(raw))
    s = sf()
    r = run_financial_report_pipeline(s, raw, recs)
    s.commit()
    assert r["rows_written"] == 1
    s.close()
