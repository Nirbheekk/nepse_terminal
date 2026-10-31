// Package financials defines normalized company financial statements, line items,
// period definitions, validation rules, and TTM synthesis.
package services

import (
	"errors"
	"fmt"
	"sort"

	"nepse/terminal/api/internal/marketdata"
)

type StatementType string

const (
	TypeBalanceSheet    StatementType = "balance_sheet"
	TypeIncomeStatement StatementType = "income_statement"
	TypeCashFlow        StatementType = "cash_flow"
)

type PeriodType string

const (
	PeriodQ1     PeriodType = "Q1"
	PeriodQ2     PeriodType = "Q2"
	PeriodQ3     PeriodType = "Q3"
	PeriodQ4     PeriodType = "Q4"
	PeriodAnnual PeriodType = "ANNUAL"
	PeriodTTM    PeriodType = "TTM"
)

type AuditStatus string

const (
	StatusUnaudited   AuditStatus = "unaudited"
	StatusAudited     AuditStatus = "audited"
	StatusRestated    AuditStatus = "restated"
	StatusPreliminary AuditStatus = "preliminary"
)

// Standard line item codes
const (
	// Balance Sheet
	LineTotalAssets            = "total_assets"
	LineCurrentAssets          = "current_assets"
	LineCashAndEquivalents     = "cash_and_equivalents"
	LineInvestments            = "investments"
	LineTotalLiabilities       = "total_liabilities"
	LineCurrentLiabilities     = "current_liabilities"
	LineTotalDebt              = "total_debt"
	LineLongTermDebt           = "long_term_debt"
	LineShareholdersEquity     = "shareholders_equity"
	LinePaidUpCapital          = "paid_up_capital"
	LineReservesAndSurplus     = "reserves_and_surplus"
	LineRetainedEarnings       = "retained_earnings"
	LineTotalSharesOutstanding = "total_shares_outstanding"
	LineBookValuePerShare      = "book_value_per_share"

	// Income Statement
	LineRevenue           = "revenue"
	LineCostOfRevenue     = "cost_of_revenue"
	LineGrossProfit       = "gross_profit"
	LineOperatingExpenses = "operating_expenses"
	LineOperatingIncome   = "operating_income"
	LineInterestExpense   = "interest_expense"
	LineTaxExpense        = "tax_expense"
	LineNetIncome         = "net_income"
	LineEPSBasic          = "eps_basic"
	LineEPSDiluted        = "eps_diluted"

	// Cash Flow
	LineOperatingCashFlow = "operating_cash_flow"
	LineInvestingCashFlow = "investing_cash_flow"
	LineFinancingCashFlow = "financing_cash_flow"
	LineCapEx             = "capex"
	LineFreeCashFlow      = "free_cash_flow"
	LineNetChangeInCash   = "net_change_in_cash"
)

// FinancialPeriod identifies a reporting period for a company.
type FinancialPeriod struct {
	ID          string      `json:"id"`
	CompanyID   string      `json:"company_id"`
	Symbol      string      `json:"symbol"`
	FiscalYear  string      `json:"fiscal_year"` // e.g. "2080/81"
	PeriodType  PeriodType  `json:"period_type"` // Q1, Q2, Q3, Q4, ANNUAL, TTM
	PeriodStart string      `json:"period_start"`
	PeriodEnd   string      `json:"period_end"`
	FilingDate  string      `json:"filing_date"`
	AuditStatus AuditStatus `json:"audit_status"`
	Currency    string      `json:"currency"`
	IsTTM       bool        `json:"is_ttm"`
}

// LineItem is a normalized line item in a statement.
type LineItem struct {
	StatementType StatementType      `json:"statement_type"`
	Code          string             `json:"code"`
	Name          string             `json:"name"`
	Amount        marketdata.Decimal `json:"amount"`
	Order         int                `json:"order"`
	Unit          string             `json:"unit"`
}

// Statement contains all normalized line items for a given period.
type Statement struct {
	Period    FinancialPeriod     `json:"period"`
	Lines     map[string]LineItem `json:"lines"`
	Validated bool                `json:"validated"`
	Warnings  []string            `json:"warnings,omitempty"`
}

