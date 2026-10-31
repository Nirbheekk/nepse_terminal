package repositories

import (
	"context"
	"fmt"
	"time"

	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/services"
)

// UpsertCorporateAction records or updates a corporate action.
func (s *Store) UpsertCorporateAction(ctx context.Context, act services.CorporateAction) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO corporate_actions (
			symbol, action_type, fiscal_year, announcement_date, book_closure_date,
			agm_date, effective_date, cash_percent, cash_amount, bonus_percent,
			rights_ratio, rights_price, split_from, split_to, status, notes
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		) ON CONFLICT (symbol, action_type, fiscal_year, book_closure_date) DO UPDATE SET
			announcement_date = EXCLUDED.announcement_date,
			agm_date = EXCLUDED.agm_date,
			effective_date = EXCLUDED.effective_date,
			cash_percent = EXCLUDED.cash_percent,
			cash_amount = EXCLUDED.cash_amount,
			bonus_percent = EXCLUDED.bonus_percent,
			rights_ratio = EXCLUDED.rights_ratio,
			rights_price = EXCLUDED.rights_price,
			split_from = EXCLUDED.split_from,
			split_to = EXCLUDED.split_to,
			status = EXCLUDED.status,
			notes = EXCLUDED.notes`,
		act.Symbol, string(act.ActionType), act.FiscalYear,
		nullableDate(act.AnnouncementDate), act.BookClosureDate,
		nullableDate(act.AGMDate), nullableDate(act.EffectiveDate),
		act.CashPercent.String(), act.CashAmount.String(), act.BonusPercent.String(),
		act.RightsRatio.String(), act.RightsPrice.String(), act.SplitFrom.String(),
		act.SplitTo.String(), string(act.Status), act.Notes,
	)
	return err
}

func nullableDate(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// GetCorporateActions retrieves corporate actions matching optional filters.
func (s *Store) GetCorporateActions(ctx context.Context, symbol, actionType string) ([]services.CorporateAction, error) {
	query := `
		SELECT id, symbol, action_type, fiscal_year, announcement_date, book_closure_date,
		       agm_date, effective_date, cash_percent, cash_amount, bonus_percent,
		       rights_ratio, rights_price, split_from, split_to, status, notes
		FROM corporate_actions
		WHERE 1=1`
	args := []any{}
	argIdx := 1

	if symbol != "" {
		query += fmt.Sprintf(" AND symbol = $%d", argIdx)
		args = append(args, symbol)
		argIdx++
	}
	if actionType != "" {
		query += fmt.Sprintf(" AND action_type = $%d", argIdx)
		args = append(args, actionType)
		argIdx++
	}
	query += " ORDER BY book_closure_date DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []services.CorporateAction
	for rows.Next() {
		var a services.CorporateAction
		var annDate, agmDate, effDate *time.Time
		var bkDate time.Time
		var actType, status string
		var cashPct, cashAmt, bonusPct, rRatio, rPrice, sFrom, sTo string

		if err := rows.Scan(
			&a.ID, &a.Symbol, &actType, &a.FiscalYear, &annDate, &bkDate,
			&agmDate, &effDate, &cashPct, &cashAmt, &bonusPct,
			&rRatio, &rPrice, &sFrom, &sTo, &status, &a.Notes,
		); err != nil {
			return nil, err
		}

		a.ActionType = services.ActionType(actType)
		a.Status = services.ActionStatus(status)
		a.BookClosureDate = bkDate.Format("2006-01-02")
		if annDate != nil {
			a.AnnouncementDate = annDate.Format("2006-01-02")
		}
		if agmDate != nil {
			a.AGMDate = agmDate.Format("2006-01-02")
		}
		if effDate != nil {
			a.EffectiveDate = effDate.Format("2006-01-02")
		}

		a.CashPercent, _ = marketdata.Parse(cashPct)
		a.CashAmount, _ = marketdata.Parse(cashAmt)
		a.BonusPercent, _ = marketdata.Parse(bonusPct)
		a.RightsRatio, _ = marketdata.Parse(rRatio)
		a.RightsPrice, _ = marketdata.Parse(rPrice)
		a.SplitFrom, _ = marketdata.Parse(sFrom)
		a.SplitTo, _ = marketdata.Parse(sTo)

		result = append(result, a)
	}
	return result, rows.Err()
}

// GetDailyOHLCV retrieves raw daily OHLCV bars for a security ordered chronologically ascending.
func (s *Store) GetDailyOHLCV(ctx context.Context, symbol, fromDate, toDate string) ([]services.OHLCVBar, error) {
	query := `
		SELECT trading_date, open, high, low, close, volume
		FROM daily_ohlcv
		WHERE symbol = $1`
	args := []any{symbol}
	argIdx := 2

	if fromDate != "" {
		query += fmt.Sprintf(" AND trading_date >= $%d", argIdx)
		args = append(args, fromDate)
		argIdx++
	}
	if toDate != "" {
		query += fmt.Sprintf(" AND trading_date <= $%d", argIdx)
		args = append(args, toDate)
		argIdx++
	}
	query += " ORDER BY trading_date ASC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bars []services.OHLCVBar
	for rows.Next() {
		var tDate time.Time
		var o, h, l, c, v string
		if err := rows.Scan(&tDate, &o, &h, &l, &c, &v); err != nil {
			return nil, err
		}
		open, _ := marketdata.Parse(o)
		high, _ := marketdata.Parse(h)
		low, _ := marketdata.Parse(l)
		closePrice, _ := marketdata.Parse(c)
		vol, _ := marketdata.Parse(v)

		bars = append(bars, services.OHLCVBar{
			Date:   tDate.Format("2006-01-02"),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePrice,
			Volume: vol,
		})
	}
	return bars, rows.Err()
}
