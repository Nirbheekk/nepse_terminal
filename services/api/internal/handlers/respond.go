package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Error codes are part of the public contract (see openapi.yaml). Clients switch
// on these, never on prose.
const (
	CodeBadRequest      = "BAD_REQUEST"
	CodeValidation      = "VALIDATION_FAILED"
	CodeUnauthenticated = "UNAUTHENTICATED"
	CodeForbidden       = "FORBIDDEN"
	CodeNotFound        = "NOT_FOUND"
	CodeConflict        = "CONFLICT"
	CodeRateLimited     = "RATE_LIMITED"
	CodeCSRF            = "CSRF_FAILED"
	CodeInternal        = "INTERNAL"
	CodeUnavailable     = "SERVICE_UNAVAILABLE"
)

// Meta travels with every response so a client never has to infer freshness.
type Meta struct {
	RequestID    string `json:"request_id"`
	AsOf         string `json:"as_of,omitempty"`
	Source       string `json:"source,omitempty"`
	LicenseClass string `json:"license_class,omitempty"`
	Freshness    string `json:"freshness,omitempty"`
	// FreshnessReason explains the verdict in words, so a client can show why a
	// figure is final or stale instead of a bare colour.
	FreshnessReason string `json:"freshness_reason,omitempty"`
	// MarketState is the exchange state the verdict was made against.
	MarketState string    `json:"market_state,omitempty"`
	SessionDate string    `json:"session_date,omitempty"`
	AgeSeconds  *int64    `json:"age_seconds,omitempty"`
	Method      string    `json:"calculation,omitempty"`
	Coverage    *Coverage `json:"coverage,omitempty"`
	Total       *int      `json:"total,omitempty"`
	NextCursor  string    `json:"next_cursor,omitempty"`
}

// Coverage states how complete a derived figure is.
type Coverage struct {
	Covered  int    `json:"covered"`
	Eligible int    `json:"eligible"`
	Unit     string `json:"unit"`
}

// APIError is the machine-readable failure shape.
type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Details   any    `json:"details,omitempty"`
}

// Envelope is the single response wrapper used by every endpoint.
type Envelope struct {
	Data  any       `json:"data"`
	Meta  Meta      `json:"meta"`
	Error *APIError `json:"error"`
}

func metaFrom(r *http.Request) Meta {
	return Meta{RequestID: RequestIDFrom(r.Context())}
}

func writeJSON(w http.ResponseWriter, status int, body Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("encode response", "error", err)
	}
}

// OK writes a 200 success envelope.
func OK(w http.ResponseWriter, r *http.Request, data any, meta Meta) {
	Succeed(w, r, http.StatusOK, data, meta)
}

// Succeed writes a success envelope with an explicit status, so a handler that
// creates a resource can return 201 rather than a misleading 200.
func Succeed(w http.ResponseWriter, r *http.Request, status int, data any, meta Meta) {
	if meta.RequestID == "" {
		meta = metaFrom(r)
	}
	writeJSON(w, status, Envelope{Data: data, Meta: meta})
}

// Fail writes an error envelope and never leaks internal error text.
func Fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	meta := metaFrom(r)
	writeJSON(w, status, Envelope{
		Meta:  meta,
		Error: &APIError{Code: code, Message: message, Retryable: status >= 500},
	})
}

func badRequest(w http.ResponseWriter, r *http.Request, msg string) {
	Fail(w, r, http.StatusBadRequest, CodeBadRequest, msg)
}

func unauthenticated(w http.ResponseWriter, r *http.Request) {
	Fail(w, r, http.StatusUnauthorized, CodeUnauthenticated, "authentication required")
}
