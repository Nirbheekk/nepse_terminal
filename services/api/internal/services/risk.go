package services

import (
	"errors"
	"sort"

	"nepse/terminal/api/internal/marketdata"
)

type LiquidityTier string

const (
	TierLiquid   LiquidityTier = "liquid"   // <= 1.0 day
	TierModerate LiquidityTier = "moderate" // 1.0 to 3.0 days
	TierIlliquid LiquidityTier = "illiquid" // 3.0 to 10.0 days
	TierSevere   LiquidityTier = "severe"   // > 10.0 days
)

// PositionLiquidity represents liquidity risk analysis for a single position.
type PositionLiquidity struct {
	Symbol            string             `json:"symbol"`
	SharesHeld        marketdata.Decimal `json:"shares_held"`
	MarketPrice       marketdata.Decimal `json:"market_price"`
	MarketValue       marketdata.Decimal `json:"market_value"`
	WeightPct         marketdata.Decimal `json:"weight_pct"`
	ADV               marketdata.Decimal `json:"adv"`                // Average Daily Volume
	ParticipationRate marketdata.Decimal `json:"participation_rate"` // e.g. 0.15 (15%)
	DaysToLiquidate   marketdata.Decimal `json:"days_to_liquidate"`
	LiquidityTier     LiquidityTier      `json:"liquidity_tier"`
	EstimatedSlippage marketdata.Decimal `json:"estimated_slippage"` // Estimated slippage cost in NPR
}

// PortfolioLiquidityReport summarizes portfolio liquidity risk.
type PortfolioLiquidityReport struct {
	PortfolioID string              `json:"portfolio_id"`
	TotalNAV    marketdata.Decimal  `json:"total_nav"`
	WeightedDTL marketdata.Decimal  `json:"weighted_dtl"` // Weighted Average Days to Liquidate
	LiquidNAV   marketdata.Decimal  `json:"liquid_nav"`   // NAV in Tier 1 & 2
	IlliquidNAV marketdata.Decimal  `json:"illiquid_nav"` // NAV in Tier 3 & 4
	IlliquidPct marketdata.Decimal  `json:"illiquid_pct"`
	Positions   []PositionLiquidity `json:"positions"`
}

// LimitBreach records a concentration or regulatory threshold violation.
type LimitBreach struct {
	Subject      string             `json:"subject"` // Symbol or Sector
	Kind         string             `json:"kind"`    // single_stock or sector
	ObservedPct  marketdata.Decimal `json:"observed_pct"`
	LimitPct     marketdata.Decimal `json:"limit_pct"`
	ExcessAmount marketdata.Decimal `json:"excess_amount"`
	Severity     string             `json:"severity"` // warning or critical
}

// ConcentrationReport evaluates concentration risk and regulatory limits.
type ConcentrationReport struct {
	PortfolioID   string                        `json:"portfolio_id"`
	TotalNAV      marketdata.Decimal            `json:"total_nav"`
	Top1Pct       marketdata.Decimal            `json:"top1_pct"`
	Top3Pct       marketdata.Decimal            `json:"top3_pct"`
	Top5Pct       marketdata.Decimal            `json:"top5_pct"`
	Top10Pct      marketdata.Decimal            `json:"top10_pct"`
	HHI           marketdata.Decimal            `json:"hhi"`            // Herfindahl-Hirschman Index
	HHIEvaluation string                        `json:"hhi_evaluation"` // unconcentrated, moderate, high
	SectorWeights map[string]marketdata.Decimal `json:"sector_weights"`
	Breaches      []LimitBreach                 `json:"breaches,omitempty"`
}

// PositionInput provides position data for risk analysis.
type PositionInput struct {
	Symbol string
	Sector string
	Shares marketdata.Decimal
	Price  marketdata.Decimal
	ADV    marketdata.Decimal
	Beta   marketdata.Decimal
}

