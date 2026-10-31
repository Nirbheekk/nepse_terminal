// Package analytics provides benchmark comparison, performance attribution (TWR, MWR/IRR),
// and comprehensive risk analytics (Beta, Alpha, Sharpe, Sortino, Treynor, Drawdowns, VaR, CVaR).
package services

import (
	"errors"
	"math"
	"sort"

	"nepse/terminal/api/internal/marketdata"
)

// DailyObservation holds paired daily portfolio and benchmark data.
type DailyObservation struct {
	Date           string             `json:"date"`
	PortfolioValue marketdata.Decimal `json:"portfolio_value"`
	NetCashFlow    marketdata.Decimal `json:"net_cash_flow"` // Deposits (+) / Withdrawals (-)
	BenchmarkValue marketdata.Decimal `json:"benchmark_value"`
}

// PerformanceMetrics holds all computed performance and benchmark comparison analytics.
type PerformanceMetrics struct {
	TotalDays int    `json:"total_days"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`

	// Returns
	PortfolioTWR    marketdata.Decimal `json:"portfolio_twr"`    // Cumulative Time-Weighted Return
	PortfolioMWR    marketdata.Decimal `json:"portfolio_mwr"`    // Annualized Money-Weighted Return / IRR
	PortfolioCAGR   marketdata.Decimal `json:"portfolio_cagr"`   // Annualized Return
	BenchmarkReturn marketdata.Decimal `json:"benchmark_return"` // Cumulative Benchmark Return
	BenchmarkCAGR   marketdata.Decimal `json:"benchmark_cagr"`   // Annualized Benchmark Return
	ExcessReturn    marketdata.Decimal `json:"excess_return"`    // Portfolio CAGR - Benchmark CAGR

	// Risk
	PortfolioVol     marketdata.Decimal `json:"portfolio_volatility"` // Annualized volatility
	BenchmarkVol     marketdata.Decimal `json:"benchmark_volatility"` // Annualized benchmark volatility
	Beta             marketdata.Decimal `json:"beta"`
	Alpha            marketdata.Decimal `json:"alpha"` // Jensen's Alpha (annualized)
	Correlation      marketdata.Decimal `json:"correlation"`
	SharpeRatio      marketdata.Decimal `json:"sharpe_ratio"`
	SortinoRatio     marketdata.Decimal `json:"sortino_ratio"`
	TreynorRatio     marketdata.Decimal `json:"treynor_ratio"`
	TrackingError    marketdata.Decimal `json:"tracking_error"`
	InformationRatio marketdata.Decimal `json:"information_ratio"`
	UpCapture        marketdata.Decimal `json:"up_capture"`
	DownCapture      marketdata.Decimal `json:"down_capture"`

	// Drawdown
	MaxDrawdown     marketdata.Decimal `json:"max_drawdown"`
	PeakDate        string             `json:"peak_date"`
	TroughDate      string             `json:"trough_date"`
	CurrentDrawdown marketdata.Decimal `json:"current_drawdown"`

	// Value at Risk
	ParametricVaR95 marketdata.Decimal `json:"parametric_var_95"`
	ParametricVaR99 marketdata.Decimal `json:"parametric_var_99"`
	HistoricalVaR95 marketdata.Decimal `json:"historical_var_95"`
	HistoricalVaR99 marketdata.Decimal `json:"historical_var_99"`
	CVaR95          marketdata.Decimal `json:"cvar_95"` // Expected shortfall at 95%
}

