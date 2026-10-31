package portfolios

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"nepse/terminal/api/internal/marketdata"
)

type CostBasisMethod string

const (
	MethodFIFO CostBasisMethod = "FIFO"
	MethodWAC  CostBasisMethod = "WAC" // Weighted Average Cost (Nepal NEPSE standard WACC)
)

type TransactionType string

const (
	TxnBuy             TransactionType = "BUY"
	TxnSell            TransactionType = "SELL"
	TxnDividendCash    TransactionType = "DIVIDEND_CASH"
	TxnDividendBonus   TransactionType = "DIVIDEND_BONUS"
	TxnRightsExercise  TransactionType = "RIGHTS_EXERCISE"
	TxnDeposit         TransactionType = "DEPOSIT"
	TxnWithdrawal      TransactionType = "WITHDRAWAL"
	TxnFee             TransactionType = "FEE"
	TxnTax             TransactionType = "TAX"
	TxnSplitAdjustment TransactionType = "SPLIT_ADJUSTMENT"
	TxnTransferIn      TransactionType = "TRANSFER_IN"
	TxnTransferOut     TransactionType = "TRANSFER_OUT"
)

// Transaction represents an executed event recorded in the portfolio ledger.
type Transaction struct {
	ID               string             `json:"id"`
	PortfolioID      string             `json:"portfolio_id"`
	Symbol           string             `json:"symbol"`
	TxnType          TransactionType    `json:"txn_type"`
	TradedOn         string             `json:"traded_on"` // YYYY-MM-DD
	SettlementDate   string             `json:"settlement_date,omitempty"`
	Quantity         marketdata.Decimal `json:"quantity"`
	Price            marketdata.Decimal `json:"price"`
	GrossAmount      marketdata.Decimal `json:"gross_amount"`
	BrokerCommission marketdata.Decimal `json:"broker_commission"`
	SEBONFee         marketdata.Decimal `json:"sebon_fee"`
	DPFee            marketdata.Decimal `json:"dp_fee"`
	CGT              marketdata.Decimal `json:"cgt"`
	TotalFees        marketdata.Decimal `json:"total_fees"`
	NetAmount        marketdata.Decimal `json:"net_amount"`
	CostBasis        marketdata.Decimal `json:"cost_basis"`
	RealizedPnL      marketdata.Decimal `json:"realized_pnl"`
	Notes            string             `json:"notes,omitempty"`
}

// Lot represents an individual purchase lot for FIFO accounting.
type Lot struct {
	ID                string             `json:"id"`
	Symbol            string             `json:"symbol"`
	TransactionID     string             `json:"transaction_id"`
	PurchaseDate      string             `json:"purchase_date"`
	OriginalQuantity  marketdata.Decimal `json:"original_quantity"`
	RemainingQuantity marketdata.Decimal `json:"remaining_quantity"`
	CostPerShare      marketdata.Decimal `json:"cost_per_share"` // Total effective cost per share including purchase fees
	TotalCost         marketdata.Decimal `json:"total_cost"`
}

// Position represents an aggregate holding in a security.
type Position struct {
	Symbol            string             `json:"symbol"`
	Quantity          marketdata.Decimal `json:"quantity"`
	CostBasis         marketdata.Decimal `json:"cost_basis"`
	AvgCostPerShare   marketdata.Decimal `json:"avg_cost_per_share"` // WACC
	MarketPrice       marketdata.Decimal `json:"market_price"`
	MarketValue       marketdata.Decimal `json:"market_value"`
	UnrealizedPnL     marketdata.Decimal `json:"unrealized_pnl"`
	UnrealizedPnLPct  marketdata.Decimal `json:"unrealized_pnl_pct"`
	RealizedPnL       marketdata.Decimal `json:"realized_pnl"`
	DividendsReceived marketdata.Decimal `json:"dividends_received"`
	LastUpdated       string             `json:"last_updated"`
}