// ComputeLiquidityRisk analyzes liquidity metrics across portfolio positions.
func ComputeLiquidityRisk(portfolioID string, positions []PositionInput, participationRate marketdata.Decimal) (*PortfolioLiquidityReport, error) {
	if participationRate.IsZero() || participationRate.IsNegative() {
		participationRate = marketdata.MustParse("0.15") // 15% default max market participation
	}

	report := &PortfolioLiquidityReport{
		PortfolioID: portfolioID,
	}

	totalNAV := marketdata.Zero()
	for _, p := range positions {
		mv := p.Shares.Mul(p.Price)
		totalNAV = totalNAV.Add(mv)
	}
	report.TotalNAV = totalNAV

	if totalNAV.IsZero() {
		return report, nil
	}

	hundred := marketdata.MustParse("100")
	oneDay := marketdata.MustParse("1.0")
	threeDays := marketdata.MustParse("3.0")
	tenDays := marketdata.MustParse("10.0")

	weightedDTLSum := marketdata.Zero()
	liquidNAV := marketdata.Zero()
	illiquidNAV := marketdata.Zero()

	for _, p := range positions {
		mv := p.Shares.Mul(p.Price)
		wt, _ := mv.Div(totalNAV)
		wtPct := wt.Mul(hundred).Round(2)

		// Days to liquidate: Shares / (ADV * participationRate)
		dailyCapacity := p.ADV.Mul(participationRate)
		dtl := marketdata.Zero()
		if dailyCapacity.IsPositive() {
			dtl = p.Shares.Quo(dailyCapacity).Round(2)
		} else if p.Shares.IsPositive() {
			// Zero volume: extremely illiquid (assign 99 days)
			dtl = marketdata.MustParse("99.00")
		}

		var tier LiquidityTier
		if dtl.LessThanOrEqual(oneDay) {
			tier = TierLiquid
			liquidNAV = liquidNAV.Add(mv)
		} else if dtl.LessThanOrEqual(threeDays) {
			tier = TierModerate
			liquidNAV = liquidNAV.Add(mv)
		} else if dtl.LessThanOrEqual(tenDays) {
			tier = TierIlliquid
			illiquidNAV = illiquidNAV.Add(mv)
		} else {
			tier = TierSevere
			illiquidNAV = illiquidNAV.Add(mv)
		}

		// Slippage estimate: (DTL * 0.005) * MarketValue (0.50% per day to liquidate)
		slippageRate := dtl.Mul(marketdata.MustParse("0.005"))
		if slippageRate.GreaterThan(marketdata.MustParse("0.15")) {
			slippageRate = marketdata.MustParse("0.15") // Cap at 15%
		}
		slippageCost := mv.Mul(slippageRate).Round(2)

		weightedDTLSum = weightedDTLSum.Add(dtl.Mul(wt))

		report.Positions = append(report.Positions, PositionLiquidity{
			Symbol:            p.Symbol,
			SharesHeld:        p.Shares,
			MarketPrice:       p.Price,
			MarketValue:       mv,
			WeightPct:         wtPct,
			ADV:               p.ADV,
			ParticipationRate: participationRate,
			DaysToLiquidate:   dtl,
			LiquidityTier:     tier,
			EstimatedSlippage: slippageCost,
		})
	}

	report.WeightedDTL = weightedDTLSum.Round(2)
	report.LiquidNAV = liquidNAV
	report.IlliquidNAV = illiquidNAV
	if totalNAV.IsPositive() {
		report.IlliquidPct = illiquidNAV.Quo(totalNAV).Mul(hundred).Round(2)
	}

	return report, nil
}