// ComputeAnalytics computes benchmark comparison and risk analytics from daily observations.
// riskFreeRate is annual rate (e.g. 0.05 for 5.0%).
func ComputeAnalytics(obs []DailyObservation, riskFreeRate marketdata.Decimal) (*PerformanceMetrics, error) {
	if len(obs) < 2 {
		return nil, errors.New("at least 2 daily observations required to compute analytics")
	}

	// Ensure chronological order
	sorted := make([]DailyObservation, len(obs))
	copy(sorted, obs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date < sorted[j].Date
	})

	N := len(sorted) - 1
	pReturns := make([]float64, N)
	bReturns := make([]float64, N)

	// Compute daily returns
	// Sub-period return: R = (V_t - (V_{t-1} + CF_t)) / (V_{t-1} + CF_t)
	// If CF occurs at start of day, or R = (V_t - CF_t - V_{t-1}) / V_{t-1}
	// We use standard Modified Dietz / sub-period formulation:
	// Base = V_{t-1} + CF_t. If Base > 0: R = (V_t - Base) / Base
	cumTWR := 1.0
	cumBench := 1.0

	for t := 1; t <= N; t++ {
		prevV := sorted[t-1].PortfolioValue.Float64()
		currV := sorted[t].PortfolioValue.Float64()
		cf := sorted[t].NetCashFlow.Float64()

		base := prevV + cf
		var rP float64
		if base > 0 {
			rP = (currV - base) / base
		}
		pReturns[t-1] = rP
		cumTWR *= (1.0 + rP)

		prevB := sorted[t-1].BenchmarkValue.Float64()
		currB := sorted[t].BenchmarkValue.Float64()
		var rB float64
		if prevB > 0 {
			rB = (currB - prevB) / prevB
		}
		bReturns[t-1] = rB
		cumBench *= (1.0 + rB)
	}

	twr := cumTWR - 1.0
	benchTotalReturn := cumBench - 1.0

	// Annualization factor (252 trading days per year)
	tradingDaysPerYear := 252.0
	years := float64(N) / tradingDaysPerYear
	if years <= 0 {
		years = float64(N) / 365.0
	}

	pCAGR := math.Pow(1.0+twr, 1.0/years) - 1.0
	bCAGR := math.Pow(1.0+benchTotalReturn, 1.0/years) - 1.0

	rfAnnual := riskFreeRate.Float64()
	rfDaily := rfAnnual / tradingDaysPerYear

	// Means and standard deviations
	meanP := mean(pReturns)
	meanB := mean(bReturns)
	stdP := stdDev(pReturns, meanP)
	stdB := stdDev(bReturns, meanB)

	annVolP := stdP * math.Sqrt(tradingDaysPerYear)
	annVolB := stdB * math.Sqrt(tradingDaysPerYear)

	// Covariance and Beta
	cov := covariance(pReturns, bReturns, meanP, meanB)
	var beta float64
	var corr float64
	if stdB > 0 {
		varB := stdB * stdB
		beta = cov / varB
		if stdP > 0 {
			corr = cov / (stdP * stdB)
		}
	}

	// Jensen's Alpha: R_p - [R_f + beta * (R_m - R_f)]
	alpha := pCAGR - (rfAnnual + beta*(bCAGR-rfAnnual))

	// Sharpe Ratio: (R_p,ann - R_f) / Vol_p
	var sharpe float64
	if annVolP > 0 {
		sharpe = (pCAGR - rfAnnual) / annVolP
	}

	// Downside deviation and Sortino Ratio
	downsideDev := downsideDeviation(pReturns, rfDaily) * math.Sqrt(tradingDaysPerYear)
	var sortino float64
	if downsideDev > 0 {
		sortino = (pCAGR - rfAnnual) / downsideDev
	}

	// Treynor Ratio: (R_p,ann - R_f) / Beta
	var treynor float64
	if math.Abs(beta) > 1e-6 {
		treynor = (pCAGR - rfAnnual) / beta
	}

	// Tracking Error and Information Ratio
	diffs := make([]float64, N)
	for i := 0; i < N; i++ {
		diffs[i] = pReturns[i] - bReturns[i]
	}
	teDaily := stdDev(diffs, mean(diffs))
	teAnn := teDaily * math.Sqrt(tradingDaysPerYear)
	var infoRatio float64
	if teAnn > 0 {
		infoRatio = (pCAGR - bCAGR) / teAnn
	}

	// Up/Down Market Capture
	upCap, downCap := captureRatios(pReturns, bReturns)

	// Maximum Drawdown
	maxDD, peakDate, troughDate, currDD := computeDrawdowns(sorted)

	// Value at Risk (VaR)
	paramVaR95 := -(meanP - 1.644853*stdP)
	paramVaR99 := -(meanP - 2.326348*stdP)
	histVaR95, cvar95 := historicalVaRAndCVaR(pReturns, 0.05)
	histVaR99, _ := historicalVaRAndCVaR(pReturns, 0.01)

	// Money-Weighted Return (IRR)
	mwr := computeIRR(sorted)

	return &PerformanceMetrics{
		TotalDays:        len(sorted),
		StartDate:        sorted[0].Date,
		EndDate:          sorted[len(sorted)-1].Date,
		PortfolioTWR:     marketdata.NewFromFloat(twr).Round(6),
		PortfolioMWR:     marketdata.NewFromFloat(mwr).Round(6),
		PortfolioCAGR:    marketdata.NewFromFloat(pCAGR).Round(6),
		BenchmarkReturn:  marketdata.NewFromFloat(benchTotalReturn).Round(6),
		BenchmarkCAGR:    marketdata.NewFromFloat(bCAGR).Round(6),
		ExcessReturn:     marketdata.NewFromFloat(pCAGR - bCAGR).Round(6),
		PortfolioVol:     marketdata.NewFromFloat(annVolP).Round(6),
		BenchmarkVol:     marketdata.NewFromFloat(annVolB).Round(6),
		Beta:             marketdata.NewFromFloat(beta).Round(4),
		Alpha:            marketdata.NewFromFloat(alpha).Round(6),
		Correlation:      marketdata.NewFromFloat(corr).Round(4),
		SharpeRatio:      marketdata.NewFromFloat(sharpe).Round(4),
		SortinoRatio:     marketdata.NewFromFloat(sortino).Round(4),
		TreynorRatio:     marketdata.NewFromFloat(treynor).Round(4),
		TrackingError:    marketdata.NewFromFloat(teAnn).Round(6),
		InformationRatio: marketdata.NewFromFloat(infoRatio).Round(4),
		UpCapture:        marketdata.NewFromFloat(upCap).Round(4),
		DownCapture:      marketdata.NewFromFloat(downCap).Round(4),
		MaxDrawdown:      marketdata.NewFromFloat(maxDD).Round(4),
		PeakDate:         peakDate,
		TroughDate:       troughDate,
		CurrentDrawdown:  marketdata.NewFromFloat(currDD).Round(4),
		ParametricVaR95:  marketdata.NewFromFloat(paramVaR95).Round(6),
		ParametricVaR99:  marketdata.NewFromFloat(paramVaR99).Round(6),
		HistoricalVaR95:  marketdata.NewFromFloat(histVaR95).Round(6),
		HistoricalVaR99:  marketdata.NewFromFloat(histVaR99).Round(6),
		CVaR95:           marketdata.NewFromFloat(cvar95).Round(6),
	}, nil
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func stdDev(vals []float64, meanVal float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	sumSq := 0.0
	for _, v := range vals {
		d := v - meanVal
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(vals)-1))
}

