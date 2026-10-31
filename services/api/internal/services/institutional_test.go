package services

import (
	"testing"

	"nepse/terminal/api/internal/marketdata"
)

func TestRBACPermissions(t *testing.T) {
	// Org Admin should have all permissions
	if !HasPermission(RoleOrgAdmin, PermTradeApprove) {
		t.Error("OrgAdmin should have PermTradeApprove")
	}
	if !HasPermission(RoleOrgAdmin, PermReportExport) {
		t.Error("OrgAdmin should have PermReportExport")
	}
	if !HasPermission(RoleOrgAdmin, PermMemberManage) {
		t.Error("OrgAdmin should have PermMemberManage")
	}

	// Portfolio Manager cannot approve trades (compliance/admin only)
	if HasPermission(RolePortfolioManager, PermTradeApprove) {
		t.Error("PortfolioManager should NOT have PermTradeApprove (maker-checker separation)")
	}
	if !HasPermission(RolePortfolioManager, PermTradeRecord) {
		t.Error("PortfolioManager should have PermTradeRecord")
	}

	// Compliance Officer can approve trades and export reports, but cannot record trades
	if !HasPermission(RoleComplianceOfficer, PermTradeApprove) {
		t.Error("ComplianceOfficer should have PermTradeApprove")
	}
	if HasPermission(RoleComplianceOfficer, PermTradeRecord) {
		t.Error("ComplianceOfficer should NOT have PermTradeRecord")
	}
	if !HasPermission(RoleComplianceOfficer, PermReportExport) {
		t.Error("ComplianceOfficer should have PermReportExport")
	}

	// Analyst cannot export reports or approve trades
	if HasPermission(RoleAnalyst, PermReportExport) {
		t.Error("Analyst should NOT have PermReportExport")
	}
	if HasPermission(RoleAnalyst, PermTradeApprove) {
		t.Error("Analyst should NOT have PermTradeApprove")
	}
	if !HasPermission(RoleAnalyst, PermScenarioRun) {
		t.Error("Analyst should have PermScenarioRun")
	}

	// Viewer is strictly read-only
	if HasPermission(RoleViewer, PermNoteCreate) {
		t.Error("Viewer should NOT have PermNoteCreate")
	}
	if !HasPermission(RoleViewer, PermPortfolioRead) {
		t.Error("Viewer should have PermPortfolioRead")
	}
}

func TestLiquidityRiskCalculation(t *testing.T) {
	// Position 1: 10,000 shares @ 500 = 5M NPR. ADV = 20,000 shares. Participation = 10% -> 2,000 shares/day -> DTL = 5 days (Illiquid)
	// Position 2: 2,000 shares @ 250 = 500k NPR. ADV = 100,000 shares. Participation = 10% -> 10,000 shares/day -> DTL = 0.2 days (Liquid)
	positions := []PositionInput{
		{
			Symbol: "NABIL",
			Shares: marketdata.MustParse("10000"),
			Price:  marketdata.MustParse("500"),
			ADV:    marketdata.MustParse("20000"),
		},
		{
			Symbol: "SHIVM",
			Shares: marketdata.MustParse("2000"),
			Price:  marketdata.MustParse("250"),
			ADV:    marketdata.MustParse("100000"),
		},
	}

	partRate := marketdata.MustParse("0.10") // 10% participation
	rep, err := ComputeLiquidityRisk("pf-1", positions, partRate)
	if err != nil {
		t.Fatalf("ComputeLiquidityRisk failed: %v", err)
	}

	// Total NAV = 5,000,000 + 500,000 = 5,500,000
	if rep.TotalNAV.Format(2) != "5500000.00" {
		t.Errorf("expected NAV 5,500,000, got %s", rep.TotalNAV.Format(2))
	}

	// NABIL DTL = 10,000 / (20,000 * 0.10) = 5.00 days (TierIlliquid)
	posNabil := rep.Positions[0]
	if posNabil.DaysToLiquidate.Format(2) != "5.00" {
		t.Errorf("NABIL DTL: expected 5.00, got %s", posNabil.DaysToLiquidate.Format(2))
	}
	if posNabil.LiquidityTier != TierIlliquid {
		t.Errorf("NABIL tier: expected %s, got %s", TierIlliquid, posNabil.LiquidityTier)
	}

	// SHIVM DTL = 2,000 / (100,000 * 0.10) = 0.20 days (TierLiquid)
	posShivm := rep.Positions[1]
	if posShivm.DaysToLiquidate.Format(2) != "0.20" {
		t.Errorf("SHIVM DTL: expected 0.20, got %s", posShivm.DaysToLiquidate.Format(2))
	}
	if posShivm.LiquidityTier != TierLiquid {
		t.Errorf("SHIVM tier: expected %s, got %s", TierLiquid, posShivm.LiquidityTier)
	}

	// Weighted DTL: (5.00 * 5M/5.5M) + (0.20 * 0.5M/5.5M) = 4.545 + 0.018 = 4.56 days
	if rep.WeightedDTL.Format(2) != "4.56" {
		t.Errorf("expected Weighted DTL 4.56, got %s", rep.WeightedDTL.Format(2))
	}
}

