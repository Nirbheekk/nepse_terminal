package portfolios

import (
	"testing"

	"nepse/terminal/api/internal/marketdata"
)

func TestNEPSEFees(t *testing.T) {
	// Turnover 40,000 <= 50,000 -> 0.40% = 160.00
	t1 := marketdata.MustParse("40000")
	c1 := CalculateBrokerCommission(t1)
	if c1.Format(2) != "160.00" {
		t.Errorf("expected commission 160.00, got %s", c1.Format(2))
	}

	// Turnover 100,000 (50k - 500k) -> 0.37% = 370.00
	t2 := marketdata.MustParse("100000")
	c2 := CalculateBrokerCommission(t2)
	if c2.Format(2) != "370.00" {
		t.Errorf("expected commission 370.00, got %s", c2.Format(2))
	}

	// SEBON fee: 100,000 * 0.00015 = 15.00
	s1 := CalculateSEBONFee(t2)
	if s1.Format(2) != "15.00" {
		t.Errorf("expected SEBON fee 15.00, got %s", s1.Format(2))
	}

	// DP fee = 25.00
	dp := FlatDPFee()
	if dp.Format(2) != "25.00" {
		t.Errorf("expected DP fee 25.00, got %s", dp.Format(2))
	}
}

func TestWACPortfolioAccounting(t *testing.T) {
	pf := NewPortfolioState("p1", "u1", "My Nepal Portfolio", MethodWAC)

	// Deposit 1,000,000
	_, err := pf.ProcessTransaction(Transaction{
		TxnType:     TxnDeposit,
		TradedOn:    "2024-01-01",
		GrossAmount: marketdata.MustParse("1000000"),
	})
	if err != nil {
		t.Fatalf("deposit failed: %v", err)
	}

	// Buy 100 shares of NABIL @ 500
	// Gross = 50,000. Comm = 200, SEBON = 7.50, DP = 25 -> Total Fees = 232.50. Net = 50,232.50
	tBuy1, err := pf.ProcessTransaction(Transaction{
		Symbol:   "NABIL",
		TxnType:  TxnBuy,
		TradedOn: "2024-01-10",
		Quantity: marketdata.MustParse("100"),
		Price:    marketdata.MustParse("500"),
	})
	if err != nil {
		t.Fatalf("buy 1 failed: %v", err)
	}
	if tBuy1.NetAmount.Format(2) != "50232.50" {
		t.Errorf("expected net buy 50232.50, got %s", tBuy1.NetAmount.Format(2))
	}

	// Buy another 100 shares of NABIL @ 600
	// Gross = 60,000. Comm = 222 (0.37%), SEBON = 9.00, DP = 25 -> Total Fees = 256.00. Net = 60,256.00
	tBuy2, err := pf.ProcessTransaction(Transaction{
		Symbol:   "NABIL",
		TxnType:  TxnBuy,
		TradedOn: "2024-02-10",
		Quantity: marketdata.MustParse("100"),
		Price:    marketdata.MustParse("600"),
	})
	if err != nil {
		t.Fatalf("buy 2 failed: %v", err)
	}
	if tBuy2.NetAmount.Format(2) != "60256.00" {
		t.Errorf("expected net buy 60256.00, got %s", tBuy2.NetAmount.Format(2))
	}

	// NABIL Position: 200 shares. Total cost = 50232.50 + 60256.00 = 110,488.50
	// Average cost per share = 110488.50 / 200 = 552.4425
	pos := pf.Positions["NABIL"]
	if pos.Quantity.Format(2) != "200.00" {
		t.Errorf("expected 200 shares, got %s", pos.Quantity.Format(2))
	}
	if pos.AvgCostPerShare.Format(2) != "552.44" {
		t.Errorf("expected avg cost 552.44, got %s", pos.AvgCostPerShare.Format(2))
	}

	// 10% Bonus Share: adds 20 shares at 0 cost
	// Quantity becomes 220, CostBasis remains 110488.50
	// New AvgCostPerShare = 110488.50 / 220 = 502.22
	_, err = pf.ProcessTransaction(Transaction{
		Symbol:   "NABIL",
		TxnType:  TxnDividendBonus,
		TradedOn: "2024-03-01",
		Quantity: marketdata.MustParse("20"),
	})
	if err != nil {
		t.Fatalf("bonus shares failed: %v", err)
	}
	if pos.Quantity.Format(2) != "220.00" {
		t.Errorf("expected 220 shares after bonus, got %s", pos.Quantity.Format(2))
	}
	if pos.AvgCostPerShare.Format(2) != "502.22" {
		t.Errorf("expected avg cost 502.22 after bonus, got %s", pos.AvgCostPerShare.Format(2))
	}

	// Sell 100 shares @ 700
	// Gross = 70,000. Fees = 70,000*0.0037 + 70,000*0.00015 + 25 = 259 + 10.50 + 25 = 294.50
	// Cost basis = 100 * 502.22045 = 50222.05
	// Realized Gain = 70000 - 294.50 - 50222.05 = 19483.45
	tSell, err := pf.ProcessTransaction(Transaction{
		Symbol:   "NABIL",
		TxnType:  TxnSell,
		TradedOn: "2024-04-01",
		Quantity: marketdata.MustParse("100"),
		Price:    marketdata.MustParse("700"),
	})
	if err != nil {
		t.Fatalf("sell failed: %v", err)
	}
	if !tSell.RealizedPnL.IsPositive() {
		t.Errorf("expected positive realized PnL, got %s", tSell.RealizedPnL.Format(2))
	}

	// Value remaining 120 shares at 750
	pf.ValuePortfolio(map[string]marketdata.Decimal{
		"NABIL": marketdata.MustParse("750.00"),
	})

	if pos.Quantity.Format(2) != "120.00" {
		t.Errorf("expected 120 remaining shares, got %s", pos.Quantity.Format(2))
	}
	if pos.MarketValue.Format(2) != "90000.00" { // 120 * 750
		t.Errorf("expected market value 90,000.00, got %s", pos.MarketValue.Format(2))
	}
}

