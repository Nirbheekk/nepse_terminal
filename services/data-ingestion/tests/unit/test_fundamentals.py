"""Unit tests for Batch 4.4 adapters."""

from pathlib import Path

from nepse_ingestion.sources.company_profile import CompanyProfileAdapter
from nepse_ingestion.sources.board import BoardDisclosureAdapter
from nepse_ingestion.sources.ownership import OwnershipDisclosureAdapter
from nepse_ingestion.sources.financial_document import IssuerFinancialReportAdapter


def test_company_profile_adapter():
    a = CompanyProfileAdapter()
    raw = a.fetch_fixture("tests/fixtures/company_profiles_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 2
    assert recs[0]["symbol"] == "NABIL"
    assert recs[0]["_valid"] is True


def test_board_adapter():
    a = BoardDisclosureAdapter()
    raw = a.fetch_fixture("tests/fixtures/board_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 2
    assert recs[1]["is_independent"] is True


def test_ownership_adapter():
    a = OwnershipDisclosureAdapter()
    raw = a.fetch_fixture("tests/fixtures/ownership_sample.json")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 2
    assert recs[0]["promoter_percent"] == 51.0
    # invalid percentage test
    bad = [{"symbol": "X", "effective_date": "2024-01-01", "promoter_percent": 150}]
    nr = a.normalize(bad)
    assert nr[0]["_valid"] is False


def test_financial_document_adapter():
    a = IssuerFinancialReportAdapter()
    raw = a.fetch_fixture("tests/fixtures/financial_report_sample.bin")
    recs = a.normalize(a.parse(raw))
    assert len(recs) == 1
    assert recs[0]["_valid"] is True


def test_entity_resolution():
    from nepse_ingestion.normalizers.entity_resolution import resolve_symbol
    r = resolve_symbol(symbol="NABIL", company_name="Nabil Bank")
    assert r.canonical_symbol == "NABIL"
    r2 = resolve_symbol(symbol="", company_name="Unknown")
    assert r2.needs_review is True
