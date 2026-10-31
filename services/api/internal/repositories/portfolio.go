package repositories

import (
	"context"
	"fmt"
	"time"

	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/portfolios"
	"nepse/terminal/api/internal/services"
)

// CreatePortfolio creates a new portfolio owned by userID.
func (s *Store) CreatePortfolio(ctx context.Context, userID, name, desc, method, benchmark string) (*portfolios.PortfolioState, error) {
	if method == "" {
		method = string(portfolios.MethodWAC)
	}
	if benchmark == "" {
		benchmark = "NEPSE"
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO portfolios (user_id, name, description, cost_basis_method, benchmark_symbol)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, name, currency, cost_basis_method, cash_balance`,
		userID, name, desc, method, benchmark,
	)

	var pf portfolios.PortfolioState
	var m string
	var cashStr string
	if err := row.Scan(&pf.ID, &pf.UserID, &pf.Name, &pf.Currency, &m, &cashStr); err != nil {
		return nil, fmt.Errorf("create portfolio: %w", err)
	}
	pf.CostBasisMethod = portfolios.CostBasisMethod(m)
	pf.CashBalance, _ = marketdata.Parse(cashStr)
	pf.Positions = make(map[string]*portfolios.Position)
	pf.Lots = make(map[string][]*portfolios.Lot)
	return &pf, nil
}

// ListPortfolios returns all portfolios owned by userID.
func (s *Store) ListPortfolios(ctx context.Context, userID string) ([]portfolios.PortfolioState, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, currency, cost_basis_method, cash_balance
		FROM portfolios
		WHERE user_id = $1
		ORDER BY name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []portfolios.PortfolioState
	for rows.Next() {
		var pf portfolios.PortfolioState
		var m, cashStr string
		if err := rows.Scan(&pf.ID, &pf.UserID, &pf.Name, &pf.Currency, &m, &cashStr); err != nil {
			return nil, err
		}
		pf.CostBasisMethod = portfolios.CostBasisMethod(m)
		pf.CashBalance, _ = marketdata.Parse(cashStr)
		result = append(result, pf)
	}
	return result, rows.Err()
}

// GetPortfolio retrieves a full portfolio state with positions and lots for a user.
func (s *Store) GetPortfolio(ctx context.Context, portfolioID, userID string) (*portfolios.PortfolioState, error) {
	var pf portfolios.PortfolioState
	var m, cashStr string
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, currency, cost_basis_method, cash_balance
		FROM portfolios
		WHERE id = $1 AND user_id = $2`, portfolioID, userID).Scan(
		&pf.ID, &pf.UserID, &pf.Name, &pf.Currency, &m, &cashStr,
	)
	if err != nil {
		return nil, err
	}
	pf.CostBasisMethod = portfolios.CostBasisMethod(m)
	pf.CashBalance, _ = marketdata.Parse(cashStr)
	pf.Positions = make(map[string]*portfolios.Position)
	pf.Lots = make(map[string][]*portfolios.Lot)

	// Load positions
	posRows, err := s.pool.Query(ctx, `
		SELECT symbol, quantity, cost_basis, avg_cost_per_share, market_price, market_value,
		       unrealized_pnl, unrealized_pnl_pct, realized_pnl, dividends_received, last_updated
		FROM portfolio_positions
		WHERE portfolio_id = $1`, portfolioID)
	if err != nil {
		return nil, err
	}
	defer posRows.Close()

	for posRows.Next() {
		var pos portfolios.Position
		var q, cb, avg, mp, mv, up, upp, rp, dr string
		var lu time.Time
		if err := posRows.Scan(
			&pos.Symbol, &q, &cb, &avg, &mp, &mv, &up, &upp, &rp, &dr, &lu,
		); err != nil {
			return nil, err
		}
		pos.Quantity, _ = marketdata.Parse(q)
		pos.CostBasis, _ = marketdata.Parse(cb)
		pos.AvgCostPerShare, _ = marketdata.Parse(avg)
		pos.MarketPrice, _ = marketdata.Parse(mp)
		pos.MarketValue, _ = marketdata.Parse(mv)
		pos.UnrealizedPnL, _ = marketdata.Parse(up)
		pos.UnrealizedPnLPct, _ = marketdata.Parse(upp)
		pos.RealizedPnL, _ = marketdata.Parse(rp)
		pos.DividendsReceived, _ = marketdata.Parse(dr)
		pos.LastUpdated = lu.Format("2006-01-02")
		pf.Positions[pos.Symbol] = &pos
	}

	// Load lots
	lotRows, err := s.pool.Query(ctx, `
		SELECT id, symbol, transaction_id, purchase_date, original_quantity, remaining_quantity, cost_per_share, total_cost
		FROM portfolio_lots
		WHERE portfolio_id = $1
		ORDER BY purchase_date ASC`, portfolioID)
	if err != nil {
		return nil, err
	}
	defer lotRows.Close()

	for lotRows.Next() {
		var lot portfolios.Lot
		var pDate time.Time
		var oq, rq, cps, tc string
		if err := lotRows.Scan(
			&lot.ID, &lot.Symbol, &lot.TransactionID, &pDate, &oq, &rq, &cps, &tc,
		); err != nil {
			return nil, err
		}
		lot.PurchaseDate = pDate.Format("2006-01-02")
		lot.OriginalQuantity, _ = marketdata.Parse(oq)
		lot.RemainingQuantity, _ = marketdata.Parse(rq)
		lot.CostPerShare, _ = marketdata.Parse(cps)
		lot.TotalCost, _ = marketdata.Parse(tc)
		pf.Lots[lot.Symbol] = append(pf.Lots[lot.Symbol], &lot)
	}

	return &pf, nil
}

