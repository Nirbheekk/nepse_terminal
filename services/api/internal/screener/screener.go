// Package screener turns a validated screening rule into one parameterised SQL
// query over the latest session.
//
// # Why the rule is not free-form SQL
//
// A screener is the one endpoint where a client would most like to write the
// query. It never does. A rule names a metric from a fixed registry, an operator
// from a fixed set, and a number; the SQL is assembled from those allowlisted
// fragments only. Values travel as parameters, never as text. That makes an
// injection attempt a validation error (the metric or operator is unknown)
// rather than something a quoting function has to survive.
//
// # What the metrics actually are
//
// Price, volume, turnover, transactions and market capitalisation come from the
// latest stored session. Float capitalisation and free float are derived from
// the security's own share counts, never guessed. Technical metrics are computed
// over the daily bars stored for that security, which is why every row carries
// `bars`: a 20-day average over three bars is not a 20-day average, and a filter
// that needs history says so through Coverage instead of quietly returning
// nothing.
package screener

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kind classifies a metric so the UI can group filters and the API can report
// per-kind coverage.
type Kind string

const (
	KindPrice       Kind = "price"
	KindVolume      Kind = "volume"
	KindFundamental Kind = "fundamental"
	KindTechnical   Kind = "technical"
)

// Metric is one filterable column: its SQL over the `b` CTE, its kind, and how
// many bars it needs before it means anything.
type Metric struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Kind        Kind   `json:"kind"`
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description,omitempty"`
	// MinBars is the history a value needs to be meaningful; 0 means the value
	// comes from the latest session alone.
	MinBars int `json:"min_bars"`

	expr  string
	nodes bool // true when the expression is nullable for thin history
}

// metrics is the allowlist. A key that is not here cannot appear in a rule.
var metrics = map[string]Metric{
	"close": {Key: "close", Label: "Last price", Kind: KindPrice, Unit: "Rs",
		expr: "b.close"},
	"change_pct": {Key: "change_pct", Label: "Change today", Kind: KindPrice, Unit: "%",
		expr: "b.change_pct"},
	"previous_close": {Key: "previous_close", Label: "Previous close", Kind: KindPrice, Unit: "Rs",
		expr: "b.previous_close"},
	"volume": {Key: "volume", Label: "Volume", Kind: KindVolume, Unit: "shares",
		expr: "b.volume"},
	"turnover": {Key: "turnover", Label: "Turnover", Kind: KindVolume, Unit: "Rs",
		expr: "b.turnover"},
	"transactions": {Key: "transactions", Label: "Transactions", Kind: KindVolume,
		expr: "b.transactions"},

	"market_cap": {Key: "market_cap", Label: "Market capitalisation", Kind: KindFundamental, Unit: "Rs",
		expr: "b.market_cap"},
	"float_market_cap": {Key: "float_market_cap", Label: "Float capitalisation", Kind: KindFundamental, Unit: "Rs",
		expr: "b.float_market_cap", nodes: true},
	"free_float_pct": {Key: "free_float_pct", Label: "Free float", Kind: KindFundamental, Unit: "%",
		expr: "b.free_float_pct", nodes: true},
	"listed_shares": {Key: "listed_shares", Label: "Listed shares", Kind: KindFundamental, Unit: "shares",
		expr: "b.listed_shares", nodes: true},
	"public_shares": {Key: "public_shares", Label: "Public shares", Kind: KindFundamental, Unit: "shares",
		expr: "b.public_shares", nodes: true},

	"sma_20": {Key: "sma_20", Label: "SMA 20", Kind: KindTechnical, Unit: "Rs", MinBars: 20,
		expr: "b.sma_20", nodes: true},
	"sma_50": {Key: "sma_50", Label: "SMA 50", Kind: KindTechnical, Unit: "Rs", MinBars: 50,
		expr: "b.sma_50", nodes: true},
	"return_5d": {Key: "return_5d", Label: "5-session return", Kind: KindTechnical, Unit: "%", MinBars: 6,
		expr: "b.return_5d", nodes: true},
	"return_20d": {Key: "return_20d", Label: "20-session return", Kind: KindTechnical, Unit: "%", MinBars: 21,
		expr: "b.return_20d", nodes: true},
	"high_52w": {Key: "high_52w", Label: "52-week high", Kind: KindTechnical, Unit: "Rs",
		expr: "b.high_52w", nodes: true},
	"low_52w": {Key: "low_52w", Label: "52-week low", Kind: KindTechnical, Unit: "Rs",
		expr: "b.low_52w", nodes: true},
	"pct_from_52w_high": {Key: "pct_from_52w_high", Label: "From 52-week high", Kind: KindTechnical, Unit: "%",
		expr: "b.pct_from_52w_high", nodes: true},
	"volume_ratio_20": {Key: "volume_ratio_20", Label: "Volume vs 20-day average", Kind: KindTechnical, Unit: "x", MinBars: 20,
		expr: "b.volume_ratio_20", nodes: true},
	"bars": {Key: "bars", Label: "Sessions stored", Kind: KindTechnical,
		expr: "b.bars", MinBars: 1},
}

