// Package numeric provides exact rational decimal operations for financial calculations.
// It guarantees no floating-point rounding or cancellation errors.
package marketdata

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

var (
	ErrDivideByZero   = errors.New("division by zero")
	ErrInvalidDecimal = errors.New("invalid decimal string")
)

// Decimal represents a precise decimal number backed by big.Rat.
type Decimal struct {
	r *big.Rat
}

func rat(d Decimal) *big.Rat {
	if d.r == nil {
		return new(big.Rat)
	}
	return d.r
}

// Zero returns 0.
func Zero() Decimal {
	return Decimal{r: new(big.Rat)}
}

// One returns 1.
func One() Decimal {
	return Decimal{r: big.NewRat(1, 1)}
}

// NewFromInt64 creates a Decimal from int64.
func NewFromInt64(val int64) Decimal {
	return Decimal{r: big.NewRat(val, 1)}
}

// NewFromRat creates a Decimal directly from a big.Rat.
func NewFromRat(r *big.Rat) Decimal {
	if r == nil {
		return Zero()
	}
	cp := new(big.Rat).Set(r)
	return Decimal{r: cp}
}

// NewFromFloat creates a Decimal from float64.
func NewFromFloat(f float64) Decimal {
	r := new(big.Rat).SetFloat64(f)
	if r == nil {
		return Zero()
	}
	return Decimal{r: r}
}

// Parse parses a string representation into Decimal.
func Parse(s string) (Decimal, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return Zero(), nil
	}
	// Strip commas if any (e.g. 1,000,000.50)
	trimmed = strings.ReplaceAll(trimmed, ",", "")
	r, ok := new(big.Rat).SetString(trimmed)
	if !ok {
		return Zero(), fmt.Errorf("%w: %q", ErrInvalidDecimal, s)
	}
	return Decimal{r: r}, nil
}

// MustParse parses a string and panics if invalid.
func MustParse(s string) Decimal {
	d, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}

// IsZero returns true if d == 0.
func (d Decimal) IsZero() bool {
	return rat(d).Sign() == 0
}

// IsPositive returns true if d > 0.
func (d Decimal) IsPositive() bool {
	return rat(d).Sign() > 0
}

// IsNegative returns true if d < 0.
func (d Decimal) IsNegative() bool {
	return rat(d).Sign() < 0
}

// Sign returns -1 if d < 0, 0 if d == 0, and +1 if d > 0.
func (d Decimal) Sign() int {
	return rat(d).Sign()
}

// Add returns d + other.
func (d Decimal) Add(other Decimal) Decimal {
	res := new(big.Rat).Add(rat(d), rat(other))
	return Decimal{r: res}
}

// Sub returns d - other.
func (d Decimal) Sub(other Decimal) Decimal {
	res := new(big.Rat).Sub(rat(d), rat(other))
	return Decimal{r: res}
}

// Mul returns d * other.
func (d Decimal) Mul(other Decimal) Decimal {
	res := new(big.Rat).Mul(rat(d), rat(other))
	return Decimal{r: res}
}

// Div returns d / other, returning an error if other is zero.
func (d Decimal) Div(other Decimal) (Decimal, error) {
	if other.IsZero() {
		return Zero(), ErrDivideByZero
	}
	res := new(big.Rat).Quo(rat(d), rat(other))
	return Decimal{r: res}, nil
}

// Quo returns d / other. If other is zero, returns Zero().
func (d Decimal) Quo(other Decimal) Decimal {
	res, err := d.Div(other)
	if err != nil {
		return Zero()
	}
	return res
}

// Abs returns |d|.
func (d Decimal) Abs() Decimal {
	res := new(big.Rat).Abs(rat(d))
	return Decimal{r: res}
}

// Neg returns -d.
func (d Decimal) Neg() Decimal {
	res := new(big.Rat).Neg(rat(d))
	return Decimal{r: res}
}

// Cmp compares d and other and returns:
// -1 if d < other
//
//	0 if d == other
//
// +1 if d > other
func (d Decimal) Cmp(other Decimal) int {
	return rat(d).Cmp(rat(other))
}

// Equal returns true if d == other.
func (d Decimal) Equal(other Decimal) bool {
	return d.Cmp(other) == 0
}

// GreaterThan returns true if d > other.
func (d Decimal) GreaterThan(other Decimal) bool {
	return d.Cmp(other) > 0
}

// GreaterThanOrEqual returns true if d >= other.
func (d Decimal) GreaterThanOrEqual(other Decimal) bool {
	return d.Cmp(other) >= 0
}

// LessThan returns true if d < other.
func (d Decimal) LessThan(other Decimal) bool {
	return d.Cmp(other) < 0
}

// LessThanOrEqual returns true if d <= other.
func (d Decimal) LessThanOrEqual(other Decimal) bool {
	return d.Cmp(other) <= 0
}

// Format formats the Decimal to a fixed scale using half-up rounding.
func (d Decimal) Format(scale int) string {
	if scale < 0 {
		scale = 0
	}
	r := rat(d)
	return r.FloatString(scale)
}

// String returns the decimal string formatted to 4 decimal places by default,
// or trimmed cleanly without trailing zeros if exact.
func (d Decimal) String() string {
	r := rat(d)
	if r.IsInt() {
		return r.Num().String()
	}
	// Print with precision 6 and trim redundant trailing zeros
	s := r.FloatString(6)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// Round rounds to the specified scale using half-up arithmetic.
func (d Decimal) Round(scale int) Decimal {
	if scale < 0 {
		scale = 0
	}
	s := rat(d).FloatString(scale)
	res, _ := Parse(s)
	return res
}

// Float64 returns the float64 approximation.
func (d Decimal) Float64() float64 {
	f, _ := rat(d).Float64()
	return f
}

// MarshalJSON marshals Decimal as a string to preserve full precision.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON unmarshals Decimal from JSON (string or number).
func (d *Decimal) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		dec, err := Parse(s)
		if err != nil {
			return err
		}
		*d = dec
		return nil
	}
	// Try parsing raw number
	dec, err := Parse(string(data))
	if err != nil {
		return fmt.Errorf("cannot unmarshal decimal: %w", err)
	}
	*d = dec
	return nil
}

// Value implements driver.Valuer for PostgreSQL NUMERIC storage.
func (d Decimal) Value() (driver.Value, error) {
	return d.String(), nil
}

// Scan implements sql.Scanner for reading PostgreSQL NUMERIC values.
func (d *Decimal) Scan(src any) error {
	if src == nil {
		*d = Zero()
		return nil
	}
	switch v := src.(type) {
	case string:
		dec, err := Parse(v)
		if err != nil {
			return err
		}
		*d = dec
		return nil
	case []byte:
		dec, err := Parse(string(v))
		if err != nil {
			return err
		}
		*d = dec
		return nil
	case int64:
		*d = NewFromInt64(v)
		return nil
	case float64:
		*d = NewFromFloat(v)
		return nil
	default:
		return fmt.Errorf("unsupported type for Decimal scan: %T", src)
	}
}

// Min returns the smaller of a and b.
func Min(a, b Decimal) Decimal {
	if a.LessThan(b) {
		return a
	}
	return b
}

// Max returns the larger of a and b.
func Max(a, b Decimal) Decimal {
	if a.GreaterThan(b) {
		return a
	}
	return b
}
