//go:build integration

package handlers

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"testing"
	"time"

	"nepse/terminal/api/internal/alerts"
	"nepse/terminal/api/internal/repositories"
)

// newUser returns a client authenticated as a fresh account, plus that account's
// id. Tenant-isolation tests need two of these and must be sure they are separate.
func newUser(t *testing.T) (*client, string) {
	t.Helper()
	c, _ := newStack(t)
	status, env := c.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": uniqueEmail(t), "password": "a-very-long-password", "display_name": "Screener User",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("register = %d, body %+v", status, env.Error)
	}
	status, env = c.do(http.MethodGet, "/api/v1/auth/me", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("/auth/me = %d", status)
	}
	me, _ := env.Data.(map[string]any)
	id, _ := me["id"].(string)
	if id == "" {
		t.Fatal("no user id in /auth/me")
	}
	return c, id
}

// mutate issues a state-changing request with the CSRF header the browser would
// send, so the tests exercise the same path a real client does.
func (c *client) mutate(method, path string, body any) (int, Envelope) {
	c.t.Helper()
	return c.do(method, path, body, map[string]string{"X-CSRF-Token": c.csrf()})
}

func rowsOf(t *testing.T, env Envelope) []map[string]any {
	t.Helper()
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("response data is not an object: %T", env.Data)
	}
	raw, _ := data["rows"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("row is not an object: %T", r)
		}
		out = append(out, m)
	}
	return out
}

func num(t *testing.T, m map[string]any, key string) float64 {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("row has no %q: %v", key, m)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("%q is not a numeric string: %T", key, v)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("%q is not a number: %q", key, s)
	}
	return f
}

// Everything scoped to a tenant must refuse an anonymous caller. A shared screen
// would be worse than useless: it would look personalised and be everyone's.
func TestScreenerSurfaceRequiresAuthentication(t *testing.T) {
	c, _ := newStack(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	anon := &client{t: t, base: c.base, http: &http.Client{Jar: jar, Timeout: 10 * time.Second}}

	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/screener/metrics"},
		{http.MethodPost, "/api/v1/screener/run"},
		{http.MethodGet, "/api/v1/screener/screens"},
		{http.MethodGet, "/api/v1/watchlists"},
		{http.MethodGet, "/api/v1/alerts"},
		{http.MethodGet, "/api/v1/alerts/events"},
		{http.MethodGet, "/api/v1/alerts/deliveries"},
		{http.MethodPost, "/api/v1/alerts/evaluate"},
	}
	for _, tc := range cases {
		var body any
		if tc.method == http.MethodPost {
			body = map[string]any{"rule": map[string]any{}}
		}
		status, env := anon.do(tc.method, tc.path, body, nil)
		if status != http.StatusUnauthorized || env.Error == nil || env.Error.Code != CodeUnauthenticated {
			t.Errorf("%s %s = %d %+v, want 401", tc.method, tc.path, status, env.Error)
		}
	}
}

