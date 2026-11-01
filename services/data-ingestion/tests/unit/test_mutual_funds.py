"""Unit tests for Batch 4.7 mutual fund adapters."""

from nepse_ingestion.sources.mutual_fund_profile import MutualFundProfileAdapter
from nepse_ingestion.sources.mutual_fund_nav import MutualFundNavAdapter
from nepse_ingestion.sources.mutual_fund_holdings import MutualFundHoldingsAdapter
from nepse_ingestion.normalizers.mf_performance import nav_return, premium_discount, concentration_ratio


def test_mf_profile_adapter():
    a = MutualFundProfileAdapter()
    raw = a.fetch_fixture("tests/fixtures/mf_profiles_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 2
    assert recs[0]["_valid"] is True
    assert recs[1]["fund_type"] == "close_ended"


def test_mf_nav_adapter():
    a = MutualFundNavAdapter()
    raw = a.fetch_fixture("tests/fixtures/mf_nav_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 2
    assert recs[0]["_valid"] is True


def test_mf_nav_invalid():
    a = MutualFundNavAdapter()
    bad = [{"fund_code": "X", "valuation_date": "2024-03-31", "nav_per_unit": -1}]
    nr = a.normalize(bad)
    assert nr[0]["_valid"] is False


def test_mf_holdings_adapter():
    a = MutualFundHoldingsAdapter()
    raw = a.fetch_fixture("tests/fixtures/mf_holdings_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 2
    assert recs[0]["holding_symbol"] == "NABIL"


def test_mf_performance():
    from decimal import Decimal
    assert nav_return(Decimal("10"), Decimal("12.5")) == Decimal("0.25")
    assert premium_discount(Decimal("13"), Decimal("12.5")) == Decimal("4")
    assert concentration_ratio([Decimal("12.5"), Decimal("8.3")]) == Decimal("12.5")