// Metrics returns the registry in a stable order for the API to advertise.
func Metrics() []Metric {
	out := make([]Metric, 0, len(metrics))
	for _, m := range metrics {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// LookupMetric exposes one metric, for validating a saved rule's subject.
func LookupMetric(key string) (Metric, bool) {
	m, ok := metrics[strings.TrimSpace(key)]
	return m, ok
}

// Operators. "between" is inclusive on both ends.
var operators = map[string]string{
	"gt":  ">",
	"gte": ">=",
	"lt":  "<",
	"lte": "<=",
	"eq":  "=",
}

// OperatorKeys returns the accepted operator names in a stable order.
func OperatorKeys() []string {
	keys := []string{"between", "eq", "gt", "gte", "lt", "lte"}
	return keys
}

// Condition is one comparison in a rule. ValueTo is set only for "between".
type Condition struct {
	Metric  string  `json:"metric"`
	Op      string  `json:"op"`
	Value   Number  `json:"value"`
	ValueTo *Number `json:"value_to,omitempty"`
	// MinBars overrides the metric's own history requirement for this
	// condition. Zero means "use the metric default".
	MinBars int `json:"min_bars,omitempty"`
}

// Universe narrows which securities are considered at all.
type Universe struct {
	Sectors     []string `json:"sectors,omitempty"`
	Instruments []string `json:"instruments,omitempty"`
	Statuses    []string `json:"statuses,omitempty"`
	Symbols     []string `json:"symbols,omitempty"`
	Search      string   `json:"search,omitempty"`
}

// Rule is a complete, validated screen.
type Rule struct {
	Universe Universe    `json:"universe"`
	All      []Condition `json:"all,omitempty"`
	Any      []Condition `json:"any,omitempty"`
	Sort     string      `json:"sort,omitempty"`
	Order    string      `json:"order,omitempty"`
	Limit    int         `json:"limit,omitempty"`
	// MinBars gates every technical metric at once; it is the smallest history a
	// caller is willing to accept. Zero means "no gate beyond each metric's own".
	MinBars int `json:"min_bars,omitempty"`
}

// Number is a JSON number kept as text so it never round-trips through float64.
type Number string

// UnmarshalJSON accepts a JSON number or a numeric string.
func (n *Number) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		s = strings.TrimSpace(str)
	}
	if s == "" {
		return errors.New("value must be a number")
	}
	if _, ok := new(big.Rat).SetString(s); !ok {
		return fmt.Errorf("%q is not a number", s)
	}
	*n = Number(s)
	return nil
}

