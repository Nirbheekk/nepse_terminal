//go:build integration

// Integration tests exercise the real router against a real PostgreSQL. They are
// behind the `integration` build tag because they need a database:
//
//	TEST_DATABASE_URL=postgres://nepse:nepse_local_dev@localhost:5433/nepse?sslmode=disable \
//	  go test ./internal/httpapi -tags=integration -count=1 -v
//
// The schema is created by the same migration runner the binary uses, so these
// tests fail if a migration is broken.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"nepse/terminal/api/internal/repositories"
)

func dbURL(t *testing.T) string {
	t.Helper()
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	t.Skip("set TEST_DATABASE_URL to run integration tests")
	return ""
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newStack(t *testing.T) (*client, *repositories.Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := repositories.Open(ctx, dbURL(t))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(st.Close)

	status, err := st.Migrate(ctx)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if status.Applied == 0 {
		t.Fatal("no migrations applied; schema is not under test")
	}

	cfg := testConfig()
	cfg.SeedOnStart = false
	srv, err := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)), status)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &client{
		t:    t,
		base: ts.URL,
		http: &http.Client{Jar: jar, Timeout: 10 * time.Second},
	}, st
}

// do issues a request and returns the status and decoded envelope.
func (c *client) do(method, path string, body any, headers map[string]string) (int, Envelope) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	var env Envelope
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &env)
	return resp.StatusCode, env
}

// authURL is the path the refresh cookie is scoped to. A cookie jar only
// returns cookies whose path matches the requested URL, which is exactly the
// browser behaviour we want to assert against.
func (c *client) authURL() *url.URL {
	u, _ := url.Parse(c.base + "/api/v1/auth/refresh")
	return u
}

func (c *client) csrf() string {
	c.t.Helper()
	u, _ := url.Parse(c.base)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == csrfCookie {
			return ck.Value
		}
	}
	return ""
}

func (c *client) cookieNames() map[string]bool {
	out := map[string]bool{}
	for _, ck := range c.http.Jar.Cookies(c.authURL()) {
		out[ck.Name] = true
	}
	return out
}

func uniqueEmail(t *testing.T) string {
	return fmt.Sprintf("%s-%d@example.test", t.Name(), time.Now().UnixNano())
}

func TestHealthAndReadiness(t *testing.T) {
	c, _ := newStack(t)

	status, env := c.do(http.MethodGet, "/healthz", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("/healthz = %d", status)
	}
	if env.Meta.RequestID == "" {
		t.Error("liveness response has no request id")
	}

	status, env = c.do(http.MethodGet, "/readyz", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("/readyz = %d, body %+v", status, env)
	}
	data, _ := env.Data.(map[string]any)
	if data["database"] != "reachable" {
		t.Errorf("readiness database = %v", data["database"])
	}
	migrations, _ := data["migrations"].(map[string]any)
	if migrations["latest"] == "" {
		t.Error("readiness does not report the applied schema version")
	}
}

