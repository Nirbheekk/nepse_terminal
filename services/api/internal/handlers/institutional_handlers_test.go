package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstitutionalEndpointsRequireAuth(t *testing.T) {
	s := testServer(t)
	handler := s.Handler()

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/workspaces"},
		{http.MethodPost, "/api/v1/workspaces"},
		{http.MethodGet, "/api/v1/workspaces/org-1/members"},
		{http.MethodPost, "/api/v1/workspaces/org-1/members"},
		{http.MethodGet, "/api/v1/workspaces/org-1/portfolios"},
		{http.MethodPost, "/api/v1/workspaces/org-1/portfolios"},
		{http.MethodGet, "/api/v1/workspaces/org-1/notes"},
		{http.MethodPost, "/api/v1/workspaces/org-1/notes"},
		{http.MethodGet, "/api/v1/workspaces/org-1/approvals"},
		{http.MethodGet, "/api/v1/workspaces/org-1/portfolios/pf-1/liquidity"},
		{http.MethodGet, "/api/v1/workspaces/org-1/portfolios/pf-1/concentration"},
		{http.MethodPost, "/api/v1/workspaces/org-1/portfolios/pf-1/stress"},
		{http.MethodGet, "/api/v1/workspaces/org-1/portfolios/pf-1/export.csv"},
		{http.MethodGet, "/api/v1/workspaces/org-1/audit"},
	}

	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, bytes.NewBufferString(`{}`))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without auth: expected 401 Unauthorized, got %d", rt.method, rt.path, rr.Code)
		}
	}
}
