package services

import (
	"testing"

	"nepse/terminal/api/internal/marketdata"
)

func TestAnnualFinancialRatios(t *testing.T) {
	stmt := NewStatement(FinancialPeriod{
		Symbol:      "NABIL",
		FiscalYear:  "2080/81",
		PeriodType:  PeriodAnnual,
		PeriodEnd:   "2024-07-15",
		AuditStatus: StatusAudited,
	})

	// Balance Sheet
	stmt.SetLine(TypeBalanceSheet, LineTotalAssets, "Total Assets", marketdata.MustParse("200000000"), 10)
	stmt.SetLine(TypeBalanceSheet, LineCurrentAssets, "Current Assets", marketdata.MustParse("80000000"), 20)
	stmt.SetLine(TypeBalanceSheet, LineCashAndEquivalents, "Cash", marketdata.MustParse("20000000"), 30)
	stmt.SetLine(TypeBalanceSheet, LineInvestments, "Investments", marketdata.MustParse("30000000"), 40)
	stmt.SetLine(TypeBalanceSheet, LineTotalLiabilities, "Total Liabilities", marketdata.MustParse("120000000"), 50)
	stmt.SetLine(TypeBalanceSheet, LineCurrentLiabilities, "Current Liabilities", marketdata.MustParse("40000000"), 60)
	stmt.SetLine(TypeBalanceSheet, LineTotalDebt, "Total Debt", marketdata.MustParse("50000000"), 70)
	stmt.SetLine(TypeBalanceSheet, LineShareholdersEquity, "Equity", marketdata.MustParse("80000000"), 80)
	stmt.SetLine(TypeBalanceSheet, LineTotalSharesOutstanding, "Shares", marketdata.MustParse("1000000"), 90)

	// Income Statement
	stmt.SetLine(TypeIncomeStatement, LineRevenue, "Revenue", marketdata.MustParse("50000000"), 10)
	stmt.SetLine(TypeIncomeStatement, LineCostOfRevenue, "Cost", marketdata.MustParse("20000000"), 20)
	stmt.SetLine(TypeIncomeStatement, LineGrossProfit, "Gross Profit", marketdata.MustParse("30000000"), 30)
	stmt.SetLine(TypeIncomeStatement, LineOperatingIncome, "Operating Income", marketdata.MustParse("20000000"), 40)
	stmt.SetLine(TypeIncomeStatement, LineInterestExpense, "Interest", marketdata.MustParse("5000000"), 50)
	stmt.SetLine(TypeIncomeStatement, LineNetIncome, "Net Income", marketdata.MustParse("12000000"), 60)

	marketPrice := marketdata.MustParse("240.00")
	r, err := ComputeRatios(stmt, marketPrice, "2024-07-20")
	if err != nil {
		t.Fatalf("ComputeRatios failed: %v", err)
	}

	// Shares = 1M, Market Cap = 240 * 1M = 240,000,000
	if r.MarketCap.Format(2) != "240000000.00" {
		t.Errorf("MarketCap: expected 240,000,000, got %s", r.MarketCap.Format(2))
	}

	// EPS = 12M / 1M = 12.00
	if r.EPS.Format(2) != "12.00" {
		t.Errorf("EPS: expected 12.00, got %s", r.EPS.Format(2))
	}

	// BVPS = 80M / 1M = 80.00
	if r.BookValuePerShare.Format(2) != "80.00" {
		t.Errorf("BVPS: expected 80.00, got %s", r.BookValuePerShare.Format(2))
	}

	// PE = 240 / 12 = 20.00
	if r.PE.Format(2) != "20.00" {
		t.Errorf("PE: expected 20.00, got %s", r.PE.Format(2))
	}

	// PB = 240 / 80 = 3.00
	if r.PB.Format(2) != "3.00" {
		t.Errorf("PB: expected 3.00, got %s", r.PB.Format(2))
	}

	// PS = 240M / 50M = 4.80
	if r.PS.Format(2) != "4.80" {
		t.Errorf("PS: expected 4.80, got %s", r.PS.Format(2))
	}

	// ROE = 12M / 80M = 0.15 (15%)
	if r.ROE.Format(4) != "0.1500" {
		t.Errorf("ROE: expected 0.1500, got %s", r.ROE.Format(4))
	}

	// ROA = 12M / 200M = 0.06 (6%)
	if r.ROA.Format(4) != "0.0600" {
		t.Errorf("ROA: expected 0.0600, got %s", r.ROA.Format(4))
	}

	// Current Ratio = 80M / 40M = 2.00
	if r.CurrentRatio.Format(2) != "2.00" {
		t.Errorf("CurrentRatio: expected 2.00, got %s", r.CurrentRatio.Format(2))
	}

	// Quick Ratio = (20M Cash + 30M Inv) / 40M = 50M / 40M = 1.25
	if r.QuickRatio.Format(2) != "1.25" {
		t.Errorf("QuickRatio: expected 1.25, got %s", r.QuickRatio.Format(2))
	}

	// Debt to Equity = 50M / 80M = 0.625
	if r.DebtToEquity.Format(3) != "0.625" {
		t.Errorf("DebtToEquity: expected 0.625, got %s", r.DebtToEquity.Format(3))
	}

	// Interest Coverage = 20M / 5M = 4.00
	if r.InterestCoverage.Format(2) != "4.00" {
		t.Errorf("InterestCoverage: expected 4.00, got %s", r.InterestCoverage.Format(2))
	}

	// Margins:
	// Gross Margin = 30M / 50M = 0.60
	if r.GrossMargin.Format(2) != "0.60" {
		t.Errorf("GrossMargin: expected 0.60, got %s", r.GrossMargin.Format(2))
	}
	// Operating Margin = 20M / 50M = 0.40
	if r.OperatingMargin.Format(2) != "0.40" {
		t.Errorf("OperatingMargin: expected 0.40, got %s", r.OperatingMargin.Format(2))
	}
	// Net Margin = 12M / 50M = 0.24
	if r.NetMargin.Format(2) != "0.24" {
		t.Errorf("NetMargin: expected 0.24, got %s", r.NetMargin.Format(2))
	}
}

