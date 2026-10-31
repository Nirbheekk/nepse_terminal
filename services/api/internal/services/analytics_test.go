package services

import (
	"testing"

	"nepse/terminal/api/internal/marketdata"
)

func TestAnalyticsCalculations(t *testing.T) {
	// Synthesize 5 trading days
	obs := []DailyObservation{
		{
			Date:           "2024-01-01",
			PortfolioValue: marketdata.MustParse("100000"),
			NetCashFlow:    marketdata.Zero(),
			BenchmarkValue: marketdata.MustParse("2000"),
		},
		{
			Date:           "2024-01-02",
			PortfolioValue: marketdata.MustParse("105000"), // +5%
			NetCashFlow:    marketdata.Zero(),
			BenchmarkValue: marketdata.MustParse("2040"), // +2%
		},
		{
			Date:           "2024-01-03",
			PortfolioValue: marketdata.MustParse("102000"), // -2.85%
			NetCashFlow:    marketdata.Zero(),
			BenchmarkValue: marketdata.MustParse("2020"), // -0.98%
		},
		{
			Date:           "2024-01-04",
			PortfolioValue: marketdata.MustParse("110000"), // +7.84%
			NetCashFlow:    marketdata.Zero(),
			BenchmarkValue: marketdata.MustParse("2100"), // +3.96%
		},
		{
			Date:           "2024-01-05",
			PortfolioValue: marketdata.MustParse("108000"), // -1.81%
			NetCashFlow:    marketdata.Zero(),
			BenchmarkValue: marketdata.MustParse("2080"), // -0.95%
		},
	}

	rf := marketdata.MustParse("0.05") // 5% risk free rate
	m, err := ComputeAnalytics(obs, rf)
	if err != nil {
		t.Fatalf("ComputeAnalytics failed: %v", err)
	}

	// Total days = 5
	if m.TotalDays != 5 {
		t.Errorf("expected 5 days, got %d", m.TotalDays)
	}

	// Overall portfolio return = (108,000 - 100,000) / 100,000 = +8.0%
	if m.PortfolioTWR.Format(4) != "0.0800" {
		t.Errorf("expected TWR 0.0800, got %s", m.PortfolioTWR.Format(4))
	}

	// Overall benchmark return = (2080 - 2000) / 2000 = +4.0%
	if m.BenchmarkReturn.Format(4) != "0.0400" {
		t.Errorf("expected Benchmark Return 0.0400, got %s", m.BenchmarkReturn.Format(4))
	}

	// Beta should be positive (portfolio moved in same direction as benchmark with higher amplitude)
	if !m.Beta.IsPositive() {
		t.Errorf("expected positive beta, got %s", m.Beta.Format(4))
	}

	// Max drawdown: peaked at 105,000 on 2024-01-02, dropped to 102,000 on 2024-01-03:
	// (105,000 - 102,000) / 105,000 = 3,000 / 105,000 = 0.02857 (2.86%)
	if m.MaxDrawdown.Format(4) != "0.0286" {
		t.Errorf("expected Max Drawdown 0.0286, got %s", m.MaxDrawdown.Format(4))
	}
	if m.PeakDate != "2024-01-02" {
		t.Errorf("expected peak date 2024-01-02, got %s", m.PeakDate)
	}
	if m.TroughDate != "2024-01-03" {
		t.Errorf("expected trough date 2024-01-03, got %s", m.TroughDate)
	}

	// Current drawdown: peaked at 110,000 on 2024-01-04, current value 108,000:
	// (110,000 - 108,000) / 110,000 = 2,000 / 110,000 = 0.01818 (1.82%)
	if m.CurrentDrawdown.Format(4) != "0.0182" {
		t.Errorf("expected Current Drawdown 0.0182, got %s", m.CurrentDrawdown.Format(4))
	}
}

func TestWithCashInflow(t *testing.T) {
	// Day 1: 100k
	// Day 2: Deposit 50k -> Value becomes 150k without market gain (return = 0)
	// Day 3: Market moves to 165k (+10% return)
	obs := []DailyObservation{
		{
			Date:           "2024-01-01",
			PortfolioValue: marketdata.MustParse("100000"),
			NetCashFlow:    marketdata.Zero(),
			BenchmarkValue: marketdata.MustParse("2000"),
		},
		{
			Date:           "2024-01-02",
			PortfolioValue: marketdata.MustParse("150000"),
			NetCashFlow:    marketdata.MustParse("50000"), // 50k deposit
			BenchmarkValue: marketdata.MustParse("2000"),
		},
		{
			Date:           "2024-01-03",
			PortfolioValue: marketdata.MustParse("165000"), // 10% gain on 150k
			NetCashFlow:    marketdata.Zero(),
			BenchmarkValue: marketdata.MustParse("2200"), // 10% gain
		},
	}

	rf := marketdata.MustParse("0.05")
	m, err := ComputeAnalytics(obs, rf)
	if err != nil {
		t.Fatalf("ComputeAnalytics failed: %v", err)
	}

	// TWR should eliminate the 50k deposit distortion:
	// Day 2 return: (150,000 - (100,000 + 50,000)) / 150,000 = 0.0%
	// Day 3 return: (165,000 - 150,000) / 150,000 = +10.0%
	// Cumulative TWR should be exactly 10.0%
	if m.PortfolioTWR.Format(4) != "0.1000" {
		t.Errorf("expected TWR 0.1000 (10%%), got %s", m.PortfolioTWR.Format(4))
	}
}