// MarshalJSON writes the number back as a JSON number when it is plain, so the
// stored definition reads like the one the client sent.
func (n Number) MarshalJSON() ([]byte, error) {
	s := string(n)
	if s == "" {
		return []byte("null"), nil
	}
	if _, ok := new(big.Rat).SetString(s); !ok {
		return nil, fmt.Errorf("%q is not a number", s)
	}
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return nil, err
	}
	return []byte(s), nil
}

// ValidationError names the field that failed, so a client can highlight it.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

var (
	sectorRe = regexp.MustCompile(`^[a-z0-9][a-z0-9 _-]{0,48}$`)
	wordRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9 _-]{0,40}$`)
	symbolRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,19}$`)
)

// Validation limits, applied here so a stored rule and an ad-hoc run cannot
// disagree about what is acceptable.
const (
	MaxConditions = 20
	MaxMembers    = 200
	DefaultLimit  = 50
	MaxLimit      = 500
	MaxSearchLen  = 60
)

// Normalize validates a rule and returns a canonical copy: trimmed, upper-cased
// symbols, lower-cased universes, defaulted sort/order/limit.
func Normalize(r Rule) (Rule, error) {
	out := Rule{
		Universe: Universe{
			Search: strings.TrimSpace(r.Universe.Search),
		},
		All:     append([]Condition(nil), r.All...),
		Any:     append([]Condition(nil), r.Any...),
		Sort:    strings.TrimSpace(r.Sort),
		Order:   strings.ToLower(strings.TrimSpace(r.Order)),
		Limit:   r.Limit,
		MinBars: r.MinBars,
	}

	if out.Limit == 0 {
		out.Limit = DefaultLimit
	}
	if out.Limit < 1 || out.Limit > MaxLimit {
		return out, invalid("limit", "must be between 1 and %d", MaxLimit)
	}
	if out.Sort == "" {
		out.Sort = "symbol"
	}
	if out.Order == "" {
		out.Order = "asc"
	}
	if out.Order != "asc" && out.Order != "desc" {
		return out, invalid("order", "must be asc or desc")
	}
	if out.Sort != "symbol" && out.Sort != "name" && out.Sort != "sector" {
		if _, ok := metrics[out.Sort]; !ok {
			return out, invalid("sort", "%q is not a filterable metric", out.Sort)
		}
	}
	if out.MinBars < 0 || out.MinBars > 2520 {
		return out, invalid("min_bars", "must be between 0 and 2520")
	}

	for i, s := range r.Universe.Sectors {
		v := strings.ToLower(strings.TrimSpace(s))
		if !sectorRe.MatchString(v) {
			return out, invalid(fmt.Sprintf("universe.sectors[%d]", i), "%q is not a sector code", s)
		}
		out.Universe.Sectors = append(out.Universe.Sectors, v)
	}
	out.Universe.Sectors = dedupe(out.Universe.Sectors)
	if len(out.Universe.Sectors) > MaxMembers {
		return out, invalid("universe.sectors", "at most %d values", MaxMembers)
	}

	for i, s := range r.Universe.Instruments {
		v := strings.ToLower(strings.TrimSpace(s))
		if !wordRe.MatchString(v) {
			return out, invalid(fmt.Sprintf("universe.instruments[%d]", i), "%q is not an instrument", s)
		}
		out.Universe.Instruments = append(out.Universe.Instruments, v)
	}
	out.Universe.Instruments = dedupe(out.Universe.Instruments)

	for i, s := range r.Universe.Statuses {
		v := strings.ToLower(strings.TrimSpace(s))
		if !wordRe.MatchString(v) {
			return out, invalid(fmt.Sprintf("universe.statuses[%d]", i), "%q is not a status", s)
		}
		out.Universe.Statuses = append(out.Universe.Statuses, v)
	}
	out.Universe.Statuses = dedupe(out.Universe.Statuses)

	for i, s := range r.Universe.Symbols {
		v := strings.ToUpper(strings.TrimSpace(s))
		if !symbolRe.MatchString(v) {
			return out, invalid(fmt.Sprintf("universe.symbols[%d]", i), "%q is not a symbol", s)
		}
		out.Universe.Symbols = append(out.Universe.Symbols, v)
	}
	out.Universe.Symbols = dedupe(out.Universe.Symbols)
	if len(out.Universe.Symbols) > MaxMembers {
		return out, invalid("universe.symbols", "at most %d values", MaxMembers)
	}
	if len(out.Universe.Search) > MaxSearchLen {
		return out, invalid("universe.search", "at most %d characters", MaxSearchLen)
	}

	if len(out.All)+len(out.Any) == 0 {
		// An empty rule is a valid request — "everything, sorted" — but saying so
		// explicitly is kinder than a silent full-table scan in a saved screen.
		out.All = nil
	}
	if len(out.All) > MaxConditions {
		return out, invalid("all", "at most %d conditions", MaxConditions)
	}
	if len(out.Any) > MaxConditions {
		return out, invalid("any", "at most %d conditions", MaxConditions)
	}
	for i := range out.All {
		c, err := normalizeCondition(out.All[i], fmt.Sprintf("all[%d]", i))
		if err != nil {
			return out, err
		}
		out.All[i] = c
	}
	for i := range out.Any {
		c, err := normalizeCondition(out.Any[i], fmt.Sprintf("any[%d]", i))
		if err != nil {
			return out, err
		}
		out.Any[i] = c
	}
	return out, nil
}

