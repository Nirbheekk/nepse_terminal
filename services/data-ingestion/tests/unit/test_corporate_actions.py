"""Unit tests for Batch 4.5 adapters."""

from nepse_ingestion.sources.dividends import DividendCapitalActionAdapter
from nepse_ingestion.sources.mergers import MergerDisclosureAdapter
from nepse_ingestion.sources.meetings import CompanyMeetingAdapter
from nepse_ingestion.sources.public_issues import PublicIssueAdapter


def test_dividend_adapter():
    a = DividendCapitalActionAdapter()
    raw = a.fetch_fixture("tests/fixtures/dividends_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 4
    assert recs[0]["_valid"] is True
    assert recs[0]["cash_percent"] == 20.0


def test_merger_adapter():
    a = MergerDisclosureAdapter()
    raw = a.fetch_fixture("tests/fixtures/mergers_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 1
    assert recs[0]["_valid"] is True
    assert recs[0]["surviving_symbol"] == "NABIL"


def test_meeting_adapter():
    a = CompanyMeetingAdapter()
    raw = a.fetch_fixture("tests/fixtures/meetings_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 2
    assert recs[0]["meeting_type"] == "AGM"
    assert recs[1]["meeting_type"] == "SGM"


def test_public_issue_adapter():
    a = PublicIssueAdapter()
    raw = a.fetch_fixture("tests/fixtures/public_issues_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 2
    assert recs[0]["issue_type"] == "IPO"
    assert recs[1]["issue_type"] == "FPO"
    # invalid date ordering check not in adapter; ensure missing symbol invalid
    bad = [{"symbol": "", "issue_type": "IPO"}]
    nr = a.normalize(bad)
    assert nr[0]["_valid"] is False
