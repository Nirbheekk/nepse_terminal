"""Unit tests for OHLCV adapter."""

from nepse_ingestion.sources.ohlcv import OhlcvAdapter


def test_ohlcv_adapter():
    a = OhlcvAdapter()
    raw = a.fetch_fixture("tests/fixtures/ohlcv_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 2
    assert recs[0]["_valid"] is True


def test_ohlcv_invalid():
    a = OhlcvAdapter()
    bad = [{"symbol": "X", "trading_date": "2024-01-15", "open": 100, "high": 90, "low": 95, "close": 98, "volume": 10}]
    nr = a.normalize(bad)
    assert nr[0]["_valid"] is False
