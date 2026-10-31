package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/auth"
	"nepse/terminal/api/internal/config"
	"nepse/terminal/api/internal/repositories"
)

func testConfig() *config.Config {
	return &config.Config{
		Env:            "test",
		HTTPAddr:       ":0",
		JWTSecret:      []byte("0123456789abcdef0123456789abcdef"),
		JWTIssuer:      "nepse-platform",
		AccessTTL:      15 * time.Minute,
		RefreshTTL:     720 * time.Hour,
		AllowedOrigins: []string{"http://localhost:3100"},
		RateLimitRPM:   600,
	}
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(testConfig(), nil, silentLogger(), repositories.MigrationStatus{Applied: 1, Latest: "0001_init"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func decode(t *testing.T, rr *httptest.ResponseRecorder) Envelope {
	t.Helper()
	var env Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not a JSON envelope: %v (%s)", err, rr.Body.String())
	}
	return env
}

// ------------------------------------------------------------------ envelope

func TestOKWritesEnvelopeWithMetadata(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/summary", nil)

	OK(rr, req, map[string]string{"hello": "world"}, Meta{RequestID: "abc"})

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cache := rr.Header().Get("Cache-Control"); cache != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store for personalised data", cache)
	}
	env := decode(t, rr)
	if env.Meta.RequestID != "abc" {
		t.Errorf("request_id = %q", env.Meta.RequestID)
	}
	if env.Error != nil {
		t.Errorf("unexpected error in success envelope: %+v", env.Error)
	}
}

func TestFailNeverLeaksInternalDetailAndMarksRetryable(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)

	Fail(rr, req, http.StatusInternalServerError, CodeInternal, "internal error")

	env := decode(t, rr)
	if env.Error == nil {
		t.Fatal("expected an error object")
	}
	if env.Error.Code != CodeInternal {
		t.Errorf("code = %q", env.Error.Code)
	}
	if !env.Error.Retryable {
		t.Error("5xx errors should be marked retryable")
	}

	rr = httptest.NewRecorder()
	Fail(rr, req, http.StatusBadRequest, CodeBadRequest, "bad")
	if decode(t, rr).Error.Retryable {
		t.Error("4xx errors should not be marked retryable")
	}
}

// ------------------------------------------------------------------ csrf

func TestCSRFRequiresMatchingHeaderAndCookie(t *testing.T) {
	h := requireCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		name   string
		method string
		cookie string
		header string
		want   int
	}{
		{"matching tokens", http.MethodPost, "tok123", "tok123", http.StatusNoContent},
		{"missing header", http.MethodPost, "tok123", "", http.StatusForbidden},
		{"missing cookie", http.MethodPost, "", "tok123", http.StatusForbidden},
		{"mismatched", http.MethodPost, "tok123", "tok124", http.StatusForbidden},
		{"GET is exempt", http.MethodGet, "", "", http.StatusNoContent},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/v1/auth/sessions/x", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: csrfCookie, Value: tc.cookie})
			}
			if tc.header != "" {
				req.Header.Set("X-CSRF-Token", tc.header)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d", rr.Code, tc.want)
			}
			if tc.want == http.StatusForbidden {
				if code := decode(t, rr).Error.Code; code != CodeCSRF {
					t.Errorf("error code = %q, want %q", code, CodeCSRF)
				}
			}
		})
	}
}

// ------------------------------------------------------------------ auth

func TestRequireAuthRejectsAnonymousAndAcceptsToken(t *testing.T) {
	s := testServer(t)
	protected := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFrom(r.Context())
		if !ok || claims.Subject == "" {
			t.Error("claims missing from the request context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	protected.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", rr.Code)
	}

	token, _, err := s.tokens.Issue("user-42", []string{"user"})
	if err != nil {
		t.Fatal(err)
	}

	for name, attach := range map[string]func(*http.Request){
		"cookie": func(r *http.Request) { r.AddCookie(&http.Cookie{Name: accessCookie, Value: token}) },
		"bearer": func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) },
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			attach(req)
			rr := httptest.NewRecorder()
			protected.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
		})
	}

	// A token signed with a different key must not pass.
	forged, _, err := auth.NewManager([]byte("ffffffffffffffffffffffffffffffff"), "nepse-platform", time.Minute).
		Issue("attacker", []string{"admin"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: accessCookie, Value: forged})
	rr = httptest.NewRecorder()
	protected.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("forged token status = %d, want 401", rr.Code)
	}
}

