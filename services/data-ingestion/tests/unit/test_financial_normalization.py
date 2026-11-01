"""Unit tests for financial normalization."""

from nepse_ingestion.normalizers.financial import map_label, validate_balance_sheet
from decimal import Decimal


def test_map_label():
    assert map_label("Total Assets") == "total_assets"
    assert map_label("Net Profit After Tax") == "net_profit"


def test_validate_balance_sheet():
    assert validate_balance_sheet(Decimal("300000"), Decimal("260000"), Decimal("40000")) is True
    assert validate_balance_sheet(Decimal("300000"), Decimal("260000"), Decimal("39999")) is False