// DeletePortfolio deletes a portfolio owned by userID.
func (s *Store) DeletePortfolio(ctx context.Context, portfolioID, userID string) error {
	cmd, err := s.pool.Exec(ctx, `
		DELETE FROM portfolios
		WHERE id = $1 AND user_id = $2`, portfolioID, userID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("portfolio not found")
	}
	return nil
}

// SavePortfolioState updates positions, cash balance, and lots transactionally.
func (s *Store) SavePortfolioState(ctx context.Context, pf *portfolios.PortfolioState) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Update cash balance
	_, err = tx.Exec(ctx, `
		UPDATE portfolios
		SET cash_balance = $1, updated_at = now()
		WHERE id = $2`, pf.CashBalance.String(), pf.ID)
	if err != nil {
		return err
	}

	// Upsert positions
	for _, pos := range pf.Positions {
		_, err = tx.Exec(ctx, `
			INSERT INTO portfolio_positions (
				portfolio_id, symbol, quantity, cost_basis, avg_cost_per_share,
				market_price, market_value, unrealized_pnl, unrealized_pnl_pct,
				realized_pnl, dividends_received, last_updated
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now()
			) ON CONFLICT (portfolio_id, symbol) DO UPDATE SET
				quantity = EXCLUDED.quantity,
				cost_basis = EXCLUDED.cost_basis,
				avg_cost_per_share = EXCLUDED.avg_cost_per_share,
				market_price = EXCLUDED.market_price,
				market_value = EXCLUDED.market_value,
				unrealized_pnl = EXCLUDED.unrealized_pnl,
				unrealized_pnl_pct = EXCLUDED.unrealized_pnl_pct,
				realized_pnl = EXCLUDED.realized_pnl,
				dividends_received = EXCLUDED.dividends_received,
				last_updated = now()`,
			pf.ID, pos.Symbol, pos.Quantity.String(), pos.CostBasis.String(),
			pos.AvgCostPerShare.String(), pos.MarketPrice.String(), pos.MarketValue.String(),
			pos.UnrealizedPnL.String(), pos.UnrealizedPnLPct.String(), pos.RealizedPnL.String(),
			pos.DividendsReceived.String(),
		)
		if err != nil {
			return fmt.Errorf("upsert position %s: %w", pos.Symbol, err)
		}
	}

	return tx.Commit(ctx)
}

