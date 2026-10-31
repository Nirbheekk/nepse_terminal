// Package ident handles security identifiers.
//
// A trading symbol is not a stable identity: exchanges reuse symbols after a
// delisting, and the same company can have several securities. ISIN is the
// identifier that survives those events, so the platform records it alongside
// the symbol and validates its check digit rather than trusting whatever the
// source sent.
package marketdata

import (
	"strings"
	"unicode"
)

// ISIN is the canonical form of an International Securities Identification
// Number along with the result of its checksum test.
type ISIN struct {
	Value       string
	CheckDigit  bool
	CountryCode string
}

// NormalizeISIN upper-cases and strips separators, then verifies the structure
// and the Luhn check digit.
//
// A value that looks like an ISIN but fails the checksum is returned with
// CheckDigit false, not dropped: the caller can still store it for later
// research, while the failure is visible in the data instead of silent.
func NormalizeISIN(raw string) (ISIN, bool) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.NewReplacer(" ", "", "-", "", ".", "").Replace(s)
	if len(s) != 12 {
		return ISIN{}, false
	}
	for i, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return ISIN{}, false
		}
		if i < 2 && !unicode.IsLetter(r) {
			return ISIN{}, false
		}
	}
	if unicode.IsLetter(rune(s[0])) == false || unicode.IsLetter(rune(s[1])) == false {
		return ISIN{}, false
	}
	return ISIN{
		Value:       s,
		CheckDigit:  luhnValid(s),
		CountryCode: s[:2],
	}, true
}

// luhnValid implements the ISIN checksum: letters are expanded to two digits
// (A=10 … Z=35), then the resulting digit string is checked with Luhn mod 10.
func luhnValid(isin string) bool {
	var b strings.Builder
	b.Grow(len(isin) * 2)
	for _, r := range isin {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			n := int(r-'A') + 10
			b.WriteString(itoa(n))
		default:
			return false
		}
	}

	digits := b.String()
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string([]rune{rune('0' + n/10), rune('0' + n%10)})
}
