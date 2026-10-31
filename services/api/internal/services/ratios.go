// Package ratios computes reporting-period-aware financial ratios using exact NUMERIC arithmetic.
// It links market price observations to specific reporting periods without look-ahead bias.
package services

import (
	"fmt"
	"nepse/terminal/api/internal/marketdata"
)

// AnnualizationFactor returns the multiplier to annualize a quarterly flow metric.
func AnnualizationFactor(pt PeriodType) marketdata.Decimal {
	switch pt {
	case PeriodQ1:
		return marketdata.MustParse("4.0")
	case PeriodQ2:
		return marketdata.MustParse("2.0")
	case PeriodQ3:
		// Q3 is 9 months, annualize by multiplying by 4/3 = 1.333333...
		d, _ := marketdata.MustParse("4").Div(marketdata.MustParse("3"))
		return d
	default: // Q4, ANNUAL, TTM
		return marketdata.One()
	}
}

// PeriodRatios contains all calculated financial ratios for a given period and market price.
type PeriodRatios struct {
	Symbol     string             `json:"symbol"`
	FiscalYear string             `json:"fiscal_year"`
	PeriodType PeriodType         `json:"period_type"`
	PeriodEnd  string             `json:"period_end"`
	AsOfDate   string             `json:"as_of_date"`
	PriceUsed  marketdata.Decimal `json:"price_used"`
	MarketCap  marketdata.Decimal `json:"market_cap"`

	// Per share values
	EPS               marketdata.Decimal `json:"eps"`
	AnnualizedEPS     marketdata.Decimal `json:"annualized_eps"`
	BookValuePerShare marketdata.Decimal `json:"book_value_per_share"`
	CashPerShare      marketdata.Decimal `json:"cash_per_share"`

	// Valuation ratios
	PE              marketdata.Decimal `json:"pe"`
	PB              marketdata.Decimal `json:"pb"`
	PS              marketdata.Decimal `json:"ps"`
	EnterpriseValue marketdata.Decimal `json:"enterprise_value"`
	EVToEBITDA      marketdata.Decimal `json:"ev_to_ebitda"`
	EarningsYield   marketdata.Decimal `json:"earnings_yield"`

	// Profitability ratios
	ROE             marketdata.Decimal `json:"roe"`
	ROA             marketdata.Decimal `json:"roa"`
	GrossMargin     marketdata.Decimal `json:"gross_margin"`
	OperatingMargin marketdata.Decimal `json:"operating_margin"`
	NetMargin       marketdata.Decimal `json:"net_margin"`

	// Solvency & Liquidity ratios
	CurrentRatio     marketdata.Decimal `json:"current_ratio"`
	QuickRatio       marketdata.Decimal `json:"quick_ratio"`
	DebtToEquity     marketdata.Decimal `json:"debt_to_equity"`
	DebtToAssets     marketdata.Decimal `json:"debt_to_assets"`
	InterestCoverage marketdata.Decimal `json:"interest_coverage"`

	// Efficiency
	AssetTurnover marketdata.Decimal `json:"asset_turnover"`

	FormulaNotes map[string]string `json:"formula_notes,omitempty"`
}