func covariance(a, b []float64, meanA, meanB float64) float64 {
	if len(a) < 2 || len(a) != len(b) {
		return 0
	}
	sum := 0.0
	for i := range a {
		sum += (a[i] - meanA) * (b[i] - meanB)
	}
	return sum / float64(len(a)-1)
}

func downsideDeviation(vals []float64, target float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sumSq := 0.0
	for _, v := range vals {
		if v < target {
			diff := v - target
			sumSq += diff * diff
		}
	}
	return math.Sqrt(sumSq / float64(len(vals)))
}

func captureRatios(p, b []float64) (up float64, down float64) {
	upP, upB := 0.0, 0.0
	cntUp := 0
	downP, downB := 0.0, 0.0
	cntDown := 0

	for i := range b {
		if b[i] > 0 {
			upP += p[i]
			upB += b[i]
			cntUp++
		} else if b[i] < 0 {
			downP += p[i]
			downB += b[i]
			cntDown++
		}
	}

	if cntUp > 0 && upB > 0 {
		up = (upP / float64(cntUp)) / (upB / float64(cntUp))
	}
	if cntDown > 0 && downB < 0 {
		down = (downP / float64(cntDown)) / (downB / float64(cntDown))
	}
	return up, down
}

func computeDrawdowns(obs []DailyObservation) (maxDD float64, peakDate, troughDate string, currDD float64) {
	if len(obs) == 0 {
		return 0, "", "", 0
	}

	peak := obs[0].PortfolioValue.Float64()
	tempPeakDate := obs[0].Date

	for _, o := range obs {
		v := o.PortfolioValue.Float64()
		if v > peak {
			peak = v
			tempPeakDate = o.Date
		}
		if peak > 0 {
			dd := (peak - v) / peak
			if dd > maxDD {
				maxDD = dd
				peakDate = tempPeakDate
				troughDate = o.Date
			}
		}
	}

	latestVal := obs[len(obs)-1].PortfolioValue.Float64()
	if peak > 0 {
		currDD = (peak - latestVal) / peak
	}
	return maxDD, peakDate, troughDate, currDD
}

func historicalVaRAndCVaR(returns []float64, alpha float64) (varVal, cvar float64) {
	if len(returns) == 0 {
		return 0, 0
	}
	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	sort.Float64s(sorted)

	idx := int(float64(len(sorted)) * alpha)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}

	varVal = -sorted[idx]

	// CVaR is the mean of losses worse than VaR
	sum := 0.0
	count := 0
	for i := 0; i <= idx; i++ {
		sum += -sorted[i]
		count++
	}
	if count > 0 {
		cvar = sum / float64(count)
	} else {
		cvar = varVal
	}
	return varVal, cvar
}

// computeIRR calculates the annualized Money-Weighted Return using bisection.
func computeIRR(obs []DailyObservation) float64 {
	if len(obs) < 2 {
		return 0
	}

	v0 := obs[0].PortfolioValue.Float64()
	vT := obs[len(obs)-1].PortfolioValue.Float64()
	T := float64(len(obs) - 1)
	if T <= 0 {
		return 0
	}

	// NPV(r) = -v0 - sum(cf_t / (1+r)^(t/252)) + vT / (1+r)^(T/252)
	npv := func(r float64) float64 {
		val := -v0
		for t := 1; t < len(obs); t++ {
			cf := obs[t].NetCashFlow.Float64()
			if cf != 0 {
				timeFrac := float64(t) / 252.0
				val -= cf / math.Pow(1.0+r, timeFrac)
			}
		}
		val += vT / math.Pow(1.0+r, T/252.0)
		return val
	}

	// Bisection search between -90% and +500%
	low := -0.90
	high := 5.0
	fLow := npv(low)
	fHigh := npv(high)

	if fLow*fHigh > 0 {
		// No sign change; return CAGR proxy
		return math.Pow(vT/v0, 252.0/T) - 1.0
	}

	for iter := 0; iter < 60; iter++ {
		mid := (low + high) / 2.0
		fMid := npv(mid)
		if math.Abs(fMid) < 1e-6 {
			return mid
		}
		if fLow*fMid < 0 {
			high = mid
			fHigh = fMid
		} else {
			low = mid
			fLow = fMid
		}
	}
	return (low + high) / 2.0
}