func TestAuthLifecycle(t *testing.T) {
	c, st := newStack(t)
	email := uniqueEmail(t)
	password := "a-very-long-password"

	// --- register
	status, env := c.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": email, "password": password, "display_name": "Integration User",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("register = %d, body %+v", status, env.Error)
	}
	user, _ := env.Data.(map[string]any)
	if user["email"] != email {
		t.Fatalf("registered email = %v", user["email"])
	}

	// Tokens must live in HttpOnly cookies, never in the body.
	if _, ok := user["access_token"]; ok {
		t.Fatal("access token was returned in the response body")
	}
	cookies := c.cookieNames()
	for _, name := range []string{accessCookie, refreshCookie, csrfCookie} {
		if !cookies[name] {
			t.Errorf("cookie %q was not set", name)
		}
	}
	if c.csrf() == "" {
		t.Fatal("csrf cookie missing")
	}

	// --- duplicate registration is a 409, not a second account
	status, env = c.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": email, "password": password, "display_name": "Dup",
	}, nil)
	if status != http.StatusConflict || env.Error.Code != CodeConflict {
		t.Fatalf("duplicate register = %d %+v", status, env.Error)
	}

	// --- me
	status, env = c.do(http.MethodGet, "/api/v1/auth/me", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("/auth/me = %d", status)
	}
	me, _ := env.Data.(map[string]any)
	if me["email"] != email {
		t.Fatalf("me.email = %v", me["email"])
	}

	// The password hash must never be serialised.
	raw, _ := json.Marshal(me)
	if bytes.Contains(bytes.ToLower(raw), []byte("argon2")) {
		t.Fatal("password hash leaked into the API response")
	}

	// --- persistence
	if n, err := st.CountUsers(context.Background()); err != nil || n == 0 {
		t.Fatalf("CountUsers = %d, %v", n, err)
	}

	// --- logout clears the session
	status, _ = c.do(http.MethodPost, "/api/v1/auth/logout", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("logout = %d", status)
	}
	status, env = c.do(http.MethodGet, "/api/v1/auth/me", nil, nil)
	if status != http.StatusUnauthorized || env.Error.Code != CodeUnauthenticated {
		t.Fatalf("me after logout = %d %+v", status, env.Error)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	c, _ := newStack(t)
	email := uniqueEmail(t)
	password := "a-very-long-password"

	if status, _ := c.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": email, "password": password, "display_name": "Login User",
	}, nil); status != http.StatusCreated {
		t.Fatalf("register = %d", status)
	}

	anon, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	fresh := &client{t: t, base: c.base, http: &http.Client{Jar: anon, Timeout: 10 * time.Second}}

	status, env := fresh.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": email, "password": "wrong-password-here",
	}, nil)
	if status != http.StatusUnauthorized || env.Error.Code != CodeUnauthenticated {
		t.Fatalf("bad password login = %d %+v", status, env.Error)
	}
	// The message must not reveal whether the account exists.
	if env.Error.Message != "invalid email or password" {
		t.Fatalf("error message leaks account state: %q", env.Error.Message)
	}

	status, _ = fresh.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": uniqueEmail(t), "password": "wrong-password-here",
	}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unknown account login = %d", status)
	}

	status, _ = fresh.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": email, "password": password,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("valid login = %d", status)
	}
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	c, _ := newStack(t)
	if status, _ := c.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": uniqueEmail(t), "password": "a-very-long-password", "display_name": "Rotator",
	}, nil); status != http.StatusCreated {
		t.Fatal("register failed")
	}

	var original string
	for _, ck := range c.http.Jar.Cookies(c.authURL()) {
		if ck.Name == refreshCookie {
			original = ck.Value
		}
	}
	if original == "" {
		t.Fatal("no refresh cookie issued")
	}

	status, _ := c.do(http.MethodPost, "/api/v1/auth/refresh", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("refresh = %d", status)
	}

	var rotated string
	for _, ck := range c.http.Jar.Cookies(c.authURL()) {
		if ck.Name == refreshCookie {
			rotated = ck.Value
		}
	}
	if rotated == original {
		t.Fatal("refresh did not rotate the refresh token")
	}

	// Present the superseded token again: the whole family must be revoked.
	replay := &client{t: t, base: c.base}
	replay.http = &http.Client{
		Jar:     mustJarWith(t, refreshCookie, original),
		Timeout: 10 * time.Second,
	}
	status, _ = replay.do(http.MethodPost, "/api/v1/auth/refresh", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("replayed refresh token = %d, want 401", status)
	}

	// The legitimate successor must now be dead too.
	status, _ = c.do(http.MethodPost, "/api/v1/auth/refresh", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("successor token still valid after reuse detection: %d", status)
	}
}

func mustJarWith(t *testing.T, name, value string) http.CookieJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &staticJar{jar: jar, name: name, value: value}
}

// staticJar adds one fixed cookie to every request.
type staticJar struct {
	jar   http.CookieJar
	name  string
	value string
}

func (s *staticJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	s.jar.SetCookies(u, cookies)
}

func (s *staticJar) Cookies(u *url.URL) []*http.Cookie {
	out := s.jar.Cookies(u)
	found := false
	for _, c := range out {
		if c.Name == s.name {
			c.Value = s.value
			found = true
		}
	}
	if !found {
		out = append(out, &http.Cookie{Name: s.name, Value: s.value, Path: "/api/v1/auth"})
	}
	return out
}

func TestCSRFBlocksCookieAuthenticatedMutation(t *testing.T) {
	c, _ := newStack(t)
	if status, _ := c.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": uniqueEmail(t), "password": "a-very-long-password", "display_name": "CSRF User",
	}, nil); status != http.StatusCreated {
		t.Fatal("register failed")
	}

	// List sessions to get a real id.
	status, env := c.do(http.MethodGet, "/api/v1/auth/sessions", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("sessions = %d", status)
	}
	list, _ := env.Data.([]any)
	if len(list) == 0 {
		t.Fatal("no sessions listed for a freshly registered user")
	}
	first, _ := list[0].(map[string]any)
	id, _ := first["id"].(string)

	status, env = c.do(http.MethodDelete, "/api/v1/auth/sessions/"+id, nil, nil)
	if status != http.StatusForbidden || env.Error.Code != CodeCSRF {
		t.Fatalf("delete without CSRF = %d %+v, want 403 CSRF", status, env.Error)
	}

	status, _ = c.do(http.MethodDelete, "/api/v1/auth/sessions/"+id, nil,
		map[string]string{"X-CSRF-Token": c.csrf()})
	if status != http.StatusOK {
		t.Fatalf("delete with CSRF = %d, want 200", status)
	}
}

