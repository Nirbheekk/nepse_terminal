package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nepse/terminal/api/internal/services"
)

func TestDeveloperEndpointsRequireAuth(t *testing.T) {
	s := testServer(t)
	handler := s.Handler()

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/developer/keys"},
		{http.MethodPost, "/api/v1/developer/keys"},
		{http.MethodDelete, "/api/v1/developer/keys/key-123"},
		{http.MethodGet, "/api/v1/exports"},
		{http.MethodPost, "/api/v1/exports"},
		{http.MethodGet, "/api/v1/exports/exp-123"},
		{http.MethodGet, "/api/v1/exports/exp-123/download"},
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

func TestLicensingMatrixPublicEndpoint(t *testing.T) {
	s := testServer(t)
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/developer/licensing-matrix", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for licensing matrix, got %d", rr.Code)
	}

	var resp struct {
		Data map[services.Dataset]services.LicensingMatrixEntry `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	liveEntry, exists := resp.Data[services.DatasetMarketLive]
	if !exists {
		t.Fatal("expected market_live dataset in licensing matrix")
	}
	if liveEntry.RedistributionPolicy != "prohibited" {
		t.Errorf("expected market_live redistribution to be prohibited, got %s", liveEntry.RedistributionPolicy)
	}
	if liveEntry.LicenseClass != "public_unverified" {
		t.Errorf("expected market_live license class public_unverified, got %s", liveEntry.LicenseClass)
	}

	finEntry, exists := resp.Data[services.DatasetFinancials]
	if !exists {
		t.Fatal("expected financials dataset in licensing matrix")
	}
	if finEntry.RedistributionPolicy != "permitted_with_attribution" {
		t.Errorf("expected financials redistribution permitted_with_attribution, got %s", finEntry.RedistributionPolicy)
	}
}