// Filter correctness is asserted against the data, not against a count: every
// returned row must satisfy the filter, and the derived metrics must match an
// independent recomputation.
func TestScreenerRunFiltersAndDerivedMetrics(t *testing.T) {
	c, _ := newUser(t)

	status, env := c.mutate(http.MethodPost, "/api/v1/screener/run", map[string]any{
		"all":   []map[string]any{{"metric": "close", "op": "gt", "value": 0}},
		"limit": 100,
	})
	if status != http.StatusOK {
		t.Fatalf("run = %d, body %+v", status, env.Error)
	}
	rows := rowsOf(t, env)
	if len(rows) == 0 {
		t.Fatal("no securities matched close > 0; the market data is missing")
	}
	if len(rows) > 100 {
		t.Fatalf("limit ignored: %d rows", len(rows))
	}
	for _, r := range rows {
		if close := num(t, r, "close"); close <= 0 {
			t.Fatalf("%v has close %v but matched close > 0", r["symbol"], close)
		}
		// change_pct must be exactly what the definition says it is.
		closeP, prev := num(t, r, "close"), num(t, r, "previous_close")
		if prev > 0 {
			want := 100 * (closeP - prev) / prev
			got := num(t, r, "change_pct")
			if math.Abs(got-want) > 0.01 {
				t.Fatalf("%v change_pct = %v, recomputed %v", r["symbol"], got, want)
			}
		}
	}

	// Coverage must describe the data that exists, and it must be non-trivial.
	data, _ := env.Data.(map[string]any)
	cov, _ := data["coverage"].(map[string]any)
	securities, _ := cov["securities"].(float64)
	if securities <= 0 {
		t.Fatalf("coverage securities = %v", cov["securities"])
	}
	if _, ok := cov["with_20_sessions"]; !ok {
		t.Fatal("coverage does not report 20-session depth")
	}

	// A range filter excludes everything outside it.
	status, env = c.mutate(http.MethodPost, "/api/v1/screener/run", map[string]any{
		"all": []map[string]any{{
			"metric": "close", "op": "between", "value": 400, "value_to": 500,
		}},
		"limit": 200,
	})
	if status != http.StatusOK {
		t.Fatalf("range run = %d %+v", status, env.Error)
	}
	for _, r := range rowsOf(t, env) {
		if c := num(t, r, "close"); c < 400 || c > 500 {
			t.Fatalf("%v close %v is outside [400,500]", r["symbol"], c)
		}
	}

	// A sector filter returns that sector only, and the sector is the one asked for.
	status, env = c.mutate(http.MethodPost, "/api/v1/screener/run", map[string]any{
		"universe": map[string]any{"sectors": []string{"commercial_banks"}},
		"limit":    50,
	})
	if status != http.StatusOK {
		t.Fatalf("sector run = %d %+v", status, env.Error)
	}
	sectorRows := rowsOf(t, env)
	if len(sectorRows) == 0 {
		t.Fatal("no commercial banks matched; the sector vocabulary is wrong")
	}
	for _, r := range sectorRows {
		if r["sector_code"] != "commercial_banks" {
			t.Fatalf("%v has sector %v", r["symbol"], r["sector_code"])
		}
	}

	// Descending sort must actually descend across the whole page.
	status, env = c.mutate(http.MethodPost, "/api/v1/screener/run", map[string]any{
		"all":   []map[string]any{{"metric": "close", "op": "gt", "value": 0}},
		"sort":  "close",
		"order": "desc",
		"limit": 25,
	})
	if status != http.StatusOK {
		t.Fatalf("sorted run = %d %+v", status, env.Error)
	}
	sorted := rowsOf(t, env)
	for i := 1; i < len(sorted); i++ {
		if num(t, sorted[i-1], "close") < num(t, sorted[i], "close") {
			t.Fatalf("not descending at %d: %v then %v", i,
				sorted[i-1]["close"], sorted[i]["close"])
		}
	}
}

// A technical filter must either answer with history or say it cannot. It must
// never silently treat two sessions as a 20-day average.
func TestScreenerTechnicalFilterRequiresHistory(t *testing.T) {
	c, _ := newUser(t)

	status, env := c.mutate(http.MethodPost, "/api/v1/screener/run", map[string]any{
		"all":   []map[string]any{{"metric": "sma_20", "op": "gt", "value": 0}},
		"limit": 50,
	})
	if status != http.StatusOK {
		t.Fatalf("technical run = %d %+v", status, env.Error)
	}
	data, _ := env.Data.(map[string]any)
	rule, _ := data["rule"].(map[string]any)
	if gate, _ := rule["min_bars"].(float64); gate != 0 {
		t.Fatalf("min_bars is returned as %v", rule["min_bars"])
	}
	for _, r := range rowsOf(t, env) {
		if bars, _ := r["bars"].(float64); bars < 20 {
			t.Fatalf("%v returned for a 20-day average with %v sessions", r["symbol"], bars)
		}
	}
	// The empty case must be explained by coverage rather than by silence.
	cov, _ := data["coverage"].(map[string]any)
	with20, _ := cov["with_20_sessions"].(float64)
	securities, _ := cov["securities"].(float64)
	if with20 > securities {
		t.Fatalf("coverage says %v of %v securities have 20 sessions", with20, securities)
	}
}