// NewStatement creates an empty Statement.
func NewStatement(p FinancialPeriod) *Statement {
	if p.Currency == "" {
		p.Currency = "NPR"
	}
	return &Statement{
		Period: p,
		Lines:  make(map[string]LineItem),
	}
}

// SetLine adds or updates a line item.
func (s *Statement) SetLine(st StatementType, code, name string, amount marketdata.Decimal, order int) {
	s.Lines[code] = LineItem{
		StatementType: st,
		Code:          code,
		Name:          name,
		Amount:        amount,
		Order:         order,
		Unit:          s.Period.Currency,
	}
}

// GetLine retrieves the line amount, or zero if missing.
func (s *Statement) GetLine(code string) marketdata.Decimal {
	if item, ok := s.Lines[code]; ok {
		return item.Amount
	}
	return marketdata.Zero()
}

// HasLine checks if the line item exists.
func (s *Statement) HasLine(code string) bool {
	_, ok := s.Lines[code]
	return ok
}

// Validate checks accounting identities and fills derived items if missing.
func (s *Statement) Validate() error {
	s.Warnings = nil
	s.Validated = true

	// 1. Balance Sheet Identity: Assets = Liabilities + Equity
	if s.HasLine(LineTotalAssets) && s.HasLine(LineTotalLiabilities) && s.HasLine(LineShareholdersEquity) {
		assets := s.GetLine(LineTotalAssets)
		liabPlusEquity := s.GetLine(LineTotalLiabilities).Add(s.GetLine(LineShareholdersEquity))
		diff := assets.Sub(liabPlusEquity).Abs()
		if diff.GreaterThan(marketdata.MustParse("1.0")) {
			s.Warnings = append(s.Warnings, fmt.Sprintf("balance sheet imbalance: assets=%s, liabilities+equity=%s, diff=%s",
				assets.String(), liabPlusEquity.String(), diff.String()))
		}
	}

	// 2. Gross profit derivation: if gross_profit is missing but revenue and cost_of_revenue exist
	if !s.HasLine(LineGrossProfit) && s.HasLine(LineRevenue) && s.HasLine(LineCostOfRevenue) {
		gp := s.GetLine(LineRevenue).Sub(s.GetLine(LineCostOfRevenue))
		s.SetLine(TypeIncomeStatement, LineGrossProfit, "Gross Profit", gp, 30)
	}

	// 3. Operating income derivation: if operating_income missing, gross_profit - operating_expenses
	if !s.HasLine(LineOperatingIncome) && s.HasLine(LineGrossProfit) && s.HasLine(LineOperatingExpenses) {
		op := s.GetLine(LineGrossProfit).Sub(s.GetLine(LineOperatingExpenses))
		s.SetLine(TypeIncomeStatement, LineOperatingIncome, "Operating Income", op, 50)
	}

	// 4. Free Cash Flow derivation: Operating Cash Flow - CapEx
	if !s.HasLine(LineFreeCashFlow) && s.HasLine(LineOperatingCashFlow) && s.HasLine(LineCapEx) {
		fcf := s.GetLine(LineOperatingCashFlow).Sub(s.GetLine(LineCapEx))
		s.SetLine(TypeCashFlow, LineFreeCashFlow, "Free Cash Flow", fcf, 50)
	}

	// 5. Book Value per Share derivation: Equity / Shares
	if !s.HasLine(LineBookValuePerShare) && s.HasLine(LineShareholdersEquity) && s.HasLine(LineTotalSharesOutstanding) {
		shares := s.GetLine(LineTotalSharesOutstanding)
		if shares.IsPositive() {
			bvps, err := s.GetLine(LineShareholdersEquity).Div(shares)
			if err == nil {
				s.SetLine(TypeBalanceSheet, LineBookValuePerShare, "Book Value Per Share", bvps, 140)
			}
		}
	}

	// 6. EPS derivation: Net Income / Shares
	if !s.HasLine(LineEPSBasic) && s.HasLine(LineNetIncome) && s.HasLine(LineTotalSharesOutstanding) {
		shares := s.GetLine(LineTotalSharesOutstanding)
		if shares.IsPositive() {
			eps, err := s.GetLine(LineNetIncome).Div(shares)
			if err == nil {
				s.SetLine(TypeIncomeStatement, LineEPSBasic, "Basic EPS", eps, 90)
			}
		}
	}

	return nil
}