// PortfolioState holds the complete in-memory ledger state for a portfolio.
type PortfolioState struct {
	ID                  string               `json:"id"`
	UserID              string               `json:"user_id"`
	Name                string               `json:"name"`
	Currency            string               `json:"currency"`
	CostBasisMethod     CostBasisMethod      `json:"cost_basis_method"`
	CashBalance         marketdata.Decimal   `json:"cash_balance"`
	Positions           map[string]*Position `json:"positions"`
	Lots                map[string][]*Lot    `json:"lots"` // Symbol -> []Lot
	TotalRealizedPnL    marketdata.Decimal   `json:"total_realized_pnl"`
	TotalDividends      marketdata.Decimal   `json:"total_dividends"`
	TotalFeesPaid       marketdata.Decimal   `json:"total_fees_paid"`
	TotalCGTPaid        marketdata.Decimal   `json:"total_cgt_paid"`
	TotalMarketValue    marketdata.Decimal   `json:"total_market_value"`
	TotalCostBasis      marketdata.Decimal   `json:"total_cost_basis"`
	TotalPortfolioValue marketdata.Decimal   `json:"total_portfolio_value"` // Cash + MarketValue
	TotalUnrealizedPnL  marketdata.Decimal   `json:"total_unrealized_pnl"`
	TotalReturn         marketdata.Decimal   `json:"total_return"`
}

// NewPortfolioState creates an empty portfolio tracker.
func NewPortfolioState(id, userID, name string, method CostBasisMethod) *PortfolioState {
	if method == "" {
		method = MethodWAC // Nepal standard
	}
	return &PortfolioState{
		ID:              id,
		UserID:          userID,
		Name:            name,
		Currency:        "NPR",
		CostBasisMethod: method,
		CashBalance:     marketdata.Zero(),
		Positions:       make(map[string]*Position),
		Lots:            make(map[string][]*Lot),
	}
}

// ProcessTransaction processes a single transaction into the portfolio state and returns the enriched transaction.
func (p *PortfolioState) ProcessTransaction(txn Transaction) (*Transaction, error) {
	switch txn.TxnType {
	case TxnDeposit:
		if txn.NetAmount.IsZero() && txn.GrossAmount.IsPositive() {
			txn.NetAmount = txn.GrossAmount
		}
		p.CashBalance = p.CashBalance.Add(txn.NetAmount)
		return &txn, nil

	case TxnWithdrawal:
		if txn.NetAmount.IsZero() && txn.GrossAmount.IsPositive() {
			txn.NetAmount = txn.GrossAmount
		}
		if p.CashBalance.LessThan(txn.NetAmount) {
			return nil, fmt.Errorf("insufficient cash balance: available %s, requested %s", p.CashBalance.String(), txn.NetAmount.String())
		}
		p.CashBalance = p.CashBalance.Sub(txn.NetAmount)
		return &txn, nil

	case TxnDividendCash:
		if txn.NetAmount.IsZero() && txn.GrossAmount.IsPositive() {
			txn.NetAmount = txn.GrossAmount.Sub(txn.CGT).Sub(txn.TotalFees)
		}
		p.CashBalance = p.CashBalance.Add(txn.NetAmount)
		p.TotalDividends = p.TotalDividends.Add(txn.NetAmount)
		if pos, ok := p.Positions[txn.Symbol]; ok {
			pos.DividendsReceived = pos.DividendsReceived.Add(txn.NetAmount)
		}
		return &txn, nil

	case TxnBuy:
		return p.processBuy(txn)

	case TxnSell:
		return p.processSell(txn)

	case TxnDividendBonus:
		return p.processBonusShare(txn)

	case TxnRightsExercise:
		return p.processRightsExercise(txn)

	default:
		return nil, fmt.Errorf("unsupported transaction type: %s", txn.TxnType)
	}
}