func TestScreenerRejectsInvalidRule(t *testing.T) {
	c, _ := newUser(t)

	cases := []struct {
		name string
		rule map[string]any
	}{
		{"unknown metric", map[string]any{"all": []map[string]any{{"metric": "pe_ratio", "op": "gt", "value": 1}}}},
		{"unknown operator", map[string]any{"all": []map[string]any{{"metric": "close", "op": "like", "value": 1}}}},
		{"inverted range", map[string]any{"all": []map[string]any{{"metric": "close", "op": "between", "value": 500, "value_to": 100}}}},
		{"bad sector", map[string]any{"universe": map[string]any{"sectors": []string{"banks', 1); --"}}}},
		{"oversized limit", map[string]any{"limit": 10000}},
	}
	for _, tc := range cases {
		status, env := c.mutate(http.MethodPost, "/api/v1/screener/run", tc.rule)
		if status != http.StatusBadRequest || env.Error == nil || env.Error.Code != CodeValidation {
			t.Errorf("%s: run = %d %+v, want 400 VALIDATION_FAILED", tc.name, status, env.Error)
		}
	}
}

func TestSavedScreensAreTenantScoped(t *testing.T) {
	a, _ := newUser(t)
	b, _ := newUser(t)

	def := map[string]any{
		"universe": map[string]any{"sectors": []string{"commercial_banks"}},
		"all":      []map[string]any{{"metric": "close", "op": "gt", "value": 0}},
		"sort":     "market_cap",
		"order":    "desc",
		"limit":    10,
	}
	status, env := a.mutate(http.MethodPost, "/api/v1/screener/screens", map[string]any{
		"name": "Banks above zero", "description": "divergence probe", "definition": def,
	})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %+v", status, env.Error)
	}
	screen, _ := env.Data.(map[string]any)
	id, _ := screen["id"].(string)
	if id == "" {
		t.Fatal("no id in the created screen")
	}
	if screen["definition"] == nil {
		t.Fatal("stored definition was not returned")
	}

	// The same name is a conflict for its owner, not a second screen.
	status, env = a.mutate(http.MethodPost, "/api/v1/screener/screens", map[string]any{
		"name": "Banks above zero", "definition": def,
	})
	if status != http.StatusConflict || env.Error.Code != CodeConflict {
		t.Fatalf("duplicate name = %d %+v", status, env.Error)
	}

	// Another tenant may use the same name: uniqueness is per owner.
	status, _ = b.mutate(http.MethodPost, "/api/v1/screener/screens", map[string]any{
		"name": "Banks above zero", "definition": def,
	})
	if status != http.StatusCreated {
		t.Fatalf("second tenant could not reuse the name: %d", status)
	}

	// B must not be able to see, run, change or delete A's screen.
	if status, env := b.do(http.MethodGet, "/api/v1/screener/screens/"+id, nil, nil); status != http.StatusNotFound {
		t.Fatalf("cross-tenant read = %d %+v, want 404", status, env.Error)
	}
	if status, _ := b.mutate(http.MethodPost, "/api/v1/screener/screens/"+id+"/run", nil); status != http.StatusNotFound {
		t.Fatalf("cross-tenant run = %d, want 404", status)
	}
	if status, _ := b.mutate(http.MethodPut, "/api/v1/screener/screens/"+id, map[string]any{
		"name": "hijacked", "definition": def,
	}); status != http.StatusNotFound {
		t.Fatalf("cross-tenant update = %d, want 404", status)
	}
	if status, _ := b.mutate(http.MethodDelete, "/api/v1/screener/screens/"+id, nil); status != http.StatusNotFound {
		t.Fatalf("cross-tenant delete = %d, want 404", status)
	}

	// A can run their saved screen and sees only their own in the list.
	status, env = a.mutate(http.MethodPost, "/api/v1/screener/screens/"+id+"/run", nil)
	if status != http.StatusOK {
		t.Fatalf("run saved = %d %+v", status, env.Error)
	}
	if len(rowsOf(t, env)) == 0 {
		t.Fatal("the saved screen returned nothing")
	}
	status, env = a.do(http.MethodGet, "/api/v1/screener/screens", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("list = %d", status)
	}
	list, _ := env.Data.([]any)
	if len(list) != 1 {
		t.Fatalf("A sees %d screens, want 1", len(list))
	}

	// Deleting twice is a 404 the second time.
	if status, _ := a.mutate(http.MethodDelete, "/api/v1/screener/screens/"+id, nil); status != http.StatusOK {
		t.Fatalf("delete = %d", status)
	}
	if status, _ := a.mutate(http.MethodDelete, "/api/v1/screener/screens/"+id, nil); status != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", status)
	}
}