func TestAccountsAreIsolated(t *testing.T) {
	a, _ := newStack(t)
	b := &client{t: t, base: a.base}
	jar, _ := cookiejar.New(nil)
	b.http = &http.Client{Jar: jar, Timeout: 10 * time.Second}

	for _, c := range []*client{a, b} {
		if status, _ := c.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
			"email": uniqueEmail(t), "password": "a-very-long-password", "display_name": "Isolated",
		}, nil); status != http.StatusCreated {
			t.Fatal("register failed")
		}
	}

	_, envA := a.do(http.MethodGet, "/api/v1/auth/me", nil, nil)
	_, envB := b.do(http.MethodGet, "/api/v1/auth/me", nil, nil)
	meA, _ := envA.Data.(map[string]any)
	meB, _ := envB.Data.(map[string]any)

	if meA["id"] == meB["id"] {
		t.Fatal("two registrations produced the same user id")
	}

	_, envSessions := a.do(http.MethodGet, "/api/v1/auth/sessions", nil, nil)
	listA, _ := envSessions.Data.([]any)
	if len(listA) != 1 {
		t.Fatalf("account A lists %d sessions, want only its own", len(listA))
	}

	// B must not be able to revoke A's session.
	first, _ := listA[0].(map[string]any)
	idA, _ := first["id"].(string)
	status, _ := b.do(http.MethodDelete, "/api/v1/auth/sessions/"+idA, nil,
		map[string]string{"X-CSRF-Token": b.csrf()})
	if status != http.StatusOK {
		t.Fatalf("cross-account revoke returned %d", status)
	}
	// It reports success (no existence oracle) but must not have revoked A.
	_, envSessions = a.do(http.MethodGet, "/api/v1/auth/sessions", nil, nil)
	listA, _ = envSessions.Data.([]any)
	if len(listA) != 1 {
		t.Fatal("account B revoked a session belonging to account A")
	}
}

