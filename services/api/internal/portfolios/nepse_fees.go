// Package portfolio implements portfolio tracking, tax lot accounting, cost basis calculation,
// realized and unrealized P&L, and corporate action absorption.
//
// ARCHITECTURAL BOUNDARY:
// Portfolio tracking is strictly decoupled from brokerage execution.
// This package owns post-trade accounting, valuation, performance, and tax lots.
// Order routing, execution gateways, OMS/TMS, and exchange connectivity live in external execution systems.
package portfolios

import (
	"nepse/terminal/api/internal/marketdata"
)

// Standard NEPSE brokerage tiers (effective SEBON directive):
// Amount <= 50,000           -> 0.40%
// 50,000 < Amount <= 500,000 -> 0.37%
// 500,000 < Amount <= 2M    -> 0.34%
// 2M < Amount <= 10M        -> 0.30%
// Amount > 10M              -> 0.27%
func CalculateBrokerCommission(turnover marketdata.Decimal) marketdata.Decimal {
	if turnover.IsZero() || turnover.IsNegative() {
		return marketdata.Zero()
	}

	hundred := marketdata.MustParse("100")
	var rate marketdata.Decimal

	fiftyThousand := marketdata.MustParse("50000")
	fiveLakh := marketdata.MustParse("500000")
	twentyLakh := marketdata.MustParse("2000000")
	oneCrore := marketdata.MustParse("10000000")

	if turnover.LessThanOrEqual(fiftyThousand) {
		rate = marketdata.MustParse("0.40")
	} else if turnover.LessThanOrEqual(fiveLakh) {
		rate = marketdata.MustParse("0.37")
	} else if turnover.LessThanOrEqual(twentyLakh) {
		rate = marketdata.MustParse("0.34")
	} else if turnover.LessThanOrEqual(oneCrore) {
		rate = marketdata.MustParse("0.30")
	} else {
		rate = marketdata.MustParse("0.27")
	}

	comm := turnover.Mul(rate).Quo(hundred)
	// Minimum broker fee Rs 10
	minFee := marketdata.MustParse("10.0")
	if comm.LessThan(minFee) {
		return minFee
	}
	return comm.Round(2)
}

// CalculateSEBONFee computes the 0.015% SEBON regulatory fee.
func CalculateSEBONFee(turnover marketdata.Decimal) marketdata.Decimal {
	if turnover.IsZero() || turnover.IsNegative() {
		return marketdata.Zero()
	}
	rate := marketdata.MustParse("0.00015") // 0.015%
	return turnover.Mul(rate).Round(2)
}

// Flat DP Fee (Demat depository fee) per transaction
func FlatDPFee() marketdata.Decimal {
	return marketdata.MustParse("25.0")
}

type InvestorType string

const (
	InvestorIndividualShortTerm InvestorType = "individual_short_term" // <= 365 days (7.5% CGT)
	InvestorIndividualLongTerm  InvestorType = "individual_long_term"  // > 365 days (5.0% CGT)
	InvestorInstitutional       InvestorType = "institutional"         // (10.0% CGT)
)

// CalculateCGT computes capital gains tax on realized profits.
func CalculateCGT(gain marketdata.Decimal, inv InvestorType) marketdata.Decimal {
	if gain.IsZero() || gain.IsNegative() {
		return marketdata.Zero()
	}

	var rate marketdata.Decimal
	switch inv {
	case InvestorIndividualLongTerm:
		rate = marketdata.MustParse("0.05") // 5%
	case InvestorInstitutional:
		rate = marketdata.MustParse("0.10") // 10%
	case InvestorIndividualShortTerm:
		fallthrough
	default:
		rate = marketdata.MustParse("0.075") // 7.5%
	}

	return gain.Mul(rate).Round(2)
}
