"""Integration test for OHLCV pipeline (SQLite)."""

from nepse_ingestion.db.base import make_session_factory, Base
from nepse_ingestion.sources.ohlcv import OhlcvAdapter
from nepse_ingestion.pipelines.ohlcv import run_ohlcv_pipeline


def test_ohlcv_pipeline_idempotent(tmp_path):
    sf, engine = make_session_factory(f"sqlite:///{tmp_path}/t.db")
    Base.metadata.create_all(engine)
    a = OhlcvAdapter()
    raw = a.fetch_fixture("tests/fixtures/ohlcv_sample.json")
    recs = a.normalize(a.parse(raw))
    s = sf(); r1 = run_ohlcv_pipeline(s, raw, recs); s.commit()
    assert r1["rows_written"] == 2
    s.close()
    s = sf(); r2 = run_ohlcv_pipeline(s, raw, recs); s.commit()
    assert r2["rows_written"] == 0
    assert r2["rows_unchanged"] == 2
    s.close()