func TestWatchlistsAreTenantScoped(t *testing.T) {
	a, _ := newUser(t)
	b, _ := newUser(t)

	status, env := a.mutate(http.MethodPost, "/api/v1/watchlists", map[string]any{"name": "Banks"})
	if status != http.StatusCreated {
		t.Fatalf("create list = %d %+v", status, env.Error)
	}
	list, _ := env.Data.(map[string]any)
	id, _ := list["id"].(string)

	// Adding a real symbol succeeds and carries the latest quote.
	status, env = a.mutate(http.MethodPost, "/api/v1/watchlists/"+id+"/items",
		map[string]any{"symbol": "nabil", "note": "watch the spread"})
	if status != http.StatusCreated {
		t.Fatalf("add item = %d %+v", status, env.Error)
	}
	status, env = a.do(http.MethodGet, "/api/v1/watchlists/"+id, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("get list = %d", status)
	}
	got, _ := env.Data.(map[string]any)
	items, _ := got["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	item, _ := items[0].(map[string]any)
	if item["symbol"] != "NABIL" {
		t.Fatalf("symbol = %v, want the canonical upper-case NABIL", item["symbol"])
	}
	if item["note"] != "watch the spread" {
		t.Fatalf("note = %v", item["note"])
	}

	// Re-adding updates the note rather than erroring or duplicating.
	status, env = a.mutate(http.MethodPost, "/api/v1/watchlists/"+id+"/items",
		map[string]any{"symbol": "NABIL", "note": "second thought"})
	if status != http.StatusOK {
		t.Fatalf("re-add = %d %+v", status, env.Error)
	}
	if added, _ := env.Data.(map[string]any)["added"].(bool); added {
		t.Fatal("re-add reported a new item")
	}

	// An unknown symbol is refused rather than stored.
	if status, _ := a.mutate(http.MethodPost, "/api/v1/watchlists/"+id+"/items",
		map[string]any{"symbol": "NOTREAL"}); status != http.StatusNotFound {
		t.Fatalf("unknown symbol = %d, want 404", status)
	}

	// B can do nothing with A's list.
	if status, _ := b.do(http.MethodGet, "/api/v1/watchlists/"+id, nil, nil); status != http.StatusNotFound {
		t.Fatalf("cross-tenant read = %d, want 404", status)
	}
	if status, _ := b.mutate(http.MethodPost, "/api/v1/watchlists/"+id+"/items",
		map[string]any{"symbol": "ADBL"}); status != http.StatusNotFound {
		t.Fatalf("cross-tenant add = %d, want 404", status)
	}
	if status, _ := b.mutate(http.MethodDelete, "/api/v1/watchlists/"+id, nil); status != http.StatusNotFound {
		t.Fatalf("cross-tenant delete = %d, want 404", status)
	}

	// Removal is scoped too: B removing from A's list is a 404, not a removal.
	if status, _ := b.mutate(http.MethodDelete, "/api/v1/watchlists/"+id+"/items/NABIL", nil); status != http.StatusNotFound {
		t.Fatalf("cross-tenant remove = %d, want 404", status)
	}
	if status, _ := a.mutate(http.MethodDelete, "/api/v1/watchlists/"+id+"/items/NABIL", nil); status != http.StatusOK {
		t.Fatalf("remove = %d", status)
	}
	if status, _ := a.mutate(http.MethodDelete, "/api/v1/watchlists/"+id+"/items/NABIL", nil); status != http.StatusNotFound {
		t.Fatalf("second remove = %d, want 404", status)
	}
}

// pickSymbol returns a symbol the stored data actually covers, so alert tests do
// not depend on a hard-coded ticker being present.
func pickSymbol(t *testing.T, c *client) string {
	t.Helper()
	status, env := c.mutate(http.MethodPost, "/api/v1/screener/run", map[string]any{
		"all":   []map[string]any{{"metric": "close", "op": "gt", "value": 0}},
		"limit": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("run = %d %+v", status, env.Error)
	}
	rows := rowsOf(t, env)
	if len(rows) == 0 {
		t.Fatal("no symbol with a price")
	}
	sym, _ := rows[0]["symbol"].(string)
	return sym
}

func TestAlertEvaluationDeduplicates(t *testing.T) {
	a, _ := newUser(t)
	b, _ := newUser(t)
	symbol := pickSymbol(t, a)

	status, env := a.mutate(http.MethodPost, "/api/v1/alerts", map[string]any{
		"name": "always true", "symbol": symbol, "metric": "close",
		"comparator": "gt", "threshold": "0", "cooldown_seconds": 3600,
	})
	if status != http.StatusCreated {
		t.Fatalf("create alert = %d %+v", status, env.Error)
	}

	status, env = a.mutate(http.MethodPost, "/api/v1/alerts/evaluate", nil)
	if status != http.StatusOK {
		t.Fatalf("evaluate = %d %+v", status, env.Error)
	}
	first, _ := env.Data.(map[string]any)
	if n, _ := first["new_events"].(float64); n != 1 {
		t.Fatalf("first cycle new_events = %v, want 1 (%+v)", first["new_events"], first)
	}
	if n, _ := first["deliveries"].(float64); n != 1 {
		t.Fatalf("first cycle deliveries = %v, want 1", first["deliveries"])
	}

	// The same session and value cannot produce a second event or a second message.
	status, env = a.mutate(http.MethodPost, "/api/v1/alerts/evaluate", nil)
	if status != http.StatusOK {
		t.Fatalf("second evaluate = %d", status)
	}
	second, _ := env.Data.(map[string]any)
	if n, _ := second["new_events"].(float64); n != 0 {
		t.Fatalf("second cycle created %v new events", second["new_events"])
	}
	if n, _ := second["duplicates"].(float64); n != 1 {
		t.Fatalf("second cycle duplicates = %v, want 1", second["duplicates"])
	}
	if n, _ := second["deliveries"].(float64); n != 0 {
		t.Fatalf("second cycle delivered %v times", second["deliveries"])
	}

	status, env = a.do(http.MethodGet, "/api/v1/alerts/events", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("events = %d", status)
	}
	events, _ := env.Data.([]any)
	if len(events) != 1 {
		t.Fatalf("events = %d, want exactly 1", len(events))
	}
	event, _ := events[0].(map[string]any)
	if event["symbol"] != symbol {
		t.Fatalf("event symbol = %v", event["symbol"])
	}

	status, env = a.do(http.MethodGet, "/api/v1/alerts/deliveries", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("deliveries = %d", status)
	}
	deliveries, _ := env.Data.([]any)
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(deliveries))
	}
	d, _ := deliveries[0].(map[string]any)
	if d["status"] != "delivered" {
		t.Fatalf("delivery status = %v", d["status"])
	}
	if attempts, _ := d["attempts"].(float64); attempts != 1 {
		t.Fatalf("attempts = %v, want 1", d["attempts"])
	}

	// Another tenant sees none of it.
	status, env = b.do(http.MethodGet, "/api/v1/alerts/events", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("B events = %d", status)
	}
	if other, _ := env.Data.([]any); len(other) != 0 {
		t.Fatalf("B sees %d of A's events", len(other))
	}

	// ...and cannot touch A's rule.
	status, env = a.do(http.MethodGet, "/api/v1/alerts", nil, nil)
	rules, _ := env.Data.([]any)
	if len(rules) != 1 {
		t.Fatalf("A rules = %d", len(rules))
	}
	rule, _ := rules[0].(map[string]any)
	ruleID, _ := rule["id"].(string)
	if status, _ := b.mutate(http.MethodDelete, "/api/v1/alerts/"+ruleID, nil); status != http.StatusNotFound {
		t.Fatalf("cross-tenant delete = %d, want 404", status)
	}
	if status, _ := b.mutate(http.MethodPut, "/api/v1/alerts/"+ruleID, map[string]any{
		"name": "hijacked", "symbol": symbol, "metric": "close", "comparator": "lt", "threshold": "1",
	}); status != http.StatusNotFound {
		t.Fatalf("cross-tenant update = %d, want 404", status)
	}
}

