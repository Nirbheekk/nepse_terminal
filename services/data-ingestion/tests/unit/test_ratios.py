"""Unit tests for ratio engine."""

from decimal import Decimal
from nepse_ingestion.normalizers.ratios import eps, pe_ratio, book_value_per_share, pb_ratio, roe, roa, net_profit_margin


def test_eps():
    assert eps(Decimal("1200"), Decimal("10000")) == Decimal("0.12")


def test_pe():
    assert pe_ratio(Decimal("100"), Decimal("0.12")) == Decimal("833.3333333333333333333333333")


def test_pe_zero_eps():
    assert pe_ratio(Decimal("100"), Decimal("0")) is None


def test_pe_negative_eps():
    assert pe_ratio(Decimal("100"), Decimal("-1")) is None


def test_bvps():
    assert book_value_per_share(Decimal("40000"), Decimal("10000")) == Decimal("4")


def test_pb():
    assert pb_ratio(Decimal("100"), Decimal("4")) == Decimal("25")


def test_roe():
    assert roe(Decimal("1200"), Decimal("40000")) == Decimal("0.03")


def test_roa():
    assert roa(Decimal("1200"), Decimal("300000")) == Decimal("0.004")


def test_net_profit_margin():
    assert net_profit_margin(Decimal("1200"), Decimal("5000")) == Decimal("0.24")