// SynthesizeTTM creates a Trailing Twelve Months (TTM) statement from 4 consecutive quarters.
// quarters must be ordered chronologically (oldest to newest, with quarters[3] being the latest quarter).
func SynthesizeTTM(quarters []*Statement) (*Statement, error) {
	if len(quarters) != 4 {
		return nil, errors.New("exactly 4 consecutive quarters required to synthesize TTM")
	}

	latest := quarters[3]
	ttmPeriod := FinancialPeriod{
		ID:          fmt.Sprintf("%s-TTM-%s", latest.Period.Symbol, latest.Period.PeriodEnd),
		CompanyID:   latest.Period.CompanyID,
		Symbol:      latest.Period.Symbol,
		FiscalYear:  latest.Period.FiscalYear,
		PeriodType:  PeriodTTM,
		PeriodStart: quarters[0].Period.PeriodStart,
		PeriodEnd:   latest.Period.PeriodEnd,
		FilingDate:  latest.Period.FilingDate,
		AuditStatus: StatusUnaudited,
		Currency:    latest.Period.Currency,
		IsTTM:       true,
	}

	ttm := NewStatement(ttmPeriod)

	// Flow metrics: Sum across all 4 quarters
	flowKeys := []struct {
		st   StatementType
		code string
		name string
		ord  int
	}{
		{TypeIncomeStatement, LineRevenue, "Revenue (TTM)", 10},
		{TypeIncomeStatement, LineCostOfRevenue, "Cost of Revenue (TTM)", 20},
		{TypeIncomeStatement, LineGrossProfit, "Gross Profit (TTM)", 30},
		{TypeIncomeStatement, LineOperatingExpenses, "Operating Expenses (TTM)", 40},
		{TypeIncomeStatement, LineOperatingIncome, "Operating Income (TTM)", 50},
		{TypeIncomeStatement, LineInterestExpense, "Interest Expense (TTM)", 60},
		{TypeIncomeStatement, LineTaxExpense, "Tax Expense (TTM)", 70},
		{TypeIncomeStatement, LineNetIncome, "Net Income (TTM)", 80},
		{TypeCashFlow, LineOperatingCashFlow, "Operating Cash Flow (TTM)", 10},
		{TypeCashFlow, LineInvestingCashFlow, "Investing Cash Flow (TTM)", 20},
		{TypeCashFlow, LineFinancingCashFlow, "Financing Cash Flow (TTM)", 30},
		{TypeCashFlow, LineCapEx, "Capital Expenditures (TTM)", 40},
		{TypeCashFlow, LineFreeCashFlow, "Free Cash Flow (TTM)", 50},
	}

	for _, k := range flowKeys {
		sum := marketdata.Zero()
		hasAny := false
		for _, q := range quarters {
			if q.HasLine(k.code) {
				sum = sum.Add(q.GetLine(k.code))
				hasAny = true
			}
		}
		if hasAny {
			ttm.SetLine(k.st, k.code, k.name, sum, k.ord)
		}
	}

	// Stock metrics: Take the latest quarter's balance sheet snapshot
	for code, item := range latest.Lines {
		if item.StatementType == TypeBalanceSheet {
			ttm.SetLine(TypeBalanceSheet, code, item.Name, item.Amount, item.Order)
		}
	}

	// Re-derive per share values for TTM
	shares := latest.GetLine(LineTotalSharesOutstanding)
	if shares.IsPositive() {
		ttmNetIncome := ttm.GetLine(LineNetIncome)
		ttmEPS, err := ttmNetIncome.Div(shares)
		if err == nil {
			ttm.SetLine(TypeIncomeStatement, LineEPSBasic, "Basic EPS (TTM)", ttmEPS, 90)
		}
	}

	_ = ttm.Validate()
	return ttm, nil
}

// ByPeriodEnd sorts statements by period end date ascending.
type ByPeriodEnd []*Statement

func (a ByPeriodEnd) Len() int           { return len(a) }
func (a ByPeriodEnd) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByPeriodEnd) Less(i, j int) bool { return a[i].Period.PeriodEnd < a[j].Period.PeriodEnd }

// SortChronological sorts a slice of statements from oldest to newest.
func SortChronological(stmts []*Statement) {
	sort.Sort(ByPeriodEnd(stmts))
}