func TestAlertRuleValidation(t *testing.T) {
	c, _ := newUser(t)
	symbol := pickSymbol(t, c)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"no subject", map[string]any{"name": "x", "metric": "close", "comparator": "gt", "threshold": "1"}},
		{"unknown metric", map[string]any{"name": "x", "symbol": symbol, "metric": "eps", "comparator": "gt", "threshold": "1"}},
		{"bad comparator", map[string]any{"name": "x", "symbol": symbol, "metric": "close", "comparator": "crosses", "threshold": "1"}},
		{"non numeric threshold", map[string]any{"name": "x", "symbol": symbol, "metric": "close", "comparator": "gt", "threshold": "high"}},
		{"cooldown too large", map[string]any{"name": "x", "symbol": symbol, "metric": "close", "comparator": "gt", "threshold": "1", "cooldown_seconds": 9999999}},
		{"too many attempts", map[string]any{"name": "x", "symbol": symbol, "metric": "close", "comparator": "gt", "threshold": "1", "max_attempts": 50}},
	}
	for _, tc := range cases {
		status, env := c.mutate(http.MethodPost, "/api/v1/alerts", tc.body)
		if status != http.StatusBadRequest {
			t.Errorf("%s: create = %d %+v, want 400", tc.name, status, env.Error)
		}
	}

	// A JSON number is the natural way to write a threshold, and it must be
	// accepted exactly like the string form.
	status, env := c.mutate(http.MethodPost, "/api/v1/alerts", map[string]any{
		"name": "numeric threshold", "symbol": symbol, "metric": "close",
		"comparator": "gt", "threshold": 415.25,
	})
	if status != http.StatusCreated {
		t.Fatalf("numeric threshold = %d %+v", status, env.Error)
	}
	created, _ := env.Data.(map[string]any)
	if created["threshold"] != "415.250000" {
		t.Fatalf("stored threshold = %v, want the exact decimal", created["threshold"])
	}
}

