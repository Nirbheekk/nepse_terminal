// Package corporateaction defines dividends, stock splits, bonus shares, rights issues,
// and implements historical price adjustment policies with backward-chaining cumulative factors.
package services

import (
	"fmt"
	"sort"

	"nepse/terminal/api/internal/marketdata"
)

type ActionType string

const (
	ActionCashDividend ActionType = "cash_dividend"
	ActionBonusShare   ActionType = "bonus_share"
	ActionRightsIssue  ActionType = "rights_issue"
	ActionStockSplit   ActionType = "stock_split"
	ActionReverseSplit ActionType = "reverse_split"
	ActionMerger       ActionType = "merger"
)

type AdjustmentPolicy string

const (
	PolicyUnadjusted       AdjustmentPolicy = "unadjusted"
	PolicySplitBonus       AdjustmentPolicy = "split_bonus"
	PolicySplitBonusRights AdjustmentPolicy = "split_bonus_rights"
	PolicyTotalReturn      AdjustmentPolicy = "total_return"
)

type ActionStatus string

const (
	StatusAnnounced   ActionStatus = "announced"
	StatusApprovedBOD ActionStatus = "approved_bod"
	StatusApprovedAGM ActionStatus = "approved_agm"
	StatusCompleted   ActionStatus = "completed"
	StatusCancelled   ActionStatus = "cancelled"
)

// CorporateAction describes an exchange or issuer event.
type CorporateAction struct {
	ID               string             `json:"id"`
	CompanyID        string             `json:"company_id"`
	Symbol           string             `json:"symbol"`
	ActionType       ActionType         `json:"action_type"`
	FiscalYear       string             `json:"fiscal_year"`
	AnnouncementDate string             `json:"announcement_date"`
	BookClosureDate  string             `json:"book_closure_date"` // Ex-date
	AGMDate          string             `json:"agm_date"`
	EffectiveDate    string             `json:"effective_date"`
	CashPercent      marketdata.Decimal `json:"cash_percent"`  // Percentage of face value (default par 100)
	CashAmount       marketdata.Decimal `json:"cash_amount"`   // Cash amount per share
	BonusPercent     marketdata.Decimal `json:"bonus_percent"` // Percentage e.g. 10.0 for 10% bonus
	RightsRatio      marketdata.Decimal `json:"rights_ratio"`  // e.g. 0.5 for 1:2 rights
	RightsPrice      marketdata.Decimal `json:"rights_price"`  // Subscription price per share
	SplitFrom        marketdata.Decimal `json:"split_from"`    // e.g. 1
	SplitTo          marketdata.Decimal `json:"split_to"`      // e.g. 2 for 2-for-1 split
	Status           ActionStatus       `json:"status"`
	Notes            string             `json:"notes,omitempty"`
}

// OHLCVBar represents a single trading session observation.
type OHLCVBar struct {
	Date   string             `json:"date"` // YYYY-MM-DD
	Open   marketdata.Decimal `json:"open"`
	High   marketdata.Decimal `json:"high"`
	Low    marketdata.Decimal `json:"low"`
	Close  marketdata.Decimal `json:"close"`
	Volume marketdata.Decimal `json:"volume"`
}

// AdjustedBar represents an adjusted OHLCV observation.
type AdjustedBar struct {
	Date            string             `json:"date"`
	Open            marketdata.Decimal `json:"open"`
	High            marketdata.Decimal `json:"high"`
	Low             marketdata.Decimal `json:"low"`
	Close           marketdata.Decimal `json:"close"`
	Volume          marketdata.Decimal `json:"volume"`
	UnadjustedClose marketdata.Decimal `json:"unadjusted_close"`
	AdjFactor       marketdata.Decimal `json:"adj_factor"`
}

