package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResearchQueryRequiresPayload(t *testing.T) {
	s := testServer(t)
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/research/query", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty query, got %d", rr.Code)
	}
}

func TestPortfolioResearchRequiresAuthentication(t *testing.T) {
	s := testServer(t)
	handler := s.Handler()

	// Querying portfolio scope without auth cookie must return 401
	payload := `{"query_text":"What is my portfolio return?","scope":"portfolio","portfolio_id":"pf-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/research/query", bytes.NewBufferString(payload))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated portfolio query, got %d", rr.Code)
	}
}

func TestPostDisclosureValidation(t *testing.T) {
	s := testServer(t)
	handler := s.Handler()

	// Invalid authority should be rejected with 400
	payload := `{"authority":"INVALID_AUTH","title":"Test circular"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/disclosures", bytes.NewBufferString(payload))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for invalid authority, got %d", rr.Code)
	}
}