func (p *PortfolioState) processBuy(txn Transaction) (*Transaction, error) {
	if txn.Quantity.IsZero() || txn.Quantity.IsNegative() {
		return nil, errors.New("buy quantity must be positive")
	}
	if txn.Price.IsZero() || txn.Price.IsNegative() {
		return nil, errors.New("buy price must be positive")
	}

	// Calculate default fees if not supplied
	gross := txn.Quantity.Mul(txn.Price)
	txn.GrossAmount = gross
	if txn.BrokerCommission.IsZero() {
		txn.BrokerCommission = CalculateBrokerCommission(gross)
	}
	if txn.SEBONFee.IsZero() {
		txn.SEBONFee = CalculateSEBONFee(gross)
	}
	if txn.DPFee.IsZero() {
		txn.DPFee = FlatDPFee()
	}
	txn.TotalFees = txn.BrokerCommission.Add(txn.SEBONFee).Add(txn.DPFee)
	txn.NetAmount = gross.Add(txn.TotalFees)
	p.TotalFeesPaid = p.TotalFeesPaid.Add(txn.TotalFees)

	// Deduct net purchase cost from cash ledger
	p.CashBalance = p.CashBalance.Sub(txn.NetAmount)

	pos, exists := p.Positions[txn.Symbol]
	if !exists {
		pos = &Position{
			Symbol:      txn.Symbol,
			LastUpdated: txn.TradedOn,
		}
		p.Positions[txn.Symbol] = pos
	}

	// Effective total cost of this purchase lot (purchase amount + fees)
	lotCost := txn.NetAmount
	costPerShare, _ := lotCost.Div(txn.Quantity)

	// Update position cost basis
	newQty := pos.Quantity.Add(txn.Quantity)
	newCost := pos.CostBasis.Add(lotCost)
	avgCost, _ := newCost.Div(newQty)

	pos.Quantity = newQty
	pos.CostBasis = newCost
	pos.AvgCostPerShare = avgCost
	pos.LastUpdated = txn.TradedOn

	// Record FIFO Lot
	lot := &Lot{
		ID:                txn.ID,
		Symbol:            txn.Symbol,
		TransactionID:     txn.ID,
		PurchaseDate:      txn.TradedOn,
		OriginalQuantity:  txn.Quantity,
		RemainingQuantity: txn.Quantity,
		CostPerShare:      costPerShare,
		TotalCost:         lotCost,
	}
	p.Lots[txn.Symbol] = append(p.Lots[txn.Symbol], lot)

	txn.CostBasis = lotCost
	return &txn, nil
}