func TestQuarterlyAnnualizedRatios(t *testing.T) {
	// Q1 Statement
	stmt := NewStatement(FinancialPeriod{
		Symbol:      "NABIL",
		FiscalYear:  "2081/82",
		PeriodType:  PeriodQ1,
		PeriodEnd:   "2024-10-15",
		AuditStatus: StatusUnaudited,
	})

	stmt.SetLine(TypeBalanceSheet, LineTotalSharesOutstanding, "Shares", marketdata.MustParse("1000000"), 10)
	stmt.SetLine(TypeBalanceSheet, LineShareholdersEquity, "Equity", marketdata.MustParse("80000000"), 20)
	stmt.SetLine(TypeIncomeStatement, LineNetIncome, "Q1 Net Income", marketdata.MustParse("3000000"), 30)

	marketPrice := marketdata.MustParse("240.00")
	r, err := ComputeRatios(stmt, marketPrice, "2024-10-20")
	if err != nil {
		t.Fatalf("ComputeRatios failed: %v", err)
	}

	// Q1 Net Income = 3M, EPS = 3.00.
	// Annualized EPS = 3.00 * 4 = 12.00
	if r.EPS.Format(2) != "3.00" {
		t.Errorf("Q1 EPS: expected 3.00, got %s", r.EPS.Format(2))
	}
	if r.AnnualizedEPS.Format(2) != "12.00" {
		t.Errorf("Annualized EPS: expected 12.00, got %s", r.AnnualizedEPS.Format(2))
	}

	// Annualized PE = 240 / 12 = 20.00
	if r.PE.Format(2) != "20.00" {
		t.Errorf("PE: expected 20.00, got %s", r.PE.Format(2))
	}

	// Annualized ROE = (3M * 4) / 80M = 12M / 80M = 0.1500
	if r.ROE.Format(4) != "0.1500" {
		t.Errorf("Annualized ROE: expected 0.1500, got %s", r.ROE.Format(4))
	}
}