func normalizeCondition(c Condition, field string) (Condition, error) {
	key := strings.TrimSpace(c.Metric)
	if _, ok := metrics[key]; !ok {
		return c, invalid(field+".metric", "%q is not a filterable metric", c.Metric)
	}
	op := strings.ToLower(strings.TrimSpace(c.Op))
	if op == "" {
		op = "gte"
	}
	if op != "between" {
		if _, ok := operators[op]; !ok {
			return c, invalid(field+".op", "%q is not one of gt, gte, lt, lte, eq, between", c.Op)
		}
	}
	if strings.TrimSpace(string(c.Value)) == "" && op != "eq" {
		return c, invalid(field+".value", "is required")
	}
	if op == "between" {
		if c.ValueTo == nil {
			return c, invalid(field+".value_to", "is required when op is between")
		}
		lo, okLo := new(big.Rat).SetString(string(c.Value))
		hi, okHi := new(big.Rat).SetString(string(*c.ValueTo))
		if !okLo || !okHi {
			return c, invalid(field+".value", "must be numeric")
		}
		if lo.Cmp(hi) > 0 {
			return c, invalid(field+".value", "must not exceed value_to")
		}
	} else if c.ValueTo != nil {
		return c, invalid(field+".value_to", "is only valid with op=between")
	}
	if c.MinBars < 0 || c.MinBars > 2520 {
		return c, invalid(field+".min_bars", "must be between 0 and 2520")
	}
	out := c
	out.Metric = key
	out.Op = op
	return out, nil
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// Fingerprint is a stable digest of a canonical rule, used to deduplicate saved
// screens and to key alert events.
func Fingerprint(r Rule) string {
	body, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Satisfies reports whether value meets the comparator against threshold, using
// exact rational arithmetic. Alert evaluation uses this so a threshold of
// 415.25 cannot pass because 415.25 rounded to 415.2500001 either way.
func Satisfies(op, value, threshold string) (bool, error) {
	v, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok {
		return false, fmt.Errorf("%q is not a number", value)
	}
	t, ok := new(big.Rat).SetString(strings.TrimSpace(threshold))
	if !ok {
		return false, fmt.Errorf("%q is not a number", threshold)
	}
	switch op {
	case "gt":
		return v.Cmp(t) > 0, nil
	case "gte":
		return v.Cmp(t) >= 0, nil
	case "lt":
		return v.Cmp(t) < 0, nil
	case "lte":
		return v.Cmp(t) <= 0, nil
	case "eq":
		return v.Cmp(t) == 0, nil
	}
	return false, fmt.Errorf("%q is not a comparator", op)
}
