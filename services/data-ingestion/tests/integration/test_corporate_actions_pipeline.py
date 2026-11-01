"""Integration tests for Batch 4.5 pipelines (SQLite)."""

from nepse_ingestion.db.base import make_session_factory, Base
from nepse_ingestion.sources.dividends import DividendCapitalActionAdapter
from nepse_ingestion.sources.mergers import MergerDisclosureAdapter
from nepse_ingestion.sources.meetings import CompanyMeetingAdapter
from nepse_ingestion.sources.public_issues import PublicIssueAdapter
from nepse_ingestion.pipelines.corporate_actions import (
    run_dividend_capital_pipeline, run_meeting_pipeline,
)
from nepse_ingestion.pipelines.mergers import run_merger_pipeline, run_public_issue_pipeline


def test_dividend_pipeline_idempotent(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    a = DividendCapitalActionAdapter()
    raw = a.fetch_fixture("tests/fixtures/dividends_sample.json")
    recs = a.normalize(a.parse(raw))
    s = sf(); r1 = run_dividend_capital_pipeline(s, raw, recs); s.commit()
    assert r1["rows_written"] == 4
    s.close()
    s = sf(); r2 = run_dividend_capital_pipeline(s, raw, recs); s.commit()
    assert r2["rows_written"] == 0
    assert r2["rows_unchanged"] == 4
    s.close()


def test_merger_pipeline(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    a = MergerDisclosureAdapter()
    raw = a.fetch_fixture("tests/fixtures/mergers_sample.json")
    recs = a.normalize(a.parse(raw))
    s = sf(); r = run_merger_pipeline(s, raw, recs); s.commit()
    assert r["rows_written"] == 1
    s.close()


def test_meeting_pipeline(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    a = CompanyMeetingAdapter()
    raw = a.fetch_fixture("tests/fixtures/meetings_sample.json")
    recs = a.normalize(a.parse(raw))
    s = sf(); r = run_meeting_pipeline(s, raw, recs); s.commit()
    assert r["rows_written"] == 2
    s.close()


def test_public_issue_pipeline(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    a = PublicIssueAdapter()
    raw = a.fetch_fixture("tests/fixtures/public_issues_sample.json")
    recs = a.normalize(a.parse(raw))
    s = sf(); r = run_public_issue_pipeline(s, raw, recs); s.commit()
    assert r["rows_written"] == 2
    s.close()
