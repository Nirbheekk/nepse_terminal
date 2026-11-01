"""Integration tests for Batch 4.7 mutual fund pipelines (SQLite)."""

from nepse_ingestion.db.base import make_session_factory, Base
from nepse_ingestion.sources.mutual_fund_profile import MutualFundProfileAdapter
from nepse_ingestion.sources.mutual_fund_nav import MutualFundNavAdapter
from nepse_ingestion.sources.mutual_fund_holdings import MutualFundHoldingsAdapter
from nepse_ingestion.pipelines.mutual_fund_profile import run_mf_profile_pipeline
from nepse_ingestion.pipelines.mutual_fund_nav import run_mf_nav_pipeline
from nepse_ingestion.pipelines.mutual_fund_holdings import run_mf_holdings_pipeline


def test_mf_profile_pipeline(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    a = MutualFundProfileAdapter()
    raw = a.fetch_fixture("tests/fixtures/mf_profiles_sample.json")
    recs = a.normalize(a.parse(raw))
    s = sf(); r = run_mf_profile_pipeline(s, raw, recs); s.commit()
    assert r["rows_written"] == 2
    s.close()


def test_mf_nav_pipeline_idempotent(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    a = MutualFundNavAdapter()
    raw = a.fetch_fixture("tests/fixtures/mf_nav_sample.json")
    recs = a.normalize(a.parse(raw))
    s = sf(); r1 = run_mf_nav_pipeline(s, raw, recs); s.commit()
    assert r1["rows_written"] == 2
    s.close()
    s = sf(); r2 = run_mf_nav_pipeline(s, raw, recs); s.commit()
    assert r2["rows_written"] == 0
    assert r2["rows_unchanged"] == 2
    s.close()


def test_mf_holdings_pipeline(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    a = MutualFundHoldingsAdapter()
    raw = a.fetch_fixture("tests/fixtures/mf_holdings_sample.json")
    recs = a.normalize(a.parse(raw))
    s = sf(); r = run_mf_holdings_pipeline(s, raw, recs); s.commit()
    assert r["rows_written"] == 2
    s.close()
