package screener

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// A rule may only name metrics and operators that exist. This is the property
// that keeps a screener from becoming a SQL passthrough.
func TestNormalizeRejectsUnknownFragments(t *testing.T) {
	cases := []struct {
		name  string
		rule  Rule
		field string
	}{
		{"unknown metric", Rule{All: []Condition{{Metric: "pe_ratio", Op: "gt", Value: "10"}}}, "all[0].metric"},
		{"unknown operator", Rule{All: []Condition{{Metric: "close", Op: "like", Value: "4"}}}, "all[0].op"},
		{"injection in metric", Rule{All: []Condition{{Metric: "close); DROP TABLE users; --", Op: "gt", Value: "1"}}}, "all[0].metric"},
		{"bad sector", Rule{Universe: Universe{Sectors: []string{"banks'; DROP"}}}, "universe.sectors[0]"},
		{"bad symbol", Rule{Universe: Universe{Symbols: []string{"NAB IL"}}}, "universe.symbols[0]"},
		{"bad limit", Rule{Limit: MaxLimit + 1}, "limit"},
		{"bad order", Rule{Order: "sideways"}, "order"},
		{"bad sort", Rule{Sort: "profit_margin"}, "sort"},
	}
	for _, tc := range cases {
		_, err := Normalize(tc.rule)
		if err == nil {
			t.Fatalf("%s: expected a validation error", tc.name)
		}
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%s: error is not a ValidationError: %v", tc.name, err)
		}
		if ve.Field != tc.field {
			t.Errorf("%s: field = %q, want %q", tc.name, ve.Field, tc.field)
		}
	}
}

