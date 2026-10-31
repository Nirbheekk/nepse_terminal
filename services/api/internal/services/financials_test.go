package services

import (
	"fmt"
	"testing"

	"nepse/terminal/api/internal/marketdata"
)

func TestFinancialStatementValidation(t *testing.T) {
	stmt := NewStatement(FinancialPeriod{
		Symbol:      "NABIL",
		FiscalYear:  "2080/81",
		PeriodType:  PeriodAnnual,
		PeriodEnd:   "2024-07-15",
		AuditStatus: StatusAudited,
	})

	// Add Balance Sheet items
	stmt.SetLine(TypeBalanceSheet, LineTotalAssets, "Total Assets", marketdata.MustParse("500000000"), 10)
	stmt.SetLine(TypeBalanceSheet, LineTotalLiabilities, "Total Liabilities", marketdata.MustParse("400000000"), 20)
	stmt.SetLine(TypeBalanceSheet, LineShareholdersEquity, "Shareholders Equity", marketdata.MustParse("100000000"), 30)
	stmt.SetLine(TypeBalanceSheet, LineTotalSharesOutstanding, "Shares", marketdata.MustParse("1000000"), 40)

	// Add Income Statement items
	stmt.SetLine(TypeIncomeStatement, LineRevenue, "Revenue", marketdata.MustParse("80000000"), 10)
	stmt.SetLine(TypeIncomeStatement, LineCostOfRevenue, "Cost of Revenue", marketdata.MustParse("30000000"), 20)
	stmt.SetLine(TypeIncomeStatement, LineOperatingExpenses, "Operating Expenses", marketdata.MustParse("20000000"), 40)
	stmt.SetLine(TypeIncomeStatement, LineNetIncome, "Net Income", marketdata.MustParse("25000000"), 80)

	// Add Cash Flow items
	stmt.SetLine(TypeCashFlow, LineOperatingCashFlow, "Operating Cash Flow", marketdata.MustParse("35000000"), 10)
	stmt.SetLine(TypeCashFlow, LineCapEx, "CapEx", marketdata.MustParse("10000000"), 20)

	if err := stmt.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if len(stmt.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %v", stmt.Warnings)
	}

	// Verify derivations
	gp := stmt.GetLine(LineGrossProfit)
	if gp.Format(2) != "50000000.00" { // 80M - 30M
		t.Errorf("expected Gross Profit 50,000,000, got %s", gp.Format(2))
	}

	op := stmt.GetLine(LineOperatingIncome)
	if op.Format(2) != "30000000.00" { // 50M - 20M
		t.Errorf("expected Operating Income 30,000,000, got %s", op.Format(2))
	}

	fcf := stmt.GetLine(LineFreeCashFlow)
	if fcf.Format(2) != "25000000.00" { // 35M - 10M
		t.Errorf("expected Free Cash Flow 25,000,000, got %s", fcf.Format(2))
	}

	bvps := stmt.GetLine(LineBookValuePerShare)
	if bvps.Format(2) != "100.00" { // 100M / 1M
		t.Errorf("expected BVPS 100.00, got %s", bvps.Format(2))
	}

	eps := stmt.GetLine(LineEPSBasic)
	if eps.Format(2) != "25.00" { // 25M / 1M
		t.Errorf("expected EPS 25.00, got %s", eps.Format(2))
	}
}

func TestTTMSynthesis(t *testing.T) {
	quarters := make([]*Statement, 4)
	qTypes := []PeriodType{PeriodQ1, PeriodQ2, PeriodQ3, PeriodQ4}
	dates := []string{"2023-10-15", "2024-01-15", "2024-04-15", "2024-07-15"}

	for i := 0; i < 4; i++ {
		s := NewStatement(FinancialPeriod{
			Symbol:      "SCB",
			FiscalYear:  "2080/81",
			PeriodType:  qTypes[i],
			PeriodStart: "2023-07-16",
			PeriodEnd:   dates[i],
			AuditStatus: StatusUnaudited,
		})
		// Revenue: 25M each quarter -> 100M TTM
		s.SetLine(TypeIncomeStatement, LineRevenue, "Revenue", marketdata.MustParse("25000000"), 10)
		// Net income: 5M each quarter -> 20M TTM
		s.SetLine(TypeIncomeStatement, LineNetIncome, "Net Income", marketdata.MustParse("5000000"), 80)
		// Shares: 2M in all quarters
		s.SetLine(TypeBalanceSheet, LineTotalSharesOutstanding, "Shares", marketdata.MustParse("2000000"), 10)
		// Equity: grows each quarter: 50M, 55M, 60M, 65M
		equity := fmt.Sprintf("%d000000", 50+5*i)
		s.SetLine(TypeBalanceSheet, LineShareholdersEquity, "Equity", marketdata.MustParse(equity), 20)

		_ = s.Validate()
		quarters[i] = s
	}

	ttm, err := SynthesizeTTM(quarters)
	if err != nil {
		t.Fatalf("failed to synthesize TTM: %v", err)
	}

	if ttm.Period.PeriodType != PeriodTTM {
		t.Errorf("expected period type TTM, got %s", ttm.Period.PeriodType)
	}

	// Verify flow items summed
	rev := ttm.GetLine(LineRevenue)
	if rev.Format(2) != "100000000.00" {
		t.Errorf("expected TTM Revenue 100,000,000, got %s", rev.Format(2))
	}

	netInc := ttm.GetLine(LineNetIncome)
	if netInc.Format(2) != "20000000.00" {
		t.Errorf("expected TTM Net Income 20,000,000, got %s", netInc.Format(2))
	}

	// Verify stock item took latest (quarter 4 equity = 65M)
	eq := ttm.GetLine(LineShareholdersEquity)
	if eq.Format(2) != "65000000.00" {
		t.Errorf("expected TTM Equity 65,000,000, got %s", eq.Format(2))
	}

	// Verify TTM EPS: 20M / 2M = 10.00
	eps := ttm.GetLine(LineEPSBasic)
	if eps.Format(2) != "10.00" {
		t.Errorf("expected TTM EPS 10.00, got %s", eps.Format(2))
	}
}
