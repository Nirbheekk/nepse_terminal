package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/portfolios"
	"nepse/terminal/api/internal/services"
)

// handleListPortfolios returns all portfolios for the authenticated user.
func (s *Server) handleListPortfolios(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	pfs, err := s.store.ListPortfolios(r.Context(), uid)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to query portfolios")
		return
	}

	OK(w, r, pfs, metaFrom(r))
}

type createPortfolioRequest struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	CostBasisMethod string `json:"cost_basis_method"` // FIFO or WAC (default WAC)
	BenchmarkSymbol string `json:"benchmark_symbol"`  // default NEPSE
}

// handleCreatePortfolio creates a new portfolio tracker.
func (s *Server) handleCreatePortfolio(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	var req createPortfolioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, r, "invalid json payload")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, r, "portfolio name is required")
		return
	}

	pf, err := s.store.CreatePortfolio(r.Context(), uid, req.Name, req.Description, req.CostBasisMethod, req.BenchmarkSymbol)
	if err != nil {
		Fail(w, r, http.StatusConflict, CodeConflict, err.Error())
		return
	}

	Succeed(w, r, http.StatusCreated, pf, metaFrom(r))
}

// handleGetPortfolio retrieves the full portfolio state including positions and P&L.
func (s *Server) handleGetPortfolio(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	id := chi.URLParam(r, "id")
	pf, err := s.store.GetPortfolio(r.Context(), id, uid)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "portfolio not found")
		return
	}

	// Enrich positions with latest market close prices if available
	prices := make(map[string]marketdata.Decimal)
	for sym := range pf.Positions {
		bars, err := s.store.GetDailyOHLCV(r.Context(), sym, "", "")
		if err == nil && len(bars) > 0 {
			prices[sym] = bars[len(bars)-1].Close
		}
	}
	pf.ValuePortfolio(prices)

	OK(w, r, pf, metaFrom(r))
}

// handleDeletePortfolio deletes a portfolio.
func (s *Server) handleDeletePortfolio(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	id := chi.URLParam(r, "id")
	if err := s.store.DeletePortfolio(r.Context(), id, uid); err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, err.Error())
		return
	}

	OK(w, r, map[string]string{"status": "deleted"}, metaFrom(r))
}

// handleGetTransactions returns paginated transactions for a portfolio.
func (s *Server) handleGetTransactions(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	id := chi.URLParam(r, "id")
	// Verify ownership
	if _, err := s.store.GetPortfolio(r.Context(), id, uid); err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "portfolio not found")
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 && val <= 500 {
			limit = val
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	txns, err := s.store.GetPortfolioTransactions(r.Context(), id, limit, offset)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load transactions")
		return
	}

	OK(w, r, txns, metaFrom(r))
}

// handleRecordTransaction executes a portfolio transaction against the state.
func (s *Server) handleRecordTransaction(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	id := chi.URLParam(r, "id")
	pf, err := s.store.GetPortfolio(r.Context(), id, uid)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "portfolio not found")
		return
	}

	var txn portfolios.Transaction
	if err := json.NewDecoder(r.Body).Decode(&txn); err != nil {
		badRequest(w, r, "invalid transaction payload")
		return
	}
	txn.PortfolioID = id
	txn.Symbol = strings.ToUpper(strings.TrimSpace(txn.Symbol))

	processed, err := pf.ProcessTransaction(txn)
	if err != nil {
		badRequest(w, r, err.Error())
		return
	}

	// Persist transaction record
	txnID, err := s.store.RecordPortfolioTransaction(r.Context(), processed)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to record transaction: "+err.Error())
		return
	}
	processed.ID = txnID

	// Persist state updates
	if err := s.store.SavePortfolioState(r.Context(), pf); err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to save portfolio state: "+err.Error())
		return
	}

	Succeed(w, r, http.StatusCreated, processed, metaFrom(r))
}

// handleGetLots returns tax lots for a portfolio (FIFO tracking).
func (s *Server) handleGetLots(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	id := chi.URLParam(r, "id")
	pf, err := s.store.GetPortfolio(r.Context(), id, uid)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "portfolio not found")
		return
	}

	sym := strings.ToUpper(r.URL.Query().Get("symbol"))
	if sym != "" {
		OK(w, r, pf.Lots[sym], metaFrom(r))
		return
	}

	OK(w, r, pf.Lots, metaFrom(r))
}

// handleGetPortfolioAnalytics computes benchmark comparison and risk analytics.
func (s *Server) handleGetPortfolioAnalytics(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(r)
	if !ok {
		unauthenticated(w, r)
		return
	}

	id := chi.URLParam(r, "id")
	if _, err := s.store.GetPortfolio(r.Context(), id, uid); err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "portfolio not found")
		return
	}

	fromDate := r.URL.Query().Get("from")
	toDate := r.URL.Query().Get("to")

	obs, err := s.store.GetDailyObservations(r.Context(), id, fromDate, toDate)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load daily observations")
		return
	}

	if len(obs) < 2 {
		Fail(w, r, http.StatusUnprocessableEntity, CodeValidation, "at least 2 daily observations required for risk analytics")
		return
	}

	rfStr := r.URL.Query().Get("risk_free_rate")
	rf := marketdata.MustParse("0.05") // 5% default Nepal Treasury rate
	if rfStr != "" {
		if parsed, err := marketdata.Parse(rfStr); err == nil {
			rf = parsed
		}
	}

	metrics, err := services.ComputeAnalytics(obs, rf)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, metrics, metaFrom(r))
}