func (p *PortfolioState) processSell(txn Transaction) (*Transaction, error) {
	pos, exists := p.Positions[txn.Symbol]
	if !exists || pos.Quantity.LessThan(txn.Quantity) {
		avail := marketdata.Zero()
		if exists {
			avail = pos.Quantity
		}
		return nil, fmt.Errorf("insufficient shares for %s: held %s, selling %s", txn.Symbol, avail.String(), txn.Quantity.String())
	}

	gross := txn.Quantity.Mul(txn.Price)
	txn.GrossAmount = gross
	if txn.BrokerCommission.IsZero() {
		txn.BrokerCommission = CalculateBrokerCommission(gross)
	}
	if txn.SEBONFee.IsZero() {
		txn.SEBONFee = CalculateSEBONFee(gross)
	}
	if txn.DPFee.IsZero() {
		txn.DPFee = FlatDPFee()
	}
	txn.TotalFees = txn.BrokerCommission.Add(txn.SEBONFee).Add(txn.DPFee)
	p.TotalFeesPaid = p.TotalFeesPaid.Add(txn.TotalFees)

	var costBasis marketdata.Decimal
	var totalCGT marketdata.Decimal

	if p.CostBasisMethod == MethodFIFO {
		// Deplete lots from oldest to newest
		needed := txn.Quantity
		sellDate, _ := time.Parse("2006-01-02", txn.TradedOn)

		lots := p.Lots[txn.Symbol]
		for _, lot := range lots {
			if lot.RemainingQuantity.IsZero() {
				continue
			}
			take := marketdata.Min(lot.RemainingQuantity, needed)
			lotBasis := take.Mul(lot.CostPerShare)
			costBasis = costBasis.Add(lotBasis)

			// Holding period for CGT
			buyDate, _ := time.Parse("2006-01-02", lot.PurchaseDate)
			isShortTerm := true
			if !buyDate.IsZero() && !sellDate.IsZero() {
				days := sellDate.Sub(buyDate).Hours() / 24
				if days > 365 {
					isShortTerm = false
				}
			}

			// Pro-rata sale proceeds and fees for this lot portion
			lotRatio, _ := take.Div(txn.Quantity)
			lotGross := gross.Mul(lotRatio)
			lotFees := txn.TotalFees.Mul(lotRatio)
			lotRealized := lotGross.Sub(lotFees).Sub(lotBasis)

			if lotRealized.IsPositive() {
				invType := InvestorIndividualShortTerm
				if !isShortTerm {
					invType = InvestorIndividualLongTerm
				}
				lotCGT := CalculateCGT(lotRealized, invType)
				totalCGT = totalCGT.Add(lotCGT)
			}

			lot.RemainingQuantity = lot.RemainingQuantity.Sub(take)
			needed = needed.Sub(take)
			if needed.IsZero() {
				break
			}
		}

		if !needed.IsZero() {
			return nil, fmt.Errorf("FIFO lot tracking error: deficit of %s shares", needed.String())
		}
	} else {
		// WAC (Weighted Average Cost)
		costBasis = pos.AvgCostPerShare.Mul(txn.Quantity)
		realizedBeforeTax := gross.Sub(txn.TotalFees).Sub(costBasis)
		if realizedBeforeTax.IsPositive() {
			totalCGT = CalculateCGT(realizedBeforeTax, InvestorIndividualShortTerm)
		}
	}

	txn.CostBasis = costBasis
	realizedPnL := gross.Sub(txn.TotalFees).Sub(costBasis)
	txn.RealizedPnL = realizedPnL
	txn.CGT = totalCGT

	netProceeds := gross.Sub(txn.TotalFees).Sub(totalCGT)
	txn.NetAmount = netProceeds

	// Update cash ledger
	p.CashBalance = p.CashBalance.Add(netProceeds)
	p.TotalRealizedPnL = p.TotalRealizedPnL.Add(realizedPnL)
	p.TotalCGTPaid = p.TotalCGTPaid.Add(totalCGT)

	// Update Position
	pos.Quantity = pos.Quantity.Sub(txn.Quantity)
	pos.CostBasis = pos.CostBasis.Sub(costBasis)
	pos.RealizedPnL = pos.RealizedPnL.Add(realizedPnL)
	pos.LastUpdated = txn.TradedOn

	if pos.Quantity.IsZero() {
		pos.CostBasis = marketdata.Zero()
		pos.AvgCostPerShare = marketdata.Zero()
	} else if p.CostBasisMethod == MethodWAC {
		avg, _ := pos.CostBasis.Div(pos.Quantity)
		pos.AvgCostPerShare = avg
	}

	return &txn, nil
}

func (p *PortfolioState) processBonusShare(txn Transaction) (*Transaction, error) {
	pos, exists := p.Positions[txn.Symbol]
	if !exists {
		return nil, fmt.Errorf("cannot apply bonus shares: no existing position in %s", txn.Symbol)
	}

	// Bonus shares increase share count at Rs 0 cost basis
	newQty := pos.Quantity.Add(txn.Quantity)
	pos.Quantity = newQty
	// Total cost basis remains unchanged, reducing average cost per share
	if newQty.IsPositive() {
		avg, _ := pos.CostBasis.Div(newQty)
		pos.AvgCostPerShare = avg
	}

	// Under FIFO: Add bonus share lot at 0 cost
	lot := &Lot{
		ID:                txn.ID,
		Symbol:            txn.Symbol,
		TransactionID:     txn.ID,
		PurchaseDate:      txn.TradedOn,
		OriginalQuantity:  txn.Quantity,
		RemainingQuantity: txn.Quantity,
		CostPerShare:      marketdata.Zero(),
		TotalCost:         marketdata.Zero(),
	}
	p.Lots[txn.Symbol] = append(p.Lots[txn.Symbol], lot)

	txn.GrossAmount = marketdata.Zero()
	txn.NetAmount = marketdata.Zero()
	txn.CostBasis = marketdata.Zero()
	return &txn, nil
}