// ComputeConcentrationRisk evaluates top-holding concentration, HHI, and sector limits.
func ComputeConcentrationRisk(portfolioID string, positions []PositionInput, maxStockPct, maxSectorPct marketdata.Decimal) (*ConcentrationReport, error) {
	if maxStockPct.IsZero() {
		maxStockPct = marketdata.MustParse("10.0") // SEBON standard 10% single stock limit
	}
	if maxSectorPct.IsZero() {
		maxSectorPct = marketdata.MustParse("25.0") // Standard 25% sector limit
	}

	report := &ConcentrationReport{
		PortfolioID:   portfolioID,
		SectorWeights: make(map[string]marketdata.Decimal),
	}

	totalNAV := marketdata.Zero()
	for _, p := range positions {
		mv := p.Shares.Mul(p.Price)
		totalNAV = totalNAV.Add(mv)
	}
	report.TotalNAV = totalNAV

	if totalNAV.IsZero() {
		return report, nil
	}

	hundred := marketdata.MustParse("100")
	type posWeight struct {
		symbol string
		sector string
		mv     marketdata.Decimal
		wtPct  marketdata.Decimal
	}

	weights := make([]posWeight, len(positions))
	sectorMVs := make(map[string]marketdata.Decimal)

	for i, p := range positions {
		mv := p.Shares.Mul(p.Price)
		wtPct := mv.Quo(totalNAV).Mul(hundred).Round(4)
		sec := p.Sector
		if sec == "" {
			sec = "Other"
		}
		weights[i] = posWeight{symbol: p.Symbol, sector: sec, mv: mv, wtPct: wtPct}
		sectorMVs[sec] = sectorMVs[sec].Add(mv)
	}

	// Sort descending by weight
	sort.Slice(weights, func(i, j int) bool {
		return weights[i].wtPct.GreaterThan(weights[j].wtPct)
	})

	// Top N calculations
	cum := marketdata.Zero()
	hhiSum := marketdata.Zero()

	for i, w := range weights {
		cum = cum.Add(w.wtPct)
		if i == 0 {
			report.Top1Pct = cum.Round(2)
		}
		if i == 2 || (i == len(weights)-1 && i < 2) {
			report.Top3Pct = cum.Round(2)
		}
		if i == 4 || (i == len(weights)-1 && i < 4) {
			report.Top5Pct = cum.Round(2)
		}
		if i == 9 || (i == len(weights)-1 && i < 9) {
			report.Top10Pct = cum.Round(2)
		}

		// HHI = sum(weightPct^2)
		hhiSum = hhiSum.Add(w.wtPct.Mul(w.wtPct))

		// Check single stock limit
		if w.wtPct.GreaterThan(maxStockPct) {
			excessPct := w.wtPct.Sub(maxStockPct)
			excessAmt := totalNAV.Mul(excessPct).Quo(hundred).Round(2)
			severity := "warning"
			if w.wtPct.GreaterThan(maxStockPct.Mul(marketdata.MustParse("1.25"))) {
				severity = "critical"
			}
			report.Breaches = append(report.Breaches, LimitBreach{
				Subject:      w.symbol,
				Kind:         "single_stock",
				ObservedPct:  w.wtPct.Round(2),
				LimitPct:     maxStockPct,
				ExcessAmount: excessAmt,
				Severity:     severity,
			})
		}
	}

	report.HHI = hhiSum.Round(1)
	if hhiSum.LessThan(marketdata.MustParse("1500")) {
		report.HHIEvaluation = "unconcentrated"
	} else if hhiSum.LessThanOrEqual(marketdata.MustParse("2500")) {
		report.HHIEvaluation = "moderately_concentrated"
	} else {
		report.HHIEvaluation = "highly_concentrated"
	}

	// Sector weights and limits
	for sec, smv := range sectorMVs {
		secPct := smv.Quo(totalNAV).Mul(hundred).Round(2)
		report.SectorWeights[sec] = secPct

		if secPct.GreaterThan(maxSectorPct) {
			excessPct := secPct.Sub(maxSectorPct)
			excessAmt := totalNAV.Mul(excessPct).Quo(hundred).Round(2)
			severity := "warning"
			if secPct.GreaterThan(maxSectorPct.Mul(marketdata.MustParse("1.25"))) {
				severity = "critical"
			}
			report.Breaches = append(report.Breaches, LimitBreach{
				Subject:      sec,
				Kind:         "sector",
				ObservedPct:  secPct,
				LimitPct:     maxSectorPct,
				ExcessAmount: excessAmt,
				Severity:     severity,
			})
		}
	}

	return report, nil
}