// CalculateActionFactor computes the price multiplier for a corporate action given cum-close price.
// Pre-event prices are multiplied by this factor; pre-event volume is divided by this factor.
func CalculateActionFactor(act CorporateAction, cumClose marketdata.Decimal, policy AdjustmentPolicy) (marketdata.Decimal, error) {
	if policy == PolicyUnadjusted {
		return marketdata.One(), nil
	}

	hundred := marketdata.MustParse("100.0")

	switch act.ActionType {
	case ActionStockSplit:
		// Split 2 for 1: split_from = 1, split_to = 2 => factor = 1 / 2 = 0.5
		if act.SplitTo.IsPositive() && act.SplitFrom.IsPositive() {
			return act.SplitFrom.Div(act.SplitTo)
		}
		return marketdata.One(), nil

	case ActionReverseSplit:
		// Reverse split 1 for 2: split_from = 2, split_to = 1 => factor = 2 / 1 = 2.0
		if act.SplitTo.IsPositive() && act.SplitFrom.IsPositive() {
			return act.SplitFrom.Div(act.SplitTo)
		}
		return marketdata.One(), nil

	case ActionBonusShare:
		// 10% bonus share adds 0.10 shares per existing share.
		// New total shares = 1 + bonus_percent / 100. Factor = 1 / (1 + bonus_percent / 100).
		if act.BonusPercent.IsPositive() {
			bRatio, err := act.BonusPercent.Div(hundred)
			if err != nil {
				return marketdata.One(), err
			}
			denom := marketdata.One().Add(bRatio)
			return marketdata.One().Div(denom)
		}
		return marketdata.One(), nil

	case ActionRightsIssue:
		if policy == PolicySplitBonus {
			// Policy ignores rights
			return marketdata.One(), nil
		}
		// Theoretical Ex-Rights Price (TERP):
		// TERP = (cumClose + r * rightsPrice) / (1 + r)
		// Factor = TERP / cumClose = (cumClose + r * rightsPrice) / ((1 + r) * cumClose)
		if act.RightsRatio.IsPositive() && cumClose.IsPositive() {
			r := act.RightsRatio
			pRights := act.RightsPrice
			numerator := cumClose.Add(r.Mul(pRights))
			onePlusR := marketdata.One().Add(r)
			terp, err := numerator.Div(onePlusR)
			if err != nil {
				return marketdata.One(), err
			}
			return terp.Div(cumClose)
		}
		return marketdata.One(), nil

	case ActionCashDividend:
		if policy != PolicyTotalReturn {
			// Cash dividends only adjusted under total return policy
			return marketdata.One(), nil
		}
		// Cash dividend amount D:
		// Either explicitly specified in CashAmount, or computed as CashPercent% of par (default 100)
		d := act.CashAmount
		if d.IsZero() && act.CashPercent.IsPositive() {
			d, _ = act.CashPercent.Div(hundred) // CashPercent% of par 100 = CashPercent rupees
		}

		if d.IsPositive() && cumClose.GreaterThan(d) {
			// Factor = (cumClose - D) / cumClose
			exPrice := cumClose.Sub(d)
			return exPrice.Div(cumClose)
		}
		return marketdata.One(), nil

	default:
		return marketdata.One(), nil
	}
}

// AdjustPrices adjusts an OHLCV price series according to corporate actions and selected policy.
// bars must be sorted chronologically ascending by Date.
func AdjustPrices(bars []OHLCVBar, actions []CorporateAction, policy AdjustmentPolicy) ([]AdjustedBar, error) {
	if len(bars) == 0 {
		return nil, nil
	}

	result := make([]AdjustedBar, len(bars))
	for i, b := range bars {
		result[i] = AdjustedBar{
			Date:            b.Date,
			Open:            b.Open,
			High:            b.High,
			Low:             b.Low,
			Close:           b.Close,
			Volume:          b.Volume,
			UnadjustedClose: b.Close,
			AdjFactor:       marketdata.One(),
		}
	}

	if policy == PolicyUnadjusted || len(actions) == 0 {
		return result, nil
	}

	// Filter actions that are completed or active with a valid BookClosureDate
	validActions := make([]CorporateAction, 0, len(actions))
	for _, a := range actions {
		if a.BookClosureDate != "" && a.Status != StatusCancelled {
			validActions = append(validActions, a)
		}
	}

	// Sort actions chronologically by BookClosureDate
	sort.Slice(validActions, func(i, j int) bool {
		return validActions[i].BookClosureDate < validActions[j].BookClosureDate
	})

	// Process each corporate action
	for _, act := range validActions {
		// Find the cum-date bar: the last trading bar strictly BEFORE BookClosureDate
		cumIdx := -1
		for i := len(bars) - 1; i >= 0; i-- {
			if bars[i].Date < act.BookClosureDate {
				cumIdx = i
				break
			}
		}

		if cumIdx < 0 {
			// Action book closure date is before our earliest bar; skip
			continue
		}

		cumClose := bars[cumIdx].Close
		factor, err := CalculateActionFactor(act, cumClose, policy)
		if err != nil {
			return nil, fmt.Errorf("action %s factor calculation failed: %w", act.ActionType, err)
		}

		if factor.Equal(marketdata.One()) {
			continue
		}

		// Backward-chain: Multiply factor to all bars on or before cum-date
		for i := 0; i <= cumIdx; i++ {
			result[i].AdjFactor = result[i].AdjFactor.Mul(factor)
			result[i].Open = result[i].Open.Mul(factor)
			result[i].High = result[i].High.Mul(factor)
			result[i].Low = result[i].Low.Mul(factor)
			result[i].Close = result[i].Close.Mul(factor)
			if factor.IsPositive() {
				result[i].Volume = result[i].Volume.Quo(factor)
			}
		}
	}

	return result, nil
}
