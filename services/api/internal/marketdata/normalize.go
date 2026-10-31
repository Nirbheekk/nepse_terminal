// Package normalize converts values that arrive from the exchange, or from the
// legacy collector's database, into the canonical forms the platform stores.
//
// The rule this package exists to enforce: a value that cannot be parsed is an
// error with a field name, never a zero. A missing price and a price of zero are
// different facts about the market, and conflating them is how a dashboard ends
// up lying.
package marketdata

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Basis says how to interpret a naive timestamp. The legacy collector is not
// consistent: SQLite rows store UTC, while the JSON snapshots the exchange
// returns carry NPT wall-clock times with no offset.
type Basis int

const (
	// BasisUTC means the value is already UTC and needs no shift.
	BasisUTC Basis = iota
	// BasisNPT means the value is Nepal local time (UTC+05:45).
	BasisNPT
)

// NepalOffset is fixed: Nepal has never observed daylight saving.
const NepalOffset = 5*time.Hour + 45*time.Minute

var (
	// Slashes appear in the exchange's own reference data for serial debentures
	// ("GBILD86/87"). They are accepted here so ingestion loses nothing; the
	// URL-safe route has a stricter rule and those symbols are listed as
	// unaddressable in the discrepancy report.
	symbolRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._/-]{0,19}$`)
	digitRe  = regexp.MustCompile(`^[0-9]+$`)
)

// Symbol upper-cases and validates a trading symbol.
func Symbol(raw string) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return "", fmt.Errorf("symbol is empty")
	}
	if !symbolRe.MatchString(s) {
		return "", fmt.Errorf("symbol %q is not a valid ticker", raw)
	}
	return s, nil
}

// Decimal converts a numeric value of unknown Go type into a plain decimal
// string suitable for a NUMERIC column.
//
// Strings are validated and returned with their digits intact (only separators
// are removed), so a value that arrived as text never round-trips through
// float64. Numbers are formatted with the shortest representation that parses
// back to the same float64, which is the best that can be done once the source
// has already collapsed them to binary floating point.
func DecimalString(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", fmt.Errorf("value is null")
	case string:
		s := strings.TrimSpace(strings.ReplaceAll(t, ",", ""))
		if s == "" {
			return "", fmt.Errorf("value is an empty string")
		}
		if f, err := strconv.ParseFloat(s, 64); err != nil {
			return "", fmt.Errorf("value %q is not numeric", t)
		} else if math.IsNaN(f) || math.IsInf(f, 0) {
			// Postgres accepts the literal 'NaN' in a NUMERIC column, where it then
			// propagates through every sum() and ordering. Refuse it at the edge
			// instead of storing a value no aggregate can recover from.
			return "", fmt.Errorf("value %q is not finite", t)
		}
		if strings.ContainsAny(s, "eE") {
			f, _ := strconv.ParseFloat(s, 64)
			return strconv.FormatFloat(f, 'f', -1, 64), nil
		}
		return s, nil
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return "", fmt.Errorf("value is not finite")
		}
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	case float32:
		return DecimalString(float64(t))
	case int:
		return strconv.Itoa(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case json.Number:
		return DecimalString(string(t))
	default:
		return "", fmt.Errorf("value of type %T is not numeric", v)
	}
}

// OptionalDecimal returns nil for a missing or explicitly null value, and an
// error only for a present-but-unparseable one.
func OptionalDecimal(v any) (*string, error) {
	if v == nil {
		return nil, nil
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		return nil, nil
	}
	d, err := DecimalString(v)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// PositiveDecimal is OptionalDecimal with a strict > 0 requirement, used for
// quantities, rates and amounts where zero is not a meaningful observation.
func PositiveDecimal(v any, field string) (string, error) {
	d, err := DecimalString(v)
	if err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	f, err := strconv.ParseFloat(d, 64)
	if err != nil {
		return "", fmt.Errorf("%s: not numeric", field)
	}
	if f <= 0 {
		return "", fmt.Errorf("%s must be positive, got %s", field, d)
	}
	return d, nil
}

// Date extracts a calendar date as YYYY-MM-DD from any of the timestamp shapes
// present in the captured files.
func Date(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("date is not a string (got %T)", v)
	}
	s = strings.TrimSpace(s)
	if len(s) < 10 {
		return "", fmt.Errorf("date %q is too short", s)
	}
	d, err := time.Parse("2006-01-02", s[:10])
	if err != nil {
		return "", fmt.Errorf("date %q is not YYYY-MM-DD", s)
	}
	return d.Format("2006-01-02"), nil
}

// Timestamp parses a timestamp under an explicit time basis and returns it in
// UTC. Passing the wrong basis shifts every row by 5h45m, which is why the basis
// is a required argument rather than a guess inside the parser.
func Timestamp(v any, basis Basis) (*time.Time, error) {
	if v == nil {
		return nil, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("timestamp is not a string (got %T)", v)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	layouts := []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err != nil {
			continue
		}
		// An explicit offset in the input wins over the declared basis.
		if strings.HasSuffix(s, "Z") || hasNumericZone(s) {
			utc := t.UTC()
			return &utc, nil
		}
		wall := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
		if basis == BasisNPT {
			shifted := wall.Add(-NepalOffset)
			return &shifted, nil
		}
		return &wall, nil
	}
	return nil, fmt.Errorf("timestamp %q is not in a recognised format", s)
}

func hasNumericZone(s string) bool {
	// Matches a trailing +05:45 / -08:00 style offset after a time component.
	if len(s) < 6 {
		return false
	}
	tail := s[len(s)-6:]
	return (tail[0] == '+' || tail[0] == '-') && tail[3] == ':'
}

// Text trims a string field, rejecting an empty required value.
func Text(v any, field string) (string, error) {
	s, ok := v.(string)
	if !ok {
		if v == nil {
			return "", fmt.Errorf("%s is null", field)
		}
		return "", fmt.Errorf("%s is not a string (got %T)", field, v)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%s is empty", field)
	}
	return s, nil
}

// OptionalText returns nil for null or blank values.
func OptionalText(v any) *string {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// Integer converts an integral float (SQLite and JSON hand back float64) or a
// numeric string into an int.
func Integer(v any) (int, error) {
	switch t := v.(type) {
	case nil:
		return 0, fmt.Errorf("value is null")
	case int:
		return t, nil
	case int64:
		return int(t), nil
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) || t != math.Trunc(t) {
			return 0, fmt.Errorf("value %v is not an integer", t)
		}
		return int(t), nil
	case string:
		s := strings.TrimSpace(t)
		if !digitRe.MatchString(strings.TrimPrefix(s, "-")) {
			return 0, fmt.Errorf("value %q is not an integer", t)
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, err
		}
		return n, nil
	}
	return 0, fmt.Errorf("value of type %T is not an integer", v)
}
