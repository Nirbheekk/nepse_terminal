package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFinancialEndpointsRequireSymbol(t *testing.T) {
	s := testServer(t)
	handler := s.Handler()

	// Financials without symbol
	req := httptest.NewRequest(http.MethodGet, "/api/v1/companies//financials", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 or 404, got %d", rr.Code)
	}

	// Ratios without symbol
	req = httptest.NewRequest(http.MethodGet, "/api/v1/companies//ratios", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 or 404, got %d", rr.Code)
	}
}

func TestPortfolioEndpointsRequireAuthentication(t *testing.T) {
	s := testServer(t)
	handler := s.Handler()

	// GET /api/v1/portfolios without auth cookie
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolios", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
	}
	env := decode(t, rr)
	if env.Error == nil || env.Error.Code != CodeUnauthenticated {
		t.Errorf("expected CodeUnauthenticated, got %+v", env.Error)
	}

	// POST /api/v1/portfolios without auth
	req = httptest.NewRequest(http.MethodPost, "/api/v1/portfolios", bytes.NewBufferString(`{"name":"My Portfolio"}`))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
	}

	// POST /api/v1/portfolios/123/transactions without auth
	req = httptest.NewRequest(http.MethodPost, "/api/v1/portfolios/123/transactions", bytes.NewBufferString(`{"symbol":"NABIL","txn_type":"BUY"}`))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
	}
}