// ------------------------------------------------------------------ cors

func TestCORSAllowlist(t *testing.T) {
	cfg := testConfig()
	h := cors(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Allowed origin is echoed and credentials are permitted.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/summary", nil)
	req.Header.Set("Origin", "http://localhost:3100")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3100" {
		t.Fatalf("Allow-Origin = %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Allow-Credentials = %q", got)
	}

	// A mutation from an unlisted origin is refused outright.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://evil.example")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST status = %d, want 403", rr.Code)
	}

	// And the browser is never told it may proceed.
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin received Allow-Origin %q", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	h := cors(testConfig())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("preflight reached the handler")
	}))
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3100")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rr.Code)
	}

	// Every method the router serves must be advertised, or the browser blocks
	// the request after a successful preflight. PUT was missing here while
	// PUT /alerts/{id} existed, which made "pause alert" fail silently.
	methods := rr.Header().Get("Access-Control-Allow-Methods")
	for _, want := range []string{"GET", "POST", "PUT", "DELETE"} {
		if !strings.Contains(methods, want) {
			t.Errorf("Allow-Methods %q does not include %s", methods, want)
		}
	}
}

// ------------------------------------------------------------------ input

func TestQueryIntBounds(t *testing.T) {
	cases := []struct {
		raw     string
		def     int
		min     int
		max     int
		want    int
		wantErr bool
	}{
		{"", 10, 1, 50, 10, false},
		{"25", 10, 1, 50, 25, false},
		{"0", 10, 1, 50, 0, true},
		{"51", 10, 1, 50, 0, true},
		{"-1", 10, 1, 50, 0, true},
		{"abc", 10, 1, 50, 0, true},
		{" 7 ", 10, 1, 50, 7, false},
		{"1e3", 10, 1, 2000, 0, true},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/?n="+url.QueryEscape(tc.raw), nil)
		got, err := queryInt(req, "n", tc.def, tc.min, tc.max)
		if tc.wantErr {
			if err == nil {
				t.Errorf("queryInt(%q) accepted an out-of-range value", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("queryInt(%q) unexpected error: %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("queryInt(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestParseCompanyFilterRejectsUnsafeInput(t *testing.T) {
	cases := []struct {
		query     string
		wantErr   bool
		wantSort  string
		wantLimit int
		wantDesc  bool
	}{
		{"", false, "symbol", 50, false},
		{"?q=nabil&limit=25", false, "symbol", 25, false},
		{"?sort=market_cap&order=desc", false, "market_cap", 50, true},
		// An unknown sort must not silently fall back: the caller would be told
		// it received a market-cap ranking while getting alphabetical order.
		{"?sort=drop%20table", true, "", 0, false},
		{"?order=sideways", true, "", 0, false},
		{"?sector=Commercial%20Banks!%27", true, "", 0, false},
		{"?limit=0", true, "", 0, false},
		{"?limit=201", true, "", 0, false},
		{"?offset=-1", true, "", 0, false},
		{"?q=" + strings.Repeat("x", 65), true, "", 0, false},
		{"?status=pending", true, "", 0, false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/companies"+tc.query, nil)
		rr := httptest.NewRecorder()
		f, ok := parseCompanyFilter(rr, req, 50, 200)
		if tc.wantErr {
			if ok {
				t.Errorf("parseCompanyFilter(%q) accepted an invalid query", tc.query)
			}
			if rr.Code != http.StatusBadRequest {
				t.Errorf("parseCompanyFilter(%q) status = %d, want 400", tc.query, rr.Code)
			}
			continue
		}
		if !ok {
			t.Errorf("parseCompanyFilter(%q) rejected a valid query: %s", tc.query, rr.Body.String())
			continue
		}
		if f.Sort != tc.wantSort || f.Limit != tc.wantLimit || f.Descending != tc.wantDesc {
			t.Errorf("parseCompanyFilter(%q) = sort %q limit %d desc %v", tc.query, f.Sort, f.Limit, f.Descending)
		}
	}
}

// The trade filter is the single validator behind the table, the total and the
// CSV export. If it accepted a query one of them could not honour, those three
// would disagree about what "the filtered set" means.
func TestParseTradeFilterValidatesEveryParameter(t *testing.T) {
	cases := []struct {
		query     string
		wantErr   bool
		wantSort  string
		wantSide  string
		wantLimit int
		wantPage  int
	}{
		{"", false, "", "", 50, 1},
		{"?symbol=rawa&sort=amount_desc", false, "amount_desc", "", 50, 1},
		// The symbol is upper-cased once here so the store never compares
		// "rawa" against "RAWA".
		{"?broker=67&side=sell", false, "", "sell", 50, 1},
		{"?limit=200&page=3", false, "", "", 200, 3},
		{"?min_amount=1000&max_amount=5000", false, "", "", 50, 1},
		// A side without a broker has no meaning.
		{"?side=buy", true, "", "", 0, 0},
		{"?side=both&broker=67", true, "", "", 0, 0},
		{"?sort=profit", true, "", "", 0, 0},
		{"?date=yesterday", true, "", "", 0, 0},
		{"?symbol=rawa%20drop", true, "", "", 0, 0},
		{"?broker=67%3Bdrop", true, "", "", 0, 0},
		// Bounds compared as decimals, not as strings: "900" > "1000" is false
		// numerically even though the strings say otherwise.
		{"?min_amount=1000&max_amount=900", true, "", "", 0, 0},
		{"?min_amount=-5", true, "", "", 0, 0},
		{"?limit=0", true, "", "", 0, 0},
		{"?limit=501", true, "", "", 0, 0},
		// Cursor and page are two ways to say where to start; both at once is
		// ambiguous.
		{"?cursor=2026102905019100&page=2", true, "", "", 0, 0},
		{"?cursor=not-a-contract", true, "", "", 0, 0},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/floorsheet"+tc.query, nil)
		rr := httptest.NewRecorder()
		f, ok := parseTradeFilter(rr, req, 50, 500)
		if tc.wantErr {
			if ok {
				t.Errorf("parseTradeFilter(%q) accepted an invalid query", tc.query)
			}
			if rr.Code != http.StatusBadRequest {
				t.Errorf("parseTradeFilter(%q) status = %d, want 400", tc.query, rr.Code)
			}
			continue
		}
		if !ok {
			t.Errorf("parseTradeFilter(%q) rejected a valid query: %s", tc.query, rr.Body.String())
			continue
		}
		if f.Sort != tc.wantSort || f.Side != tc.wantSide || f.Limit != tc.wantLimit {
			t.Errorf("parseTradeFilter(%q) = sort %q side %q limit %d",
				tc.query, f.Sort, f.Side, f.Limit)
		}
		if wantOffset := (tc.wantPage - 1) * tc.wantLimit; f.Offset != wantOffset {
			t.Errorf("parseTradeFilter(%q) offset = %d, want %d", tc.query, f.Offset, wantOffset)
		}
	}
}

// Decimals must be ordered without converting to float64, so a comparison
// cannot pass because two values round the same.
func TestCompareDecimalIsExact(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"900", "1000", -1},
		{"1000", "900", 1},
		{"1000", "1000", 0},
		{"1000.50", "1000.5", 0},
		{"1000.51", "1000.5", 1},
		{"9007199254740993", "9007199254740992", 1},
	}
	for _, tc := range cases {
		if got := compareDecimal(tc.a, tc.b); got != tc.want {
			t.Errorf("compareDecimal(%s, %s) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSessionAgeUsesNepalTime(t *testing.T) {
	// A session dated today in Nepal must be 0 days old even when the server
	// clock is still on the previous UTC day.
	npt := time.FixedZone("NPT", 5*3600+45*60)
	today := time.Now().In(npt).Format("2006-01-02")
	if got := sessionAgeDays(today); got != 0 {
		t.Fatalf("sessionAgeDays(today) = %d, want 0", got)
	}
	if got := sessionAgeDays("2020-01-01"); got <= 4 {
		t.Fatalf("sessionAgeDays(old) = %d, want a stale age", got)
	}
	if got := sessionAgeDays(""); got != 0 {
		t.Fatalf("sessionAgeDays(empty) = %d, want 0", got)
	}
}

func TestMarketMetaDistinguishesFinalFromStale(t *testing.T) {
	s := testServer(t) // nil store: the status lookup is skipped, not fatal
	npt := time.FixedZone("NPT", 5*3600+45*60)
	today := time.Now().In(npt).Format("2006-01-02")

	// A capture from a minute ago while the market is open is live.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/summary", nil)
	meta := s.marketMeta(req, time.Now().Add(-time.Minute), today)
	if meta.Freshness != "live" {
		t.Fatalf("recent capture = %q, want live (%s)", meta.Freshness, meta.FreshnessReason)
	}
	if meta.AsOf == "" || meta.SessionDate != today {
		t.Fatalf("meta lost its timestamp: as_of=%q session=%q", meta.AsOf, meta.SessionDate)
	}

	// Ten minutes old is delayed, an hour old is stale.
	if got := s.marketMeta(req, time.Now().Add(-10*time.Minute), today).Freshness; got != "delayed" {
		t.Errorf("10-minute-old capture = %q, want delayed", got)
	}
	if got := s.marketMeta(req, time.Now().Add(-time.Hour), today).Freshness; got != "stale" {
		t.Errorf("hour-old capture = %q, want stale", got)
	}

	// A session that is days old is stale regardless of when it was ingested:
	// a re-ingest must not make old data look current.
	old := s.marketMeta(req, time.Now(), "2020-01-01")
	if old.Freshness != "stale" {
		t.Fatalf("week-old session = %q, want stale", old.Freshness)
	}
	if old.FreshnessReason == "" {
		t.Error("stale verdict has no reason")
	}
}

func TestDecodeJSONRejectsUnknownFieldsAndArrays(t *testing.T) {
	type payload struct {
		Email string `json:"email"`
	}
	cases := []struct {
		body    string
		wantErr bool
	}{
		{`{"email":"a@b.co"}`, false},
		{`{"email":"a@b.co","admin":true}`, true},
		{`[{"email":"a@b.co"}]`, true},
		{`{"email":"a@b.co"}{"email":"c@d.co"}`, true},
		{`{}`, false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
		var p payload
		err := decodeJSON(req, &p)
		if tc.wantErr && err == nil {
			t.Errorf("decodeJSON(%s) accepted invalid input", tc.body)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("decodeJSON(%s) failed: %v", tc.body, err)
		}
	}
}

// Symbol validation guards every /companies/{symbol} lookup, so a traversal or
// injection attempt is rejected before it reaches SQL.
func TestCleanSymbolRejectsUnsafeInput(t *testing.T) {
	r := chi.NewRouter()
	var got string
	var code int
	r.Get("/{symbol}", func(w http.ResponseWriter, req *http.Request) {
		sym, ok := cleanSymbol(w, req)
		if !ok {
			code = http.StatusBadRequest
			return
		}
		got = sym
		code = http.StatusOK
	})

	for _, bad := range []string{
		"%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"nab;il",
		"NABIL'--",
		"VAULT%20",
		strings.Repeat("A", 40),
	} {
		req := httptest.NewRequest(http.MethodGet, "/"+bad, nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code == http.StatusOK {
			t.Errorf("symbol %q was accepted as %q", bad, got)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/nabil", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if code != http.StatusOK || got != "NABIL" {
		t.Fatalf("cleanSymbol(nabil) = %q, %d; want NABIL, 200", got, code)
	}
}

func TestRateLimitRejectsBurst(t *testing.T) {
	// A budget of one request per minute: the second immediate call must be
	// refused, and the bucket is keyed by client address.
	m := newLimiterMap(1)
	h := rateLimit(m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/summary", nil)
	req.RemoteAddr = "203.0.113.7:5555"

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", rr.Code)
	}
	if env := decode(t, rr); env.Error.Code != CodeRateLimited {
		t.Errorf("error code = %q", env.Error.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("429 response is missing Retry-After")
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	for header, want := range map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "same-site",
	} {
		if got := rr.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestUnknownRouteReturnsJSONEnvelope(t *testing.T) {
	s := testServer(t)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	if env := decode(t, rr); env.Error == nil || env.Error.Code != CodeNotFound {
		t.Fatalf("body is not the documented error envelope: %s", rr.Body.String())
	}
}

func TestLivenessRespondsWithoutDatabase(t *testing.T) {
	// The store is nil here on purpose: liveness must not depend on the database.
	s := testServer(t)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	env := decode(t, rr)
	data, _ := env.Data.(map[string]any)
	if data["status"] != "ok" {
		t.Fatalf("status field = %v", data["status"])
	}
}
