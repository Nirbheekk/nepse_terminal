# Batch 4.6 — Financial Normalization

## Canonical metric dictionary
revenue, operating_income, interest_income, interest_expense, net_profit, net_profit_attributable, total_assets, total_liabilities, total_equity, paid_up_capital, shares_outstanding, retained_earnings, operating/investing/financing cash flow, cash_and_equivalents, reported_eps, reported_book_value_per_share.

## Label mapping
map_label() preserves original labels and maps to canonical codes. Ambiguous mappings flagged.

## Unit normalization
parse_decimal + normalize_units (thousands/lakhs/crores/millions -> NPR). Missing values never zero.