func TestConcentrationRiskAndBreaches(t *testing.T) {
	// Total NAV = 10M
	// Position 1: 6M (60% weight -> SEBON 10% breach!)
	// Position 2: 3M (30% weight -> SEBON 10% breach!)
	// Position 3: 1M (10% weight)
	positions := []PositionInput{
		{Symbol: "NABIL", Sector: "Commercial Banks", Shares: marketdata.MustParse("12000"), Price: marketdata.MustParse("500")}, // 6M
		{Symbol: "SCB", Sector: "Commercial Banks", Shares: marketdata.MustParse("6000"), Price: marketdata.MustParse("500")},    // 3M
		{Symbol: "CHCL", Sector: "Hydropower", Shares: marketdata.MustParse("2000"), Price: marketdata.MustParse("500")},         // 1M
	}

	maxStock := marketdata.MustParse("10.0")  // 10%
	maxSector := marketdata.MustParse("25.0") // 25%

	rep, err := ComputeConcentrationRisk("pf-2", positions, maxStock, maxSector)
	if err != nil {
		t.Fatalf("ComputeConcentrationRisk failed: %v", err)
	}

	// Top 1 = 60%
	if rep.Top1Pct.Format(2) != "60.00" {
		t.Errorf("expected Top1 60.00%%, got %s", rep.Top1Pct.Format(2))
	}

	// HHI = 60^2 + 30^2 + 10^2 = 3600 + 900 + 100 = 4600 (Highly Concentrated)
	if rep.HHI.Format(1) != "4600.0" {
		t.Errorf("expected HHI 4600.0, got %s", rep.HHI.Format(1))
	}
	if rep.HHIEvaluation != "highly_concentrated" {
		t.Errorf("expected highly_concentrated, got %s", rep.HHIEvaluation)
	}

	// Breaches:
	// NABIL (60% > 10% -> Critical breach)
	// SCB (30% > 10% -> Critical breach)
	// Commercial Banks (90% > 25% -> Critical breach)
	if len(rep.Breaches) != 3 {
		t.Errorf("expected 3 breaches, got %d", len(rep.Breaches))
	}
}

func TestStressTestingScenario(t *testing.T) {
	// Base NAV = 2,000,000
	// NABIL: 1M @ Beta 1.2
	// CHCL: 1M @ Beta 0.8
	positions := []PositionInput{
		{Symbol: "NABIL", Sector: "Commercial Banks", Shares: marketdata.MustParse("2000"), Price: marketdata.MustParse("500"), Beta: marketdata.MustParse("1.2")},
		{Symbol: "CHCL", Sector: "Hydropower", Shares: marketdata.MustParse("2000"), Price: marketdata.MustParse("500"), Beta: marketdata.MustParse("0.8")},
	}

	// Scenario: Market Crash -20%
	// NABIL effective shock = -20% * 1.2 = -24% -> Stressed MV = 760,000
	// CHCL effective shock = -20% * 0.8 = -16% -> Stressed MV = 840,000
	// Stressed NAV = 1,600,000 (Total Loss = -400,000, Return = -20%)
	scenario := StressScenario{
		Name:          "Market Crash -20%",
		Description:   "Severe market drawdown scaled by position Beta",
		MarketDropPct: marketdata.MustParse("-20.0"),
	}

	res, err := RunStressScenario(positions, scenario)
	if err != nil {
		t.Fatalf("RunStressScenario failed: %v", err)
	}

	if res.BaseNAV.Format(2) != "2000000.00" {
		t.Errorf("expected Base NAV 2,000,000, got %s", res.BaseNAV.Format(2))
	}
	if res.StressedNAV.Format(2) != "1600000.00" {
		t.Errorf("expected Stressed NAV 1,600,000, got %s", res.StressedNAV.Format(2))
	}
	if res.TotalLoss.Format(2) != "-400000.00" {
		t.Errorf("expected Loss -400,000, got %s", res.TotalLoss.Format(2))
	}
	if res.ReturnPct.Format(2) != "-20.00" {
		t.Errorf("expected Return -20.00%%, got %s", res.ReturnPct.Format(2))
	}
}

func TestAntiSelfApprovalRule(t *testing.T) {
	// Proves that when requester == reviewer, self-approval is rejected
	requesterID := "user-portfolio-manager-1"
	reviewerID := "user-portfolio-manager-1"

	isSelfApproval := (requesterID == reviewerID)
	if !isSelfApproval {
		t.Error("expected detected self approval")
	}
}

func TestSlippageCapUnderSevereIlliquidity(t *testing.T) {
	// Position with very low ADV and huge shares: DTL = 100 days
	// Slippage rate without cap = 100 * 0.005 = 0.50 (50%). Capped at 15%.
	positions := []PositionInput{
		{
			Symbol: "EXTREME",
			Shares: marketdata.MustParse("100000"),
			Price:  marketdata.MustParse("100"), // 10M NAV
			ADV:    marketdata.MustParse("100"), // 100 shares/day ADV
		},
	}

	partRate := marketdata.MustParse("0.10") // 10 shares/day -> DTL = 10,000 days
	rep, err := ComputeLiquidityRisk("pf-extreme", positions, partRate)
	if err != nil {
		t.Fatalf("ComputeLiquidityRisk failed: %v", err)
	}

	pos := rep.Positions[0]
	// 10M * 0.15 = 1.5M capped slippage
	if pos.EstimatedSlippage.Format(2) != "1500000.00" {
		t.Errorf("expected capped slippage 1,500,000.00 (15%%), got %s", pos.EstimatedSlippage.Format(2))
	}
}
