package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/research"
	"nepse/terminal/api/internal/services"
)

// handleListDisclosures returns authorized disclosures matching filters.
func (s *Server) handleListDisclosures(w http.ResponseWriter, r *http.Request) {
	authority := strings.ToUpper(r.URL.Query().Get("authority"))
	symbol := strings.ToUpper(r.URL.Query().Get("symbol"))
	category := strings.ToLower(r.URL.Query().Get("category"))

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 && val <= 100 {
			limit = val
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	discs, err := s.store.ListDisclosures(r.Context(), authority, symbol, category, limit, offset)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to query disclosures")
		return
	}

	OK(w, r, discs, metaFrom(r))
}

// handleGetDisclosure retrieves a single disclosure with its attachments and licensing metadata.
func (s *Server) handleGetDisclosure(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		badRequest(w, r, "missing disclosure id")
		return
	}

	disc, err := s.store.GetDisclosure(r.Context(), id)
	if err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "disclosure not found")
		return
	}

	OK(w, r, disc, metaFrom(r))
}

// handlePostDisclosure ingests an authorized disclosure with attachments.
func (s *Server) handlePostDisclosure(w http.ResponseWriter, r *http.Request) {
	var d services.Disclosure
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		badRequest(w, r, "invalid json payload")
		return
	}

	if err := d.Validate(); err != nil {
		badRequest(w, r, err.Error())
		return
	}

	id, err := s.store.InsertDisclosure(r.Context(), d)
	if err != nil {
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "failed to store disclosure: "+err.Error())
		return
	}
	d.ID = id

	Succeed(w, r, http.StatusCreated, d, metaFrom(r))
}

// handleResearchQuery handles an evidence-grounded research query.
func (s *Server) handleResearchQuery(w http.ResponseWriter, r *http.Request) {
	var req research.ResearchQuery
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, r, "invalid research query payload")
		return
	}

	// Extract user ID from auth context if present
	if uid, ok := userIDFrom(r); ok {
		req.UserID = uid
	}

	// Guard portfolio-scoped queries
	if req.Scope == "portfolio" || req.PortfolioID != "" {
		if req.UserID == "" {
			Fail(w, r, http.StatusUnauthorized, CodeUnauthenticated, "authentication required to access portfolio evidence")
			return
		}
	}

	assistant := research.NewAssistant(s.store)
	resp, err := assistant.Execute(r.Context(), req)
	if err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			Fail(w, r, http.StatusForbidden, CodeForbidden, err.Error())
			return
		}
		badRequest(w, r, err.Error())
		return
	}

	// Persist the query and citations for auditability
	_, _ = s.store.RecordResearchQuery(r.Context(), req, resp)

	OK(w, r, resp, metaFrom(r))
}
