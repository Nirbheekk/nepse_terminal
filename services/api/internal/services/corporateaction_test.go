package services

import (
	"testing"

	"nepse/terminal/api/internal/marketdata"
)

func TestStockSplitAdjustment(t *testing.T) {
	bars := []OHLCVBar{
		{Date: "2024-01-01", Open: marketdata.MustParse("200"), High: marketdata.MustParse("210"), Low: marketdata.MustParse("195"), Close: marketdata.MustParse("200"), Volume: marketdata.MustParse("1000")},
		{Date: "2024-01-02", Open: marketdata.MustParse("200"), High: marketdata.MustParse("205"), Low: marketdata.MustParse("198"), Close: marketdata.MustParse("200"), Volume: marketdata.MustParse("1200")},
		{Date: "2024-01-05", Open: marketdata.MustParse("100"), High: marketdata.MustParse("105"), Low: marketdata.MustParse("98"), Close: marketdata.MustParse("100"), Volume: marketdata.MustParse("2500")},
	}

	actions := []CorporateAction{
		{
			ActionType:      ActionStockSplit,
			BookClosureDate: "2024-01-05",
			SplitFrom:       marketdata.MustParse("1"),
			SplitTo:         marketdata.MustParse("2"), // 2 for 1
			Status:          StatusCompleted,
		},
	}

	adj, err := AdjustPrices(bars, actions, PolicySplitBonus)
	if err != nil {
		t.Fatalf("AdjustPrices failed: %v", err)
	}

	// Pre-split bars should have prices halved (200 -> 100) and volume doubled (1000 -> 2000)
	if adj[0].Close.Format(2) != "100.00" {
		t.Errorf("expected adjusted close 100.00, got %s", adj[0].Close.Format(2))
	}
	if adj[0].Volume.Format(2) != "2000.00" {
		t.Errorf("expected adjusted volume 2000.00, got %s", adj[0].Volume.Format(2))
	}

	// Post-split bar should remain unchanged
	if adj[2].Close.Format(2) != "100.00" {
		t.Errorf("expected post-split close 100.00, got %s", adj[2].Close.Format(2))
	}
}

func TestBonusShareAdjustment(t *testing.T) {
	bars := []OHLCVBar{
		{Date: "2024-02-01", Close: marketdata.MustParse("220"), Volume: marketdata.MustParse("1000")},
		{Date: "2024-02-05", Close: marketdata.MustParse("200"), Volume: marketdata.MustParse("1500")},
	}

	actions := []CorporateAction{
		{
			ActionType:      ActionBonusShare,
			BookClosureDate: "2024-02-05",
			BonusPercent:    marketdata.MustParse("10.0"), // 10% bonus share
			Status:          StatusCompleted,
		},
	}

	adj, err := AdjustPrices(bars, actions, PolicySplitBonus)
	if err != nil {
		t.Fatalf("AdjustPrices failed: %v", err)
	}

	// Factor = 1 / 1.10 = 0.9090909...
	// Pre-bonus close = 220 * (1/1.10) = 200.00
	if adj[0].Close.Format(2) != "200.00" {
		t.Errorf("expected pre-bonus adjusted close 200.00, got %s", adj[0].Close.Format(2))
	}
}

func TestRightsIssueAdjustmentTERP(t *testing.T) {
	// Cum close = 300, Rights ratio = 1:1 (1.0), Rights price = 100
	// TERP = (300 + 1 * 100) / 2 = 200
	// Factor = 200 / 300 = 2/3 = 0.666667
	bars := []OHLCVBar{
		{Date: "2024-03-01", Close: marketdata.MustParse("300"), Volume: marketdata.MustParse("1000")},
		{Date: "2024-03-05", Close: marketdata.MustParse("200"), Volume: marketdata.MustParse("2000")},
	}

	actions := []CorporateAction{
		{
			ActionType:      ActionRightsIssue,
			BookClosureDate: "2024-03-05",
			RightsRatio:     marketdata.MustParse("1.0"),
			RightsPrice:     marketdata.MustParse("100.0"),
			Status:          StatusCompleted,
		},
	}

	// PolicySplitBonus should IGNORE rights
	adjSB, err := AdjustPrices(bars, actions, PolicySplitBonus)
	if err != nil {
		t.Fatalf("AdjustPrices failed: %v", err)
	}
	if adjSB[0].Close.Format(2) != "300.00" {
		t.Errorf("PolicySplitBonus should not adjust rights: got %s", adjSB[0].Close.Format(2))
	}

	// PolicySplitBonusRights should ADJUST rights
	adjSBR, err := AdjustPrices(bars, actions, PolicySplitBonusRights)
	if err != nil {
		t.Fatalf("AdjustPrices failed: %v", err)
	}
	// 300 * (2/3) = 200.00
	if adjSBR[0].Close.Format(2) != "200.00" {
		t.Errorf("expected pre-rights adjusted close 200.00, got %s", adjSBR[0].Close.Format(2))
	}
}

func TestCashDividendTotalReturnPolicy(t *testing.T) {
	// Cum close = 500, Cash dividend = Rs 50 (10%)
	// Ex-dividend price = 450, Factor = 450 / 500 = 0.90
	bars := []OHLCVBar{
		{Date: "2024-04-01", Close: marketdata.MustParse("500"), Volume: marketdata.MustParse("1000")},
		{Date: "2024-04-05", Close: marketdata.MustParse("450"), Volume: marketdata.MustParse("1000")},
	}

	actions := []CorporateAction{
		{
			ActionType:      ActionCashDividend,
			BookClosureDate: "2024-04-05",
			CashAmount:      marketdata.MustParse("50.0"),
			Status:          StatusCompleted,
		},
	}

	// PolicySplitBonusRights should IGNORE cash dividend
	adjSBR, _ := AdjustPrices(bars, actions, PolicySplitBonusRights)
	if adjSBR[0].Close.Format(2) != "500.00" {
		t.Errorf("PolicySplitBonusRights should not adjust cash dividend: got %s", adjSBR[0].Close.Format(2))
	}

	// PolicyTotalReturn should ADJUST cash dividend: 500 * 0.9 = 450.00
	adjTR, _ := AdjustPrices(bars, actions, PolicyTotalReturn)
	if adjTR[0].Close.Format(2) != "450.00" {
		t.Errorf("PolicyTotalReturn expected 450.00, got %s", adjTR[0].Close.Format(2))
	}
}