// failingDeliverer lets the retry schedule be tested without a real outage.
type failingDeliverer struct{ calls int }

func (f *failingDeliverer) Deliver(context.Context, alerts.Delivery) error {
	f.calls++
	return errors.New("channel down")
}

// A failed delivery is retried on a backoff and then left failed exactly once its
// allowance is spent — never retried forever, never silently dropped.
func TestAlertDeliveryRetriesThenExhausts(t *testing.T) {
	c, st := newStack(t)
	status, env := c.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": uniqueEmail(t), "password": "a-very-long-password", "display_name": "Retry User",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("register = %d", status)
	}
	status, env = c.do(http.MethodGet, "/api/v1/auth/me", nil, nil)
	me, _ := env.Data.(map[string]any)
	userID, _ := me["id"].(string)

	symbol := pickSymbol(t, c)
	status, env = c.mutate(http.MethodPost, "/api/v1/alerts", map[string]any{
		"name": "retry probe", "symbol": symbol, "metric": "close",
		"comparator": "gt", "threshold": "0", "cooldown_seconds": 0,
		"max_attempts": 2,
	})
	if status != http.StatusCreated {
		t.Fatalf("create alert = %d %+v", status, env.Error)
	}

	t0 := time.Now().UTC()
	failing := &failingDeliverer{}
	engine := &alerts.Engine{
		Store: st, Deliverer: failing,
		Now: func() time.Time { return t0 },
	}
	res, err := engine.EvaluateForUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Deliveries != 1 || res.Failed != 1 {
		t.Fatalf("first cycle = %+v, want 1 delivery and 1 failure", res)
	}

	// Not due yet: the 30s backoff must hold a premature retry back.
	if early, err := st.DueDeliveries(context.Background(), t0.Add(10*time.Second), 100); err != nil {
		t.Fatal(err)
	} else if len(early) != 0 {
		t.Fatalf("%d deliveries were due 10s after the first attempt", len(early))
	}

	due, err := st.DueDeliveries(context.Background(), t0.Add(31*time.Second), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) == 0 {
		t.Fatal("a failed delivery was not rescheduled")
	}

	// After the 30s backoff the retry runs, and spends the last attempt.
	later := &alerts.Engine{
		Store: st, Deliverer: failing,
		Now: func() time.Time { return t0.Add(31 * time.Second) },
	}
	if _, err := later.EvaluateForUser(context.Background(), userID); err != nil {
		t.Fatalf("retry cycle: %v", err)
	}

	status, env = c.do(http.MethodGet, "/api/v1/alerts/deliveries", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("deliveries = %d", status)
	}
	list, _ := env.Data.([]any)
	if len(list) != 1 {
		t.Fatalf("deliveries = %d, want 1 row (a retry must not create a second)", len(list))
	}
	d, _ := list[0].(map[string]any)
	if d["status"] != "failed" {
		t.Fatalf("status = %v, want failed after the allowance is spent", d["status"])
	}
	if attempts, _ := d["attempts"].(float64); attempts != 2 {
		t.Fatalf("attempts = %v, want 2", d["attempts"])
	}
	if msg, _ := d["last_error"].(string); msg == "" {
		t.Fatal("the failure reason was not recorded")
	}
	if next, ok := d["next_attempt_at"]; ok && next != nil {
		t.Fatalf("a terminal failure still has a next attempt: %v", next)
	}
	if failing.calls != 2 {
		t.Fatalf("deliverer called %d times, want 2", failing.calls)
	}

	// A terminal failure is not retried again, however many cycles run.
	again := &alerts.Engine{
		Store: st, Deliverer: failing,
		Now: func() time.Time { return t0.Add(24 * time.Hour) },
	}
	if _, err := again.EvaluateForUser(context.Background(), userID); err != nil {
		t.Fatal(err)
	}
	if failing.calls != 2 {
		t.Fatalf("a failed delivery was retried after exhaustion (%d calls)", failing.calls)
	}
}

