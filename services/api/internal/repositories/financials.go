package repositories

import (
	"context"
	"fmt"
	"time"

	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/services"
)

// UpsertFinancialPeriod stores or updates a financial reporting period.
func (s *Store) UpsertFinancialPeriod(ctx context.Context, p services.FinancialPeriod) (string, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO financial_periods (
			symbol, fiscal_year, period_type, period_start, period_end, filing_date, audit_status, currency, is_ttm
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (symbol, fiscal_year, period_type, audit_status) DO UPDATE SET
			period_start = EXCLUDED.period_start,
			period_end = EXCLUDED.period_end,
			filing_date = EXCLUDED.filing_date,
			currency = EXCLUDED.currency,
			is_ttm = EXCLUDED.is_ttm
		RETURNING id`,
		p.Symbol, p.FiscalYear, string(p.PeriodType), p.PeriodStart, p.PeriodEnd, p.FilingDate, string(p.AuditStatus), p.Currency, p.IsTTM,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", fmt.Errorf("upsert financial period: %w", err)
	}
	return id, nil
}

// UpsertStatementLines inserts or updates normalized statement lines.
func (s *Store) UpsertStatementLines(ctx context.Context, periodID string, lines []services.LineItem) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, line := range lines {
		_, err := tx.Exec(ctx, `
			INSERT INTO financial_statement_lines (
				period_id, statement_type, line_code, line_name, amount, line_order, unit
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (period_id, line_code) DO UPDATE SET
				line_name = EXCLUDED.line_name,
				amount = EXCLUDED.amount,
				line_order = EXCLUDED.line_order,
				unit = EXCLUDED.unit`,
			periodID, string(line.StatementType), line.Code, line.Name, line.Amount.String(), line.Order, line.Unit,
		)
		if err != nil {
			return fmt.Errorf("upsert line %s: %w", line.Code, err)
		}
	}
	return tx.Commit(ctx)
}

// GetFinancialPeriods returns all reporting periods for a given security symbol.
func (s *Store) GetFinancialPeriods(ctx context.Context, symbol string) ([]services.FinancialPeriod, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, company_id, symbol, fiscal_year, period_type, period_start, period_end, filing_date, audit_status, currency, is_ttm
		FROM financial_periods
		WHERE symbol = $1
		ORDER BY period_end DESC`, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []services.FinancialPeriod
	for rows.Next() {
		var p services.FinancialPeriod
		var compID *string
		var pStart, pEnd, fDate time.Time
		if err := rows.Scan(
			&p.ID, &compID, &p.Symbol, &p.FiscalYear, &p.PeriodType,
			&pStart, &pEnd, &fDate, &p.AuditStatus, &p.Currency, &p.IsTTM,
		); err != nil {
			return nil, err
		}
		if compID != nil {
			p.CompanyID = *compID
		}
		p.PeriodStart = pStart.Format("2006-01-02")
		p.PeriodEnd = pEnd.Format("2006-01-02")
		p.FilingDate = fDate.Format("2006-01-02")
		result = append(result, p)
	}
	return result, rows.Err()
}

// GetStatement retrieves a full statement with all normalized lines.
func (s *Store) GetStatement(ctx context.Context, periodID string) (*services.Statement, error) {
	var p services.FinancialPeriod
	var compID *string
	var pStart, pEnd, fDate time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, company_id, symbol, fiscal_year, period_type, period_start, period_end, filing_date, audit_status, currency, is_ttm
		FROM financial_periods
		WHERE id = $1`, periodID).Scan(
		&p.ID, &compID, &p.Symbol, &p.FiscalYear, &p.PeriodType,
		&pStart, &pEnd, &fDate, &p.AuditStatus, &p.Currency, &p.IsTTM,
	)
	if err != nil {
		return nil, err
	}
	if compID != nil {
		p.CompanyID = *compID
	}
	p.PeriodStart = pStart.Format("2006-01-02")
	p.PeriodEnd = pEnd.Format("2006-01-02")
	p.FilingDate = fDate.Format("2006-01-02")

	stmt := services.NewStatement(p)

	rows, err := s.pool.Query(ctx, `
		SELECT statement_type, line_code, line_name, amount, line_order, unit
		FROM financial_statement_lines
		WHERE period_id = $1
		ORDER BY line_order ASC`, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var stType, code, name, unit string
		var amtStr string
		var order int
		if err := rows.Scan(&stType, &code, &name, &amtStr, &order, &unit); err != nil {
			return nil, err
		}
		amt, _ := marketdata.Parse(amtStr)
		stmt.SetLine(services.StatementType(stType), code, name, amt, order)
	}
	_ = stmt.Validate()
	return stmt, rows.Err()
}

// UpsertFinancialRatios records computed reporting-period-aware ratios.
func (s *Store) UpsertFinancialRatios(ctx context.Context, r *services.PeriodRatios, periodID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO financial_ratios (
			period_id, symbol, as_of_date, price_used, market_cap, eps, annualized_eps,
			book_value_per_share, pe_ratio, pb_ratio, ps_ratio, ev_to_ebitda, roe, roa,
			gross_margin, operating_margin, net_margin, current_ratio, quick_ratio,
			debt_to_equity, interest_coverage, asset_turnover, formula_details
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
		) ON CONFLICT (period_id, as_of_date) DO UPDATE SET
			price_used = EXCLUDED.price_used,
			market_cap = EXCLUDED.market_cap,
			eps = EXCLUDED.eps,
			annualized_eps = EXCLUDED.annualized_eps,
			book_value_per_share = EXCLUDED.book_value_per_share,
			pe_ratio = EXCLUDED.pe_ratio,
			pb_ratio = EXCLUDED.pb_ratio,
			ps_ratio = EXCLUDED.ps_ratio,
			ev_to_ebitda = EXCLUDED.ev_to_ebitda,
			roe = EXCLUDED.roe,
			roa = EXCLUDED.roa,
			gross_margin = EXCLUDED.gross_margin,
			operating_margin = EXCLUDED.operating_margin,
			net_margin = EXCLUDED.net_margin,
			current_ratio = EXCLUDED.current_ratio,
			quick_ratio = EXCLUDED.quick_ratio,
			debt_to_equity = EXCLUDED.debt_to_equity,
			interest_coverage = EXCLUDED.interest_coverage,
			asset_turnover = EXCLUDED.asset_turnover,
			formula_details = EXCLUDED.formula_details`,
		periodID, r.Symbol, r.AsOfDate, r.PriceUsed.String(), r.MarketCap.String(),
		r.EPS.String(), r.AnnualizedEPS.String(), r.BookValuePerShare.String(),
		r.PE.String(), r.PB.String(), r.PS.String(), r.EVToEBITDA.String(),
		r.ROE.String(), r.ROA.String(), r.GrossMargin.String(), r.OperatingMargin.String(),
		r.NetMargin.String(), r.CurrentRatio.String(), r.QuickRatio.String(),
		r.DebtToEquity.String(), r.InterestCoverage.String(), r.AssetTurnover.String(),
		r.FormulaNotes,
	)
	return err
}