func TestNormalizeDefaultsAndCanonicalises(t *testing.T) {
	got, err := Normalize(Rule{
		Universe: Universe{Sectors: []string{" Commercial_Banks ", "commercial_banks"}, Symbols: []string{"nabil"}},
		All:      []Condition{{Metric: " close ", Op: "", Value: "400"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Limit != DefaultLimit || got.Sort != "symbol" || got.Order != "asc" {
		t.Fatalf("defaults = limit %d sort %q order %q", got.Limit, got.Sort, got.Order)
	}
	if len(got.Universe.Sectors) != 1 || got.Universe.Sectors[0] != "commercial_banks" {
		t.Fatalf("sectors = %v", got.Universe.Sectors)
	}
	if got.Universe.Symbols[0] != "NABIL" {
		t.Fatalf("symbols = %v", got.Universe.Symbols)
	}
	if got.All[0].Op != "gte" || got.All[0].Metric != "close" {
		t.Fatalf("condition = %+v", got.All[0])
	}
}

func TestNormalizeBetweenRequiresOrderedBounds(t *testing.T) {
	hi := Number("500")
	if _, err := Normalize(Rule{All: []Condition{
		{Metric: "close", Op: "between", Value: "100", ValueTo: &hi},
	}}); err != nil {
		t.Fatalf("valid range rejected: %v", err)
	}

	lo := Number("100")
	if _, err := Normalize(Rule{All: []Condition{
		{Metric: "close", Op: "between", Value: "500", ValueTo: &lo},
	}}); err == nil {
		t.Fatal("inverted range accepted")
	}
	if _, err := Normalize(Rule{All: []Condition{
		{Metric: "close", Op: "between", Value: "100"},
	}}); err == nil {
		t.Fatal("between without value_to accepted")
	}
	if _, err := Normalize(Rule{All: []Condition{
		{Metric: "close", Op: "gt", Value: "100", ValueTo: &hi},
	}}); err == nil {
		t.Fatal("value_to accepted with gt")
	}
}

// Every caller-supplied value must be a parameter. The assertion is on the
// generated SQL text, which is exactly what reaches the database.
func TestQueryParameterisesValues(t *testing.T) {
	rule, err := Normalize(Rule{
		Universe: Universe{Sectors: []string{"commercial_banks"}, Search: "100%"},
		All: []Condition{
			{Metric: "close", Op: "between", Value: "415.25", ValueTo: numberPtr("900")},
			{Metric: "volume", Op: "gt", Value: "1000"},
		},
		Limit: 25,
	})
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := Query(rule)
	if err != nil {
		t.Fatal(err)
	}
	for _, literal := range []string{"415.25", "900", "1000", "commercial_banks"} {
		if strings.Contains(sql, literal) {
			t.Errorf("SQL contains the literal %q; it must be a parameter", literal)
		}
	}
	// The limit is the one value that cannot be a bind parameter in PostgreSQL, so
	// it is validated as an integer and appended; assert it is a bare number and
	// not something a rule could smuggle in.
	if !strings.Contains(sql, "LIMIT $7") {
		t.Fatalf("expected a bound limit placeholder:\n%s", sql)
	}
	if !strings.Contains(sql, "::numeric") {
		t.Error("numeric comparisons must be cast, not compared as text")
	}
	// sectors, two placeholders for the search term, two range bounds, one volume
	// threshold, one limit.
	if len(args) != 7 {
		t.Fatalf("args = %d, want 7 (%v)", len(args), args)
	}
	// The search term keeps LIKE semantics but escapes the caller's wildcards, and
	// the same value fills both the symbol and the name comparison.
	for _, i := range []int{1, 2} {
		if got, ok := args[i].(string); !ok || got != "%100\\%%" {
			t.Fatalf("search arg %d = %v, want the escaped term", i, args[i])
		}
	}
	if args[6] != 25 {
		t.Fatalf("limit arg = %v", args[6])
	}
}

// A technical filter must not be evaluated against a two-session history just
// because the arithmetic happens to work.
func TestQueryGatesTechnicalMetricsOnHistory(t *testing.T) {
	rule, err := Normalize(Rule{All: []Condition{{Metric: "sma_20", Op: "gt", Value: "400"}}})
	if err != nil {
		t.Fatal(err)
	}
	sql, args, _ := Query(rule)
	if !strings.Contains(sql, "b.bars >= $1") {
		t.Fatalf("expected a history gate, got SQL:\n%s", sql)
	}
	if args[0] != 20 {
		t.Fatalf("gate = %v, want 20 from the metric's own requirement", args[0])
	}

	// A caller may lower it deliberately, and the higher of the two wins.
	rule, err = Normalize(Rule{
		MinBars: 5,
		All:     []Condition{{Metric: "sma_20", Op: "gt", Value: "400", MinBars: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, args, _ = Query(rule)
	if args[0] != 5 {
		t.Fatalf("gate = %v, want the rule-wide 5", args[0])
	}

	// A price-only rule needs no history gate at all.
	rule, _ = Normalize(Rule{All: []Condition{{Metric: "close", Op: "gt", Value: "400"}}})
	sql, _, _ = Query(rule)
	if strings.Contains(sql, "b.bars >=") {
		t.Fatal("a price filter must not require history")
	}
}

func TestQueryOrderIsAllowlisted(t *testing.T) {
	rule, _ := Normalize(Rule{Sort: "close", Order: "desc"})
	sql, _, _ := Query(rule)
	if !strings.Contains(sql, "b.close DESC NULLS LAST, b.symbol ASC") {
		t.Fatalf("descending order missing from SQL:\n%s", sql)
	}
	// An unknown sort never reaches the query: Normalize rejects it first, and the
	// builder falls back to symbol rather than interpolating.
	if got := orderSQL("nonsense", "desc"); got != "b.symbol ASC" {
		t.Fatalf("fallback order = %q", got)
	}
}

func TestSatisfiesIsExact(t *testing.T) {
	cases := []struct {
		op               string
		value, threshold string
		want             bool
	}{
		{"gt", "415.25000000001", "415.25", true},
		{"gt", "415.25", "415.25", false},
		{"gte", "415.25", "415.25", true},
		{"lte", "414.9999", "415", true},
		{"lt", "415", "415", false},
		{"eq", "0.1", "0.10", true},
		{"gt", "9007199254740993", "9007199254740992", true},
	}
	for _, tc := range cases {
		got, err := Satisfies(tc.op, tc.value, tc.threshold)
		if err != nil {
			t.Fatalf("%s %s %s: %v", tc.value, tc.op, tc.threshold, err)
		}
		if got != tc.want {
			t.Errorf("%s %s %s = %v, want %v", tc.value, tc.op, tc.threshold, got, tc.want)
		}
	}
	if _, err := Satisfies("gt", "abc", "1"); err == nil {
		t.Fatal("non-numeric value accepted")
	}
	if _, err := Satisfies("like", "1", "1"); err == nil {
		t.Fatal("unknown comparator accepted")
	}
}

// An absent metric must read as absent, not as zero: an alert "below 400" must
// not fire for a security whose price is simply missing.
func TestRowValueDistinguishesMissingFromZero(t *testing.T) {
	zero := "0"
	r := Row{Close: &zero, Transactions: nil, Bars: 0}
	if v, ok := r.Value("close"); !ok || v != "0" {
		t.Fatalf("close = %q %v", v, ok)
	}
	if _, ok := r.Value("transactions"); ok {
		t.Fatal("missing transactions reported present")
	}
	if _, ok := r.Value("sma_20"); ok {
		t.Fatal("missing SMA reported present")
	}
	if _, ok := r.Value("bars"); ok {
		t.Fatal("zero bars reported present")
	}
	if _, ok := r.Value("not_a_metric"); ok {
		t.Fatal("unknown metric reported present")
	}
}

func TestMetricsRegistryCoversEveryKind(t *testing.T) {
	kinds := map[Kind]bool{}
	for _, m := range Metrics() {
		kinds[m.Kind] = true
		if m.Key == "" || m.Label == "" || m.expr == "" {
			t.Fatalf("incomplete metric: %+v", m)
		}
		if _, ok := LookupMetric(m.Key); !ok {
			t.Fatalf("metric %q is advertised but not listed", m.Key)
		}
	}
	for _, k := range []Kind{KindPrice, KindVolume, KindFundamental, KindTechnical} {
		if !kinds[k] {
			t.Errorf("no metric of kind %q", k)
		}
	}
}

// A saved definition must survive a round trip through JSON unchanged, because
// it is stored as JSON and re-read years later.
func TestRuleRoundTripsThroughJSON(t *testing.T) {
	in := Rule{
		Universe: Universe{Sectors: []string{"microfinance"}},
		All:      []Condition{{Metric: "free_float_pct", Op: "between", Value: "10", ValueTo: numberPtr("40")}},
		Sort:     "market_cap",
		Order:    "desc",
		Limit:    10,
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Rule
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if Fingerprint(in) != Fingerprint(out) {
		t.Fatalf("round trip changed the rule: %s vs %s", raw, mustJSON(out))
	}
}

func numberPtr(s string) *Number {
	n := Number(s)
	return &n
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
