package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/repositories"
)

func (s *Server) handleListPublicIssues(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(r.URL.Query().Get("symbol"))
	rows, err := s.store.ListPublicIssues(r.Context(), symbol)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load public issues")
		return
	}
	if rows == nil {
		rows = []repositories.PublicIssueRow{}
	}
	OK(w, r, rows, metaFrom(r))
}

func (s *Server) handleListMergers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListMergers(r.Context())
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load mergers")
		return
	}
	if rows == nil {
		rows = []repositories.MergerRow{}
	}
	OK(w, r, rows, metaFrom(r))
}

func (s *Server) handleListCompanyMeetings(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(r.URL.Query().Get("symbol"))
	if symbol == "" {
		symbol = strings.ToUpper(chi.URLParam(r, "symbol"))
	}
	rows, err := s.store.ListCompanyMeetings(r.Context(), symbol)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load meetings")
		return
	}
	if rows == nil {
		rows = []repositories.CompanyMeetingRow{}
	}
	OK(w, r, rows, metaFrom(r))
}