// RecordPortfolioTransaction inserts an executed transaction.
func (s *Store) RecordPortfolioTransaction(ctx context.Context, txn *portfolios.Transaction) (string, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO portfolio_transactions (
			portfolio_id, symbol, txn_type, traded_on, settlement_date,
			quantity, price, gross_amount, broker_commission, sebon_fee,
			dp_fee, cgt, total_fees, net_amount, cost_basis, realized_pnl, notes
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		) RETURNING id`,
		txn.PortfolioID, txn.Symbol, string(txn.TxnType), txn.TradedOn,
		nullableDate(txn.SettlementDate), txn.Quantity.String(), txn.Price.String(),
		txn.GrossAmount.String(), txn.BrokerCommission.String(), txn.SEBONFee.String(),
		txn.DPFee.String(), txn.CGT.String(), txn.TotalFees.String(), txn.NetAmount.String(),
		txn.CostBasis.String(), txn.RealizedPnL.String(), txn.Notes,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// GetPortfolioTransactions returns transactions for a portfolio.
func (s *Store) GetPortfolioTransactions(ctx context.Context, portfolioID string, limit, offset int) ([]portfolios.Transaction, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, portfolio_id, symbol, txn_type, traded_on, settlement_date,
		       quantity, price, gross_amount, broker_commission, sebon_fee,
		       dp_fee, cgt, total_fees, net_amount, cost_basis, realized_pnl, notes
		FROM portfolio_transactions
		WHERE portfolio_id = $1
		ORDER BY traded_on DESC, created_at DESC
		LIMIT $2 OFFSET $3`, portfolioID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []portfolios.Transaction
	for rows.Next() {
		var t portfolios.Transaction
		var tType string
		var tDate time.Time
		var sDate *time.Time
		var q, p, g, bc, sf, dp, cgt, tf, na, cb, rp string

		if err := rows.Scan(
			&t.ID, &t.PortfolioID, &t.Symbol, &tType, &tDate, &sDate,
			&q, &p, &g, &bc, &sf, &dp, &cgt, &tf, &na, &cb, &rp, &t.Notes,
		); err != nil {
			return nil, err
		}
		t.TxnType = portfolios.TransactionType(tType)
		t.TradedOn = tDate.Format("2006-01-02")
		if sDate != nil {
			t.SettlementDate = sDate.Format("2006-01-02")
		}
		t.Quantity, _ = marketdata.Parse(q)
		t.Price, _ = marketdata.Parse(p)
		t.GrossAmount, _ = marketdata.Parse(g)
		t.BrokerCommission, _ = marketdata.Parse(bc)
		t.SEBONFee, _ = marketdata.Parse(sf)
		t.DPFee, _ = marketdata.Parse(dp)
		t.CGT, _ = marketdata.Parse(cgt)
		t.TotalFees, _ = marketdata.Parse(tf)
		t.NetAmount, _ = marketdata.Parse(na)
		t.CostBasis, _ = marketdata.Parse(cb)
		t.RealizedPnL, _ = marketdata.Parse(rp)
		result = append(result, t)
	}
	return result, rows.Err()
}

// SaveDailySnapshot records a daily valuation snapshot for a portfolio.
func (s *Store) SaveDailySnapshot(ctx context.Context, portfolioID string, obs services.DailyObservation, costBasis marketdata.Decimal, dailyReturn, twr float64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO portfolio_daily_snapshots (
			portfolio_id, snapshot_date, portfolio_value, cash_balance, total_cost_basis,
			net_cash_flow, benchmark_value, daily_return, twr
		) VALUES (
			$1, $2, $3, 0, $4, $5, $6, $7, $8
		) ON CONFLICT (portfolio_id, snapshot_date) DO UPDATE SET
			portfolio_value = EXCLUDED.portfolio_value,
			total_cost_basis = EXCLUDED.total_cost_basis,
			net_cash_flow = EXCLUDED.net_cash_flow,
			benchmark_value = EXCLUDED.benchmark_value,
			daily_return = EXCLUDED.daily_return,
			twr = EXCLUDED.twr`,
		portfolioID, obs.Date, obs.PortfolioValue.String(), costBasis.String(),
		obs.NetCashFlow.String(), obs.BenchmarkValue.String(), dailyReturn, twr,
	)
	return err
}

// GetDailyObservations loads daily snapshots for analytics calculation.
func (s *Store) GetDailyObservations(ctx context.Context, portfolioID, fromDate, toDate string) ([]services.DailyObservation, error) {
	query := `
		SELECT snapshot_date, portfolio_value, net_cash_flow, COALESCE(benchmark_value, 0)
		FROM portfolio_daily_snapshots
		WHERE portfolio_id = $1`
	args := []any{portfolioID}
	argIdx := 2

	if fromDate != "" {
		query += fmt.Sprintf(" AND snapshot_date >= $%d", argIdx)
		args = append(args, fromDate)
		argIdx++
	}
	if toDate != "" {
		query += fmt.Sprintf(" AND snapshot_date <= $%d", argIdx)
		args = append(args, toDate)
		argIdx++
	}
	query += " ORDER BY snapshot_date ASC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []services.DailyObservation
	for rows.Next() {
		var sDate time.Time
		var pv, ncf, bv string
		if err := rows.Scan(&sDate, &pv, &ncf, &bv); err != nil {
			return nil, err
		}
		pVal, _ := marketdata.Parse(pv)
		nCash, _ := marketdata.Parse(ncf)
		bVal, _ := marketdata.Parse(bv)

		result = append(result, services.DailyObservation{
			Date:           sDate.Format("2006-01-02"),
			PortfolioValue: pVal,
			NetCashFlow:    nCash,
			BenchmarkValue: bVal,
		})
	}
	return result, rows.Err()
}
