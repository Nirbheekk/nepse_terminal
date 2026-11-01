package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/repositories"
)

func (s *Server) handleGetMutualFund(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := s.store.GetMutualFund(r.Context(), id)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "mutual fund not found")
		return
	}
	OK(w, r, m, metaFrom(r))
}

func (s *Server) handleListMutualFundHoldings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := s.store.ListMutualFundHoldings(r.Context(), id)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load holdings")
		return
	}
	if rows == nil {
		rows = []repositories.MutualFundHoldingRow{}
	}
	OK(w, r, rows, metaFrom(r))
}

func (s *Server) handleMutualFundCoverage(w http.ResponseWriter, r *http.Request) {
	cov, err := s.store.MutualFundCoverage(r.Context())
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load coverage")
		return
	}
	OK(w, r, cov, metaFrom(r))
}

func (s *Server) handleMutualFundAllocations(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	OK(w, r, map[string]any{"fund_id": id, "allocations": []any{}, "missing_data": true}, metaFrom(r))
}

func (s *Server) handleMutualFundFinancials(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	OK(w, r, map[string]any{"fund_id": id, "financials": []any{}, "missing_data": true}, metaFrom(r))
}

func (s *Server) handleMutualFundPerformance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	OK(w, r, map[string]any{"fund_id": id, "performance": map[string]any{}, "missing_data": true}, metaFrom(r))
}