func (p *PortfolioState) processRightsExercise(txn Transaction) (*Transaction, error) {
	pos, exists := p.Positions[txn.Symbol]
	if !exists {
		pos = &Position{
			Symbol:      txn.Symbol,
			LastUpdated: txn.TradedOn,
		}
		p.Positions[txn.Symbol] = pos
	}

	// Rights cost = Quantity * Subscription Price
	rightsCost := txn.Quantity.Mul(txn.Price)
	txn.GrossAmount = rightsCost
	txn.NetAmount = rightsCost

	// Deduct from cash ledger
	p.CashBalance = p.CashBalance.Sub(rightsCost)

	// Increase position
	newQty := pos.Quantity.Add(txn.Quantity)
	newCost := pos.CostBasis.Add(rightsCost)
	avg, _ := newCost.Div(newQty)

	pos.Quantity = newQty
	pos.CostBasis = newCost
	pos.AvgCostPerShare = avg

	// Record FIFO Lot
	lot := &Lot{
		ID:                txn.ID,
		Symbol:            txn.Symbol,
		TransactionID:     txn.ID,
		PurchaseDate:      txn.TradedOn,
		OriginalQuantity:  txn.Quantity,
		RemainingQuantity: txn.Quantity,
		CostPerShare:      txn.Price,
		TotalCost:         rightsCost,
	}
	p.Lots[txn.Symbol] = append(p.Lots[txn.Symbol], lot)

	txn.CostBasis = rightsCost
	return &txn, nil
}

// ValuePortfolio updates market prices, unrealized P&L and total portfolio value.
func (p *PortfolioState) ValuePortfolio(prices map[string]marketdata.Decimal) {
	totalMV := marketdata.Zero()
	totalCost := marketdata.Zero()
	hundred := marketdata.MustParse("100.0")

	for sym, pos := range p.Positions {
		if pos.Quantity.IsZero() {
			continue
		}
		if pr, ok := prices[sym]; ok {
			pos.MarketPrice = pr
		}
		mv := pos.Quantity.Mul(pos.MarketPrice)
		pos.MarketValue = mv
		pos.UnrealizedPnL = mv.Sub(pos.CostBasis)
		if pos.CostBasis.IsPositive() {
			pct, err := pos.UnrealizedPnL.Div(pos.CostBasis)
			if err == nil {
				pos.UnrealizedPnLPct = pct.Mul(hundred).Round(2)
			}
		}

		totalMV = totalMV.Add(mv)
		totalCost = totalCost.Add(pos.CostBasis)
	}

	p.TotalMarketValue = totalMV
	p.TotalCostBasis = totalCost
	p.TotalPortfolioValue = totalMV.Add(p.CashBalance)
	p.TotalUnrealizedPnL = totalMV.Sub(totalCost)

	// Total Return = RealizedPnL + UnrealizedPnL + Dividends - Fees - CGT
	p.TotalReturn = p.TotalRealizedPnL.
		Add(p.TotalUnrealizedPnL).
		Add(p.TotalDividends).
		Sub(p.TotalFeesPaid).
		Sub(p.TotalCGTPaid)
}

// SortTransactionsChronological sorts transactions by TradedOn ascending.
func SortTransactionsChronological(txns []Transaction) {
	sort.Slice(txns, func(i, j int) bool {
		return txns[i].TradedOn < txns[j].TradedOn
	})
}
