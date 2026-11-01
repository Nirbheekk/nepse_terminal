package handlers

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/repositories"
)

func (s *Server) handleGetCompanyProfile(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))
	if symbol == "" {
		badRequest(w, r, "missing company symbol")
		return
	}
	profile, err := s.store.GetCompanyProfile(r.Context(), symbol)
	if err != nil {
		// Not found is acceptable: return empty profile placeholder.
		OK(w, r, map[string]any{"symbol": symbol, "profile": nil, "available": false}, metaFrom(r))
		return
	}
	OK(w, r, map[string]any{"symbol": symbol, "profile": profile, "available": true}, metaFrom(r))
}

func (s *Server) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))
	if symbol == "" {
		badRequest(w, r, "missing company symbol")
		return
	}
	members, err := s.store.ListBoardMembers(r.Context(), symbol)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load board")
		return
	}
	if members == nil {
		members = []repositories.BoardMember{}
	}
	OK(w, r, map[string]any{"symbol": symbol, "members": members}, metaFrom(r))
}

func (s *Server) handleGetOwnership(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))
	if symbol == "" {
		badRequest(w, r, "missing company symbol")
		return
	}
	snaps, err := s.store.ListOwnershipSnapshots(r.Context(), symbol)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load ownership")
		return
	}
	if snaps == nil {
		snaps = []repositories.OwnershipSnapshot{}
	}
	OK(w, r, map[string]any{"symbol": symbol, "snapshots": snaps}, metaFrom(r))
}

func (s *Server) handleListMutualFunds(w http.ResponseWriter, r *http.Request) {
	funds, err := s.store.ListMutualFunds(r.Context())
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load mutual funds")
		return
	}
	if funds == nil {
		funds = []repositories.MutualFund{}
	}
	OK(w, r, funds, metaFrom(r))
}

func (s *Server) handleGetMutualFundNAV(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	nav, err := s.store.ListMutualFundNAV(r.Context(), id)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load nav")
		return
	}
	if nav == nil {
		nav = []repositories.MutualFundNAV{}
	}
	OK(w, r, map[string]any{"fund_id": id, "nav": nav}, metaFrom(r))
}

func (s *Server) handleDataCoverage(w http.ResponseWriter, r *http.Request) {
	cov, err := s.store.DataCoverage(r.Context())
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to load coverage")
		return
	}
	OK(w, r, cov, metaFrom(r))
}

// end of file
