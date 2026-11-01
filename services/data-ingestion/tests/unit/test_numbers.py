"""Unit tests for number normalization."""

from decimal import Decimal
from nepse_ingestion.normalizers.numbers import parse_decimal, map_period_type, fiscal_year_from_dates, clean_symbol
from datetime import date


def test_parse_decimal_missing():
    assert parse_decimal(None) is None
    assert parse_decimal("") is None
    assert parse_decimal("-") is None
    assert parse_decimal("N/A") is None


def test_parse_decimal_parentheses_negative():
    assert parse_decimal("(100.5)") == Decimal("-100.5")


def test_parse_decimal_commas():
    assert parse_decimal("1,234,567.89") == Decimal("1234567.89")


def test_parse_decimal_does_not_zero_missing():
    assert parse_decimal(None) is None  # never zero


def test_map_period_type():
    assert map_period_type("Q1") == "Q1"
    assert map_period_type("Second Quarter") == "Q2"
    assert map_period_type("Annual") == "ANNUAL"


def test_fiscal_year():
    assert fiscal_year_from_dates(date(2024, 1, 1), date(2024, 3, 31)) == "2023/2024"
    assert fiscal_year_from_dates(date(2024, 8, 1), date(2024, 9, 30)) == "2024/2025"


def test_clean_symbol():
    assert clean_symbol(" nab ") == "NAB"
