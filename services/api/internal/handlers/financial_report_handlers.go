package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/repositories"
)

func (s *Server) handleListFinancialReports(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(r.URL.Query().Get("symbol"))
	if symbol == "" {
		symbol = strings.ToUpper(chi.URLParam(r, "symbol"))
	}
	rows, err := s.store.ListFinancialReports(r.Context(), symbol)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load financial reports")
		return
	}
	if rows == nil {
		rows = []repositories.FinancialReportRow{}
	}
	OK(w, r, rows, metaFrom(r))
}

func (s *Server) handleFinancialCoverage(w http.ResponseWriter, r *http.Request) {
	cov, err := s.store.FinancialReportCoverage(r.Context())
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load financial coverage")
		return
	}
	OK(w, r, cov, metaFrom(r))
}
