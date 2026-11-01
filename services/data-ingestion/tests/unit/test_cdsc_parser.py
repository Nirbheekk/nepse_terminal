"""Unit tests for CDSC IPO parser."""

from pathlib import Path
from nepse_ingestion.sources.cdsc import CdscIpoAdapter


def test_cdsc_parse_fixture():
    adapter = CdscIpoAdapter()
    raw = adapter.fetch_fixture("tests/fixtures/cdsc_ipolist_sample.html")
    recs = adapter.parse(raw)
    assert len(recs) == 3
    assert recs[0]["symbol"] == "NABIL"
    assert recs[1]["issue_type"] == "FPO"


def test_cdsc_normalize():
    adapter = CdscIpoAdapter()
    raw = adapter.fetch_fixture("tests/fixtures/cdsc_ipolist_sample.html")
    recs = adapter.normalize(adapter.parse(raw))
    assert all(r["_valid"] for r in recs)
