// Package repositories - mutual fund data access (Batch 4.7).
package repositories

import (
	"context"
	"database/sql"
)

type MutualFundDetail struct {
	ID       string `json:"id"`
	FundCode string `json:"fund_code"`
	FundName string `json:"fund_name"`
	Manager  string `json:"manager"`
	FundType string `json:"fund_type"`
	Status   string `json:"status"`
}

func (s *Store) GetMutualFund(ctx context.Context, id string) (*MutualFundDetail, error) {
	q := `SELECT id::text, fund_code, fund_name, manager, fund_type, status FROM mutual_funds WHERE id = $1::uuid`
	var m MutualFundDetail
	err := s.pool.QueryRow(ctx, q, id).Scan(&m.ID, &m.FundCode, &m.FundName, &m.Manager, &m.FundType, &m.Status)
	if err != nil {
		return nil, wrapNoRows(err)
	}
	return &m, nil
}

type MutualFundHoldingRow struct {
	HoldingSymbol string  `json:"holding_symbol"`
	HoldingName   string  `json:"holding_name"`
	Units         *string `json:"units"`
	MarketValue   *string `json:"market_value"`
	WeightPercent *string `json:"weight_percent"`
	AsOfDate      string  `json:"as_of_date"`
}

func (s *Store) ListMutualFundHoldings(ctx context.Context, fundID string) ([]MutualFundHoldingRow, error) {
	q := `SELECT holding_symbol, holding_name, units::text, market_value::text, weight_percent::text, as_of_date::text FROM mutual_fund_portfolio_disclosures WHERE mutual_fund_id = $1::uuid ORDER BY as_of_date DESC, market_value DESC NULLS LAST`
	rows, err := s.pool.Query(ctx, q, fundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MutualFundHoldingRow, 0)
	for rows.Next() {
		var r MutualFundHoldingRow
		var u, mv, wp sql.NullString
		var ad string
		if err := rows.Scan(&r.HoldingSymbol, &r.HoldingName, &u, &mv, &wp, &ad); err != nil {
			return nil, err
		}
		if u.Valid { v := u.String; r.Units = &v }
		if mv.Valid { v := mv.String; r.MarketValue = &v }
		if wp.Valid { v := wp.String; r.WeightPercent = &v }
		r.AsOfDate = ad
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) MutualFundCoverage(ctx context.Context) (map[string]int, error) {
	tables := []string{"mutual_funds", "mutual_fund_schemes", "mutual_fund_nav_history", "mutual_fund_portfolio_disclosures"}
	out := map[string]int{}
	for _, t := range tables {
		var n int
		if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM "+t).Scan(&n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, nil
}