// Deleting a rule must take its history with it: leaving orphaned deliveries
// would keep sending someone else's alerts.
func TestDeletingAlertRuleRemovesHistory(t *testing.T) {
	c, _ := newUser(t)
	symbol := pickSymbol(t, c)

	status, env := c.mutate(http.MethodPost, "/api/v1/alerts", map[string]any{
		"name": "delete probe", "symbol": symbol, "metric": "close",
		"comparator": "gt", "threshold": "0",
	})
	if status != http.StatusCreated {
		t.Fatalf("create = %d", status)
	}
	rule, _ := env.Data.(map[string]any)
	id, _ := rule["id"].(string)

	if status, _ := c.mutate(http.MethodPost, "/api/v1/alerts/evaluate", nil); status != http.StatusOK {
		t.Fatalf("evaluate = %d", status)
	}
	if status, env := c.do(http.MethodGet, "/api/v1/alerts/deliveries", nil, nil); status != http.StatusOK {
		t.Fatalf("deliveries = %d", status)
	} else if list, _ := env.Data.([]any); len(list) == 0 {
		t.Fatal("nothing was delivered to delete")
	}

	if status, _ := c.mutate(http.MethodDelete, "/api/v1/alerts/"+id, nil); status != http.StatusOK {
		t.Fatalf("delete = %d", status)
	}
	status, env = c.do(http.MethodGet, "/api/v1/alerts/deliveries", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("deliveries after delete = %d", status)
	}
	if list, _ := env.Data.([]any); len(list) != 0 {
		t.Fatalf("%d deliveries survived their rule", len(list))
	}
}

var _ = repositories.ErrNotFound