// TestMarketSurface walks the endpoints the terminal's three market screens use:
// the overview payload, exchange status, sector filter menu, the filtered and
// sorted company list, company detail and both CSV exports.
func TestMarketSurface(t *testing.T) {
	c, _ := newStack(t)

	status, env := c.do(http.MethodGet, "/api/v1/market/overview", nil, nil)
	switch status {
	case http.StatusOK:
		ov, _ := env.Data.(map[string]any)
		for _, key := range []string{"summary", "movers", "indices", "generated_at"} {
			if _, ok := ov[key]; !ok {
				t.Errorf("overview is missing %q", key)
			}
		}
		if sum, ok := ov["summary"].(map[string]any); ok {
			if sum["trading_date"] == nil || sum["trading_date"] == "" {
				t.Error("overview summary has no trading date")
			}
		}
		if env.Meta.Freshness == "" {
			t.Error("overview carries no freshness verdict")
		}
		if env.Meta.FreshnessReason == "" {
			t.Error("overview carries no freshness reason")
		}
	case http.StatusNotFound:
		if env.Error == nil || env.Error.Code != CodeNotFound {
			t.Fatalf("empty overview error = %+v", env.Error)
		}
	default:
		t.Fatalf("overview = %d", status)
	}

	status, env = c.do(http.MethodGet, "/api/v1/market/status", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("market status = %d", status)
	}
	ms, _ := env.Data.(map[string]any)
	switch ms["state"] {
	case "open", "preopen", "closed", "unknown":
	default:
		t.Errorf("market status state = %v", ms["state"])
	}
	if _, ok := ms["is_open"]; !ok {
		t.Error("market status is missing is_open")
	}

	status, env = c.do(http.MethodGet, "/api/v1/sectors", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("sectors = %d", status)
	}
	if _, ok := env.Data.([]any); !ok {
		t.Errorf("sectors payload is %T, want an array", env.Data)
	}

	// Sorted list: market capitalisation descending, with the total preserved.
	status, env = c.do(http.MethodGet, "/api/v1/companies?sort=market_cap&order=desc&limit=5", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("sorted companies = %d", status)
	}
	if env.Meta.Total == nil {
		t.Fatal("companies response has no total")
	}
	rows, _ := env.Data.([]any)
	if len(rows) > 1 {
		prev := -1.0
		for _, raw := range rows {
			m, _ := raw.(map[string]any)
			capStr, _ := m["market_cap"].(string)
			cap := 0.0
			if capStr != "" {
				fmt.Sscanf(capStr, "%f", &cap)
			}
			if cap > prev && prev >= 0 {
				t.Errorf("market_cap order broken: %f after %f", cap, prev)
			}
			prev = cap
		}
	}

	// A filtered page must report the filtered total, not the whole directory.
	status, env = c.do(http.MethodGet, "/api/v1/sectors", nil, nil)
	if status == http.StatusOK {
		if list, ok := env.Data.([]any); ok && len(list) > 0 {
			first, _ := list[0].(map[string]any)
			code, _ := first["code"].(string)
			count, _ := first["companies"].(float64)
			status, env = c.do(http.MethodGet, "/api/v1/companies?sector="+url.QueryEscape(code)+"&limit=200", nil, nil)
			if status != http.StatusOK {
				t.Fatalf("sector filter = %d", status)
			}
			if env.Meta.Total == nil || *env.Meta.Total != int(count) {
				t.Errorf("sector total = %v, want %v", env.Meta.Total, count)
			}
		}
	}

	for _, bad := range []string{
		"/api/v1/companies?sort=profit",
		"/api/v1/companies?order=up",
		"/api/v1/companies?sector=Not A Sector",
		"/api/v1/companies?limit=0",
	} {
		if status, _ := c.do(http.MethodGet, bad, nil, nil); status != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, status)
		}
	}

	// CSV export: real CSV with the provenance header, not an HTML error page.
	resp, err := c.http.Get(c.base + "/api/v1/companies/export.csv?sort=market_cap&order=desc&limit=5")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("companies CSV = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("CSV content type = %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("CSV content disposition = %q", cd)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{"# generated_at", "# session_date", "# licence", "market_cap_method", "float_market_cap"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("CSV is missing %q", want)
		}
	}

	// Company detail and its price-history export, using a real symbol.
	status, env = c.do(http.MethodGet, "/api/v1/companies?limit=1", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("company list = %d", status)
	}
	if list, ok := env.Data.([]any); ok && len(list) > 0 {
		first, _ := list[0].(map[string]any)
		sym, _ := first["symbol"].(string)

		status, env = c.do(http.MethodGet, "/api/v1/companies/"+sym, nil, nil)
		if status != http.StatusOK {
			t.Fatalf("company %s = %d", sym, status)
		}
		detail, _ := env.Data.(map[string]any)
		// trading_date and transactions are omitted for a listed security that
		// never traded, so only the always-present method fields are required.
		for _, key := range []string{"market_cap_method", "float_market_cap_method", "status", "instrument"} {
			if _, ok := detail[key]; !ok {
				t.Errorf("company detail is missing %q", key)
			}
		}
		if env.Meta.Freshness == "" {
			t.Error("company detail carries no freshness verdict")
		}

		resp, err := c.http.Get(c.base + "/api/v1/companies/" + sym + "/prices.csv")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("prices CSV = %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte("trading_date,open,high,low,close")) {
			t.Error("prices CSV has no header row")
		}
	}
}

func TestMarketEndpointsShape(t *testing.T) {
	c, _ := newStack(t)

	// /health always answers and reports real table counts.
	status, env := c.do(http.MethodGet, "/api/v1/health", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("/api/v1/health = %d", status)
	}
	h, _ := env.Data.(map[string]any)
	for _, key := range []string{"companies", "securities", "sessions", "trades", "bars"} {
		if _, ok := h[key]; !ok {
			t.Errorf("health payload is missing %q", key)
		}
	}

	// Summary is either data or an explicit absence — never a fake zero row.
	status, env = c.do(http.MethodGet, "/api/v1/market/summary", nil, nil)
	switch status {
	case http.StatusOK:
		sum, _ := env.Data.(map[string]any)
		if sum["trading_date"] == "" || sum["trading_date"] == nil {
			t.Error("summary is missing trading_date")
		}
		if env.Meta.Source == "" {
			t.Error("summary is missing provenance metadata")
		}
	case http.StatusNotFound:
		if env.Error.Code != CodeNotFound {
			t.Errorf("missing summary error code = %q", env.Error.Code)
		}
	default:
		t.Fatalf("summary status = %d", status)
	}

	// Input validation is enforced before the database is touched.
	status, env = c.do(http.MethodGet, "/api/v1/floorsheet?limit=99999", nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("out-of-range limit = %d, want 400", status)
	}
	status, _ = c.do(http.MethodGet, "/api/v1/floorsheet?date=yesterday", nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("malformed date = %d, want 400", status)
	}
	status, _ = c.do(http.MethodGet, "/api/v1/market/top-movers?type=hacked", nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("unknown mover type = %d, want 400", status)
	}
	status, _ = c.do(http.MethodGet, "/api/v1/companies/NOT%20A%20SYMBOL", nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("malformed symbol = %d, want 400", status)
	}
}