// ComputeRatios calculates reporting-period-aware ratios using the given statement and market price.
func ComputeRatios(stmt *Statement, marketPrice marketdata.Decimal, asOfDate string) (*PeriodRatios, error) {
	if stmt == nil {
		return nil, fmt.Errorf("financial statement is required")
	}

	p := stmt.Period
	r := &PeriodRatios{
		Symbol:       p.Symbol,
		FiscalYear:   p.FiscalYear,
		PeriodType:   p.PeriodType,
		PeriodEnd:    p.PeriodEnd,
		AsOfDate:     asOfDate,
		PriceUsed:    marketPrice,
		FormulaNotes: make(map[string]string),
	}

	// 1. Shares & Market Cap
	shares := stmt.GetLine(LineTotalSharesOutstanding)
	if shares.IsPositive() {
		r.MarketCap = marketPrice.Mul(shares)
	}

	// 2. Per Share metrics
	netIncome := stmt.GetLine(LineNetIncome)
	equity := stmt.GetLine(LineShareholdersEquity)
	cash := stmt.GetLine(LineCashAndEquivalents)
	revenue := stmt.GetLine(LineRevenue)
	totalAssets := stmt.GetLine(LineTotalAssets)
	currentAssets := stmt.GetLine(LineCurrentAssets)
	currentLiabilities := stmt.GetLine(LineCurrentLiabilities)
	totalDebt := stmt.GetLine(LineTotalDebt)
	grossProfit := stmt.GetLine(LineGrossProfit)
	operatingIncome := stmt.GetLine(LineOperatingIncome)
	interestExpense := stmt.GetLine(LineInterestExpense)

	factor := AnnualizationFactor(p.PeriodType)
	annualizedNetIncome := netIncome.Mul(factor)
	annualizedRevenue := revenue.Mul(factor)

	if shares.IsPositive() {
		// EPS
		eps, err := netIncome.Div(shares)
		if err == nil {
			r.EPS = eps
			r.AnnualizedEPS = eps.Mul(factor)
			r.FormulaNotes["eps"] = "Net Income / Shares Outstanding"
			r.FormulaNotes["annualized_eps"] = fmt.Sprintf("EPS * %s (Annualization Factor for %s)", factor.Format(2), p.PeriodType)
		}

		// Book Value Per Share
		bvps, err := equity.Div(shares)
		if err == nil {
			r.BookValuePerShare = bvps
			r.FormulaNotes["book_value_per_share"] = "Shareholders Equity / Shares Outstanding"
		}

		// Cash Per Share
		if cash.IsPositive() {
			cps, err := cash.Div(shares)
			if err == nil {
				r.CashPerShare = cps
				r.FormulaNotes["cash_per_share"] = "Cash & Equivalents / Shares Outstanding"
			}
		}
	}

	// 3. Valuation Ratios
	if marketPrice.IsPositive() {
		// P/E (Price to Annualized EPS)
		if r.AnnualizedEPS.IsPositive() {
			pe, err := marketPrice.Div(r.AnnualizedEPS)
			if err == nil {
				r.PE = pe
				r.FormulaNotes["pe"] = "Market Price / Annualized EPS"
			}
		}

		// P/B (Price to Book)
		if r.BookValuePerShare.IsPositive() {
			pb, err := marketPrice.Div(r.BookValuePerShare)
			if err == nil {
				r.PB = pb
				r.FormulaNotes["pb"] = "Market Price / Book Value Per Share"
			}
		}

		// P/S (Price to Sales)
		if r.MarketCap.IsPositive() && annualizedRevenue.IsPositive() {
			ps, err := r.MarketCap.Div(annualizedRevenue)
			if err == nil {
				r.PS = ps
				r.FormulaNotes["ps"] = "Market Capitalization / Annualized Revenue"
			}
		}

		// Earnings Yield
		if r.AnnualizedEPS.IsPositive() {
			ey, err := r.AnnualizedEPS.Div(marketPrice)
			if err == nil {
				r.EarningsYield = ey
				r.FormulaNotes["earnings_yield"] = "Annualized EPS / Market Price"
			}
		}
	}

	// Enterprise Value = Market Cap + Total Debt - Cash
	if r.MarketCap.IsPositive() {
		ev := r.MarketCap.Add(totalDebt).Sub(cash)
		if ev.IsPositive() {
			r.EnterpriseValue = ev
			r.FormulaNotes["enterprise_value"] = "Market Cap + Total Debt - Cash & Cash Equivalents"

			// EV/EBITDA
			ebitda := operatingIncome.Mul(factor)
			if ebitda.IsPositive() {
				evEbitda, err := ev.Div(ebitda)
				if err == nil {
					r.EVToEBITDA = evEbitda
					r.FormulaNotes["ev_to_ebitda"] = "Enterprise Value / Annualized Operating Income (EBITDA proxy)"
				}
			}
		}
	}

	// 4. Profitability Ratios
	if equity.IsPositive() {
		// ROE = Annualized Net Income / Shareholders Equity
		roe, err := annualizedNetIncome.Div(equity)
		if err == nil {
			r.ROE = roe
			r.FormulaNotes["roe"] = "Annualized Net Income / Shareholders Equity"
		}
	}

	if totalAssets.IsPositive() {
		// ROA = Annualized Net Income / Total Assets
		roa, err := annualizedNetIncome.Div(totalAssets)
		if err == nil {
			r.ROA = roa
			r.FormulaNotes["roa"] = "Annualized Net Income / Total Assets"
		}

		// Asset Turnover = Annualized Revenue / Total Assets
		if annualizedRevenue.IsPositive() {
			at, err := annualizedRevenue.Div(totalAssets)
			if err == nil {
				r.AssetTurnover = at
				r.FormulaNotes["asset_turnover"] = "Annualized Revenue / Total Assets"
			}
		}
	}

	if revenue.IsPositive() {
		// Margins
		if grossProfit.IsPositive() {
			gm, err := grossProfit.Div(revenue)
			if err == nil {
				r.GrossMargin = gm
				r.FormulaNotes["gross_margin"] = "Gross Profit / Revenue"
			}
		}
		if operatingIncome.IsPositive() {
			om, err := operatingIncome.Div(revenue)
			if err == nil {
				r.OperatingMargin = om
				r.FormulaNotes["operating_margin"] = "Operating Income / Revenue"
			}
		}
		if netIncome.IsPositive() {
			nm, err := netIncome.Div(revenue)
			if err == nil {
				r.NetMargin = nm
				r.FormulaNotes["net_margin"] = "Net Income / Revenue"
			}
		}
	}

	// 5. Solvency & Liquidity Ratios
	if currentLiabilities.IsPositive() {
		// Current Ratio = Current Assets / Current Liabilities
		cr, err := currentAssets.Div(currentLiabilities)
		if err == nil {
			r.CurrentRatio = cr
			r.FormulaNotes["current_ratio"] = "Current Assets / Current Liabilities"
		}

		// Quick Ratio = (Cash & Equivalents + Investments) / Current Liabilities
		investments := stmt.GetLine(LineInvestments)
		quickAssets := cash.Add(investments)
		if quickAssets.IsPositive() {
			qr, err := quickAssets.Div(currentLiabilities)
			if err == nil {
				r.QuickRatio = qr
				r.FormulaNotes["quick_ratio"] = "(Cash + Investments) / Current Liabilities"
			}
		}
	}

	if equity.IsPositive() && totalDebt.IsPositive() {
		// Debt to Equity
		de, err := totalDebt.Div(equity)
		if err == nil {
			r.DebtToEquity = de
			r.FormulaNotes["debt_to_equity"] = "Total Debt / Shareholders Equity"
		}
	}

	if totalAssets.IsPositive() && totalDebt.IsPositive() {
		// Debt to Assets
		da, err := totalDebt.Div(totalAssets)
		if err == nil {
			r.DebtToAssets = da
			r.FormulaNotes["debt_to_assets"] = "Total Debt / Total Assets"
		}
	}

	if interestExpense.IsPositive() && operatingIncome.IsPositive() {
		// Interest Coverage = Operating Income / Interest Expense
		ic, err := operatingIncome.Div(interestExpense)
		if err == nil {
			r.InterestCoverage = ic
			r.FormulaNotes["interest_coverage"] = "Operating Income / Interest Expense"
		}
	}

	return r, nil
}
