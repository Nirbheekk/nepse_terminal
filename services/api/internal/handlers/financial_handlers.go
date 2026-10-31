package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/services"
)

// handleGetFinancials returns reporting periods and full normalized statements.
func (s *Server) handleGetFinancials(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))
	if symbol == "" {
		badRequest(w, r, "missing company symbol")
		return
	}

	periods, err := s.store.GetFinancialPeriods(r.Context(), symbol)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to query financial periods")
		return
	}

	statements := make([]*services.Statement, 0, len(periods))
	for _, p := range periods {
		stmt, err := s.store.GetStatement(r.Context(), p.ID)
		if err == nil && stmt != nil {
			statements = append(statements, stmt)
		}
	}

	OK(w, r, map[string]any{
		"symbol":     symbol,
		"periods":    periods,
		"statements": statements,
	}, metaFrom(r))
}

type ingestStatementPayload struct {
	Period services.FinancialPeriod `json:"period"`
	Lines  []services.LineItem      `json:"lines"`
}

// handlePostFinancials ingests a normalized financial statement.
func (s *Server) handlePostFinancials(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))
	if symbol == "" {
		badRequest(w, r, "missing company symbol")
		return
	}

	var req ingestStatementPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, r, "invalid json payload")
		return
	}
	req.Period.Symbol = symbol

	stmt := services.NewStatement(req.Period)
	for _, line := range req.Lines {
		stmt.SetLine(line.StatementType, line.Code, line.Name, line.Amount, line.Order)
	}
	if err := stmt.Validate(); err != nil {
		badRequest(w, r, err.Error())
		return
	}

	periodID, err := s.store.UpsertFinancialPeriod(r.Context(), stmt.Period)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to save period")
		return
	}

	lineSlice := make([]services.LineItem, 0, len(stmt.Lines))
	for _, l := range stmt.Lines {
		lineSlice = append(lineSlice, l)
	}
	if err := s.store.UpsertStatementLines(r.Context(), periodID, lineSlice); err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to save lines")
		return
	}

	stmt.Period.ID = periodID
	Succeed(w, r, http.StatusCreated, stmt, metaFrom(r))
}

// handleGetRatios computes reporting-period-aware ratios.
func (s *Server) handleGetRatios(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))
	if symbol == "" {
		badRequest(w, r, "missing company symbol")
		return
	}

	asOfDate := r.URL.Query().Get("as_of")
	periods, err := s.store.GetFinancialPeriods(r.Context(), symbol)
	if err != nil || len(periods) == 0 {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "no financial statements found for symbol")
		return
	}

	// Select the latest period filed on or before asOfDate (no look-ahead bias)
	var activePeriod *services.FinancialPeriod
	for i := range periods {
		if asOfDate == "" || periods[i].FilingDate <= asOfDate {
			activePeriod = &periods[i]
			break
		}
	}
	if activePeriod == nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "no statements published on or before as-of date")
		return
	}

	stmt, err := s.store.GetStatement(r.Context(), activePeriod.ID)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load statement")
		return
	}

	// Determine price used: query parameter or latest close
	priceStr := r.URL.Query().Get("price")
	var price marketdata.Decimal
	if priceStr != "" {
		price, _ = marketdata.Parse(priceStr)
	} else {
		// Use latest bar close if available
		bars, err := s.store.GetDailyOHLCV(r.Context(), symbol, "", "")
		if err == nil && len(bars) > 0 {
			price = bars[len(bars)-1].Close
		}
	}

	computed, err := services.ComputeRatios(stmt, price, asOfDate)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, computed, metaFrom(r))
}

// handleGetCorporateActions returns corporate actions.
func (s *Server) handleGetCorporateActions(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))
	if symbol == "" {
		symbol = strings.ToUpper(r.URL.Query().Get("symbol"))
	}
	actType := r.URL.Query().Get("action_type")

	actions, err := s.store.GetCorporateActions(r.Context(), symbol, actType)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load corporate actions")
		return
	}

	OK(w, r, actions, metaFrom(r))
}

// handlePostCorporateAction registers a corporate action.
func (s *Server) handlePostCorporateAction(w http.ResponseWriter, r *http.Request) {
	var act services.CorporateAction
	if err := json.NewDecoder(r.Body).Decode(&act); err != nil {
		badRequest(w, r, "invalid corporate action payload")
		return
	}

	symParam := strings.ToUpper(chi.URLParam(r, "symbol"))
	if symParam != "" {
		act.Symbol = symParam
	}
	act.Symbol = strings.ToUpper(act.Symbol)

	if act.Symbol == "" || act.ActionType == "" || act.BookClosureDate == "" {
		badRequest(w, r, "symbol, action_type and book_closure_date are required")
		return
	}

	if err := s.store.UpsertCorporateAction(r.Context(), act); err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to save corporate action")
		return
	}

	Succeed(w, r, http.StatusCreated, act, metaFrom(r))
}

// handleGetAdjustedPrices returns historical pricing with corporate-action adjustments.
func (s *Server) handleGetAdjustedPrices(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))
	if symbol == "" {
		badRequest(w, r, "missing company symbol")
		return
	}

	policyStr := r.URL.Query().Get("policy")
	policy := services.AdjustmentPolicy(policyStr)
	if policy == "" {
		policy = services.PolicySplitBonus
	}

	fromDate := r.URL.Query().Get("from")
	toDate := r.URL.Query().Get("to")

	bars, err := s.store.GetDailyOHLCV(r.Context(), symbol, fromDate, toDate)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to query daily OHLCV")
		return
	}

	actions, err := s.store.GetCorporateActions(r.Context(), symbol, "")
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to query corporate actions")
		return
	}

	adjBars, err := services.AdjustPrices(bars, actions, policy)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, err.Error())
		return
	}

	OK(w, r, map[string]any{
		"symbol": symbol,
		"policy": policy,
		"bars":   adjBars,
	}, metaFrom(r))
}