func TestFIFOPortfolioAccounting(t *testing.T) {
	pf := NewPortfolioState("p2", "u1", "FIFO Portfolio", MethodFIFO)

	// Buy lot 1: 50 shares @ 100 on 2023-01-01 (Long term when sold on 2024-03-01)
	_, _ = pf.ProcessTransaction(Transaction{
		ID:       "lot1",
		Symbol:   "SHIVM",
		TxnType:  TxnBuy,
		TradedOn: "2023-01-01",
		Quantity: marketdata.MustParse("50"),
		Price:    marketdata.MustParse("100"),
	})

	// Buy lot 2: 50 shares @ 200 on 2024-01-01 (Short term when sold on 2024-03-01)
	_, _ = pf.ProcessTransaction(Transaction{
		ID:       "lot2",
		Symbol:   "SHIVM",
		TxnType:  TxnBuy,
		TradedOn: "2024-01-01",
		Quantity: marketdata.MustParse("50"),
		Price:    marketdata.MustParse("200"),
	})

	// Sell 60 shares @ 250 on 2024-03-01
	// FIFO should consume entire 50 shares of lot 1 (purchased @ 100), and 10 shares of lot 2 (purchased @ 200)
	tSell, err := pf.ProcessTransaction(Transaction{
		Symbol:   "SHIVM",
		TxnType:  TxnSell,
		TradedOn: "2024-03-01",
		Quantity: marketdata.MustParse("60"),
		Price:    marketdata.MustParse("250"),
	})
	if err != nil {
		t.Fatalf("sell FIFO failed: %v", err)
	}

	lots := pf.Lots["SHIVM"]
	if lots[0].RemainingQuantity.Format(2) != "0.00" {
		t.Errorf("lot 1 should be completely depleted, got remaining %s", lots[0].RemainingQuantity.Format(2))
	}
	if lots[1].RemainingQuantity.Format(2) != "40.00" {
		t.Errorf("lot 2 should have 40 shares remaining, got %s", lots[1].RemainingQuantity.Format(2))
	}

	if !tSell.RealizedPnL.IsPositive() {
		t.Errorf("expected positive realized PnL, got %s", tSell.RealizedPnL.Format(2))
	}
}