// StressScenario defines a market or sector shock parameters.
type StressScenario struct {
	Name           string                        `json:"name"`
	Description    string                        `json:"description"`
	MarketDropPct  marketdata.Decimal            `json:"market_drop_pct"`         // e.g. -20.0
	SectorShocks   map[string]marketdata.Decimal `json:"sector_shocks,omitempty"` // Sector -> Shock %
	CustomShocks   map[string]marketdata.Decimal `json:"custom_shocks,omitempty"` // Symbol -> Shock %
	LiquidityShock bool                          `json:"liquidity_shock"`         // If true, adds slippage impact
}

// PositionStressOutcome shows position-level stress impact.
type PositionStressOutcome struct {
	Symbol         string             `json:"symbol"`
	BaseMV         marketdata.Decimal `json:"base_mv"`
	EffectiveShock marketdata.Decimal `json:"effective_shock_pct"`
	StressedMV     marketdata.Decimal `json:"stressed_mv"`
	DollarPnL      marketdata.Decimal `json:"dollar_pnl"`
}

// ScenarioResult summarizes portfolio outcome under stress.
type ScenarioResult struct {
	ScenarioName     string                  `json:"scenario_name"`
	Description      string                  `json:"description"`
	BaseNAV          marketdata.Decimal      `json:"base_nav"`
	StressedNAV      marketdata.Decimal      `json:"stressed_nav"`
	TotalLoss        marketdata.Decimal      `json:"total_loss"`
	ReturnPct        marketdata.Decimal      `json:"return_pct"`
	PositionOutcomes []PositionStressOutcome `json:"position_outcomes"`
}

// RunStressScenario executes scenario stress-testing on portfolio positions.
func RunStressScenario(positions []PositionInput, scenario StressScenario) (*ScenarioResult, error) {
	if len(positions) == 0 {
		return nil, errors.New("no positions to stress test")
	}

	hundred := marketdata.MustParse("100")
	baseNAV := marketdata.Zero()
	stressedNAV := marketdata.Zero()
	outcomes := make([]PositionStressOutcome, len(positions))

	for i, p := range positions {
		mv := p.Shares.Mul(p.Price)
		baseNAV = baseNAV.Add(mv)

		// Determine shock percentage for this position:
		// 1. Explicit custom shock
		// 2. Sector shock
		// 3. Market shock scaled by position Beta
		var shockPct marketdata.Decimal

		if s, ok := scenario.CustomShocks[p.Symbol]; ok {
			shockPct = s
		} else if s, ok := scenario.SectorShocks[p.Sector]; ok {
			shockPct = s
		} else if !scenario.MarketDropPct.IsZero() {
			beta := p.Beta
			if beta.IsZero() {
				beta = marketdata.One()
			}
			shockPct = scenario.MarketDropPct.Mul(beta).Round(2)
		}

		// If liquidity shock is enabled, add additional slippage penalty (-2.5%)
		if scenario.LiquidityShock {
			shockPct = shockPct.Sub(marketdata.MustParse("2.50"))
		}

		// Floor shock at -100% (equity cannot drop below 0)
		if shockPct.LessThan(marketdata.MustParse("-100.0")) {
			shockPct = marketdata.MustParse("-100.0")
		}

		lossFactor := marketdata.One().Add(shockPct.Quo(hundred))
		stressedMV := mv.Mul(lossFactor).Round(2)
		pnl := stressedMV.Sub(mv)

		stressedNAV = stressedNAV.Add(stressedMV)
		outcomes[i] = PositionStressOutcome{
			Symbol:         p.Symbol,
			BaseMV:         mv,
			EffectiveShock: shockPct,
			StressedMV:     stressedMV,
			DollarPnL:      pnl,
		}
	}

	totalLoss := stressedNAV.Sub(baseNAV)
	returnPct := marketdata.Zero()
	if baseNAV.IsPositive() {
		returnPct = totalLoss.Quo(baseNAV).Mul(hundred).Round(2)
	}

	return &ScenarioResult{
		ScenarioName:     scenario.Name,
		Description:      scenario.Description,
		BaseNAV:          baseNAV,
		StressedNAV:      stressedNAV,
		TotalLoss:        totalLoss,
		ReturnPct:        returnPct,
		PositionOutcomes: outcomes,
	}, nil
}
