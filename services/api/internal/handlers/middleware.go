package handlers

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"nepse/terminal/api/internal/auth"
	"nepse/terminal/api/internal/config"
	"nepse/terminal/api/internal/services"
)

type ctxKey string

const (
	ctxRequestID ctxKey = "request_id"
	ctxClaims    ctxKey = "claims"
)

// RequestIDFrom returns the correlation id for the current request.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRequestID).(string); ok {
		return v
	}
	return ""
}

// ClaimsFrom returns the authenticated claims, if any.
func ClaimsFrom(ctx context.Context) (*auth.Claims, bool) {
	c, ok := ctx.Value(ctxClaims).(*auth.Claims)
	return c, ok
}

func newRequestID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "req-unknown"
	}
	return hex.EncodeToString(buf)
}

// requestID respects an inbound X-Request-Id (so a proxy or the web tier can
// correlate logs) but always echoes one back.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if id == "" || len(id) > 128 {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
}

// securityHeaders are applied to every response, including errors.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// requestLogger emits one structured line per request.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFrom(r.Context()),
				"remote", clientIP(r),
			)
		})
	}
}

func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic", "recovered", rec, "path", r.URL.Path, "request_id", RequestIDFrom(r.Context()))
					Fail(w, r, http.StatusInternalServerError, CodeInternal, "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP prefers the proxy header only when the direct peer is loopback, so a
// remote caller cannot spoof its address.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "127.0.0.1" || host == "::1" {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if first := strings.TrimSpace(strings.Split(fwd, ",")[0]); first != "" {
				return first
			}
		}
	}
	return host
}

// cors applies an explicit allowlist. Requests without Origin (server-to-server,
// curl) are allowed; a disallowed Origin is rejected outright for mutations.
func cors(cfg *config.Config) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimRight(r.Header.Get("Origin"), "/")
			if origin != "" {
				if allowed[origin] {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Request-Id")
					// PUT must be listed: /alerts/{id}, /watchlists/{id} and
					// /screener/screens/{id} all update via PUT, and a method missing
					// from this list fails the browser's preflight check with
					// ERR_FAILED even though the route exists and answers curl.
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Add("Vary", "Origin")
				} else if r.Method != http.MethodGet && r.Method != http.MethodHead {
					Fail(w, r, http.StatusForbidden, CodeForbidden, "origin not allowed")
					return
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// limiterMap is a per-key token bucket with idle eviction, so memory cannot grow
// without bound as new clients arrive.
type limiterMap struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    rate.Limit // sustained requests per second
	burst   int        // allowance per minute, spent immediately
}

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

func newLimiterMap(rpm int) *limiterMap {
	m := &limiterMap{
		buckets: map[string]*bucket{},
		rate:    rate.Limit(float64(rpm) / 60.0),
		burst:   rpm,
	}
	go m.evictLoop()
	return m
}

func (m *limiterMap) allow(key string) bool {
	m.mu.Lock()
	b, ok := m.buckets[key]
	now := time.Now()
	if !ok {
		b = &bucket{lim: rate.NewLimiter(m.rate, m.burst)}
		m.buckets[key] = b
	}
	b.seen = now
	m.mu.Unlock()
	return b.lim.Allow()
}

func (m *limiterMap) evictLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute)
		m.mu.Lock()
		for k, b := range m.buckets {
			if b.seen.Before(cutoff) {
				delete(m.buckets, k)
			}
		}
		m.mu.Unlock()
	}
}

func rateLimit(m *limiterMap) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !m.allow(clientIP(r)) {
				w.Header().Set("Retry-After", "60")
				Fail(w, r, http.StatusTooManyRequests, CodeRateLimited, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerOrCookie extracts the access token: cookie first (browser), then
// Authorization header (tooling).
func bearerOrCookie(r *http.Request, cookieName string) string {
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		return c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

// requireAuth rejects unauthenticated requests before they reach a handler.
// Supports JWT access cookie, Bearer JWT token, and cryptographic developer API keys.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Check for Developer API Key (header X-API-Key or Bearer np_live_...)
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer np_live_") {
				apiKey = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
			}
		}
		if apiKey != "" {
			hash := services.HashKey(apiKey)
			devKey, err := s.store.GetDeveloperKeyByHash(r.Context(), hash)
			if err != nil || devKey == nil {
				unauthenticated(w, r)
				return
			}
			if err := devKey.RecordRequest(); err != nil {
				w.Header().Set("Retry-After", "3600")
				Fail(w, r, http.StatusTooManyRequests, CodeRateLimited, err.Error())
				return
			}
			_ = s.store.IncrementDeveloperKeyUsage(r.Context(), devKey.ID)

			claims := &auth.Claims{
				Roles: []string{"developer"},
			}
			claims.Subject = devKey.UserID
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxClaims, claims)))
			return
		}

		// 2. Cookie or Bearer JWT token
		raw := bearerOrCookie(r, accessCookie)
		if raw == "" {
			unauthenticated(w, r)
			return
		}
		claims, err := s.tokens.Parse(raw)
		if err != nil {
			unauthenticated(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxClaims, claims)))
	})
}

// requireCSRF implements double-submit validation for cookie-authenticated
// mutations: the header must match the readable CSRF cookie.
// Programmatic API callers (using Authorization: Bearer or X-API-Key) bypass CSRF.
func requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" {
			next.ServeHTTP(w, r)
			return
		}
		csrfCookie, err := r.Cookie(csrfCookie)
		header := r.Header.Get("X-CSRF-Token")
		if err != nil || csrfCookie.Value == "" || header == "" ||
			subtle.ConstantTimeCompare([]byte(csrfCookie.Value), []byte(header)) != 1 {
			Fail(w, r, http.StatusForbidden, CodeCSRF, "missing or mismatched CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}
