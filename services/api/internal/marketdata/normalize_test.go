package marketdata

import (
	"testing"
	"time"
)

func TestDecimalPreservesSourceText(t *testing.T) {
	// A string is returned with its digits intact: routing it through float64
	// would change 12345678901234567 into 12345678901234568.
	got, err := DecimalString("12345678901234567")
	if err != nil {
		t.Fatal(err)
	}
	if got != "12345678901234567" {
		t.Fatalf("Decimal preserved %q, want the original string", got)
	}

	got, err = DecimalString("1,234.5000")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1234.5000" {
		t.Fatalf("DecimalString(1,234.5000) = %q, want 1234.5000", got)
	}

	got, err = DecimalString(float64(249.5))
	if err != nil {
		t.Fatal(err)
	}
	if got != "249.5" {
		t.Fatalf("DecimalString(249.5) = %q", got)
	}

	got, err = DecimalString(42)
	if err != nil || got != "42" {
		t.Fatalf("DecimalString(42) = %q, %v", got, err)
	}
}

func TestDecimalRejectsNonNumbers(t *testing.T) {
	for _, bad := range []any{nil, "", "  ", "abc", "1.2.3", "NaN"} {
		if _, err := DecimalString(bad); err == nil {
			t.Errorf("DecimalString(%#v) accepted an invalid value", bad)
		}
	}
}

func TestOptionalDecimalDistinguishesMissingFromInvalid(t *testing.T) {
	// Missing is not an error: a suspended security legitimately has no volume.
	if v, err := OptionalDecimal(nil); err != nil || v != nil {
		t.Fatalf("OptionalDecimal(nil) = %v, %v", v, err)
	}
	if v, err := OptionalDecimal(""); err != nil || v != nil {
		t.Fatalf("OptionalDecimal(\"\") = %v, %v", v, err)
	}
	// Present but unparseable is an error: it means the payload changed shape.
	if _, err := OptionalDecimal("n/a"); err == nil {
		t.Fatal("OptionalDecimal(\"n/a\") should be an error")
	}
}

func TestPositiveDecimal(t *testing.T) {
	if v, err := PositiveDecimal(10.5, "quantity"); err != nil || v != "10.5" {
		t.Fatalf("PositiveDecimal(10.5) = %q, %v", v, err)
	}
	for _, bad := range []any{0, -1, "0", "-5"} {
		if _, err := PositiveDecimal(bad, "quantity"); err == nil {
			t.Errorf("PositiveDecimal(%v) accepted a non-positive value", bad)
		}
	}
}

func TestDateTakesTheCalendarPart(t *testing.T) {
	cases := map[string]string{
		"2026-10-29":              "2026-10-29",
		"2026-10-29T11:49:59":     "2026-10-29",
		"2026-10-29 06:04:59.839": "2026-10-29",
	}
	for in, want := range cases {
		got, err := Date(in)
		if err != nil {
			t.Fatalf("Date(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("Date(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := Date("29/10/2026"); err == nil {
		t.Error("Date accepted a non-ISO date")
	}
}

// The time basis is the single most dangerous input in this package: the legacy
// collector stores UTC in SQLite while the exchange's JSON carries NPT wall
// clocks with no offset. Interpreting one as the other shifts every row by
// 5h45m, which is exactly the defect this test pins down.
func TestTimestampHonoursTheDeclaredBasis(t *testing.T) {
	utc, err := Timestamp("2026-10-29 06:04:59.839663", BasisUTC)
	if err != nil || utc == nil {
		t.Fatalf("Timestamp UTC: %v", err)
	}
	if got := utc.Format("15:04:05"); got != "06:04:59" {
		t.Fatalf("UTC basis shifted the value to %s", got)
	}

	npt, err := Timestamp("2026-10-29T11:49:59.839663", BasisNPT)
	if err != nil || npt == nil {
		t.Fatalf("Timestamp NPT: %v", err)
	}
	if got := npt.UTC().Format("15:04:05"); got != "06:04:59" {
		t.Fatalf("NPT 11:49:59 should be 06:04:59 UTC, got %s", got)
	}

	// The same wall clock under the two bases must differ by exactly 5h45m.
	wallUTC, _ := Timestamp("2026-10-29T11:49:59", BasisUTC)
	wallNPT, _ := Timestamp("2026-10-29T11:49:59", BasisNPT)
	if diff := wallUTC.Sub(*wallNPT); diff != NepalOffset {
		t.Fatalf("basis difference = %v, want %v", diff, NepalOffset)
	}
}

func TestTimestampWithExplicitZoneIgnoresBasis(t *testing.T) {
	// An offset in the input is authoritative: Nepal is UTC+05:45, so 11:49:59+05:45
	// is 06:04:59Z no matter which basis is declared.
	for _, basis := range []Basis{BasisUTC, BasisNPT} {
		got, err := Timestamp("2026-10-29T11:49:59+05:45", basis)
		if err != nil || got == nil {
			t.Fatalf("basis %v: %v", basis, err)
		}
		if formatted := got.UTC().Format("15:04:05"); formatted != "06:04:59" {
			t.Errorf("basis %v: explicit offset ignored, got %s", basis, formatted)
		}
	}
}

func TestTimestampEmptyIsNilNotError(t *testing.T) {
	for _, v := range []any{nil, "", "   "} {
		got, err := Timestamp(v, BasisUTC)
		if err != nil {
			t.Errorf("Timestamp(%#v) errored on an absent value: %v", v, err)
		}
		if got != nil {
			t.Errorf("Timestamp(%#v) = %v, want nil", v, got)
		}
	}
}

func TestTimestampRejectsGarbage(t *testing.T) {
	if _, err := Timestamp("yesterday", BasisUTC); err == nil {
		t.Fatal("Timestamp accepted a non-timestamp")
	}
}

func TestSymbol(t *testing.T) {
	good := map[string]string{
		"nabil":      "NABIL",
		" NABIL ":    "NABIL",
		"GBILD86/87": "GBILD86/87", // present in the exchange's own reference data
		"ADBLB86":    "ADBLB86",
		"NIFRA_P":    "NIFRA_P",
	}
	for in, want := range good {
		got, err := Symbol(in)
		if err != nil {
			t.Errorf("Symbol(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Symbol(%q) = %q, want %q", in, got, want)
		}
	}

	for _, bad := range []string{"", "   ", "NABIL;DROP", "NABIL'--", "A B", "%2e%2e"} {
		if _, err := Symbol(bad); err == nil {
			t.Errorf("Symbol(%q) accepted an unsafe value", bad)
		}
	}
}

func TestInteger(t *testing.T) {
	cases := []struct {
		in   any
		want int
		ok   bool
	}{
		{float64(5), 5, true},
		{int64(7), 7, true},
		{"12", 12, true},
		{float64(5.5), 0, false},
		{"5.5", 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		got, err := Integer(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("Integer(%#v) = %d, %v", c.in, got, err)
		}
		if !c.ok && err == nil {
			t.Errorf("Integer(%#v) accepted a non-integer", c.in)
		}
	}
}

func TestTextAndOptionalText(t *testing.T) {
	if v, err := Text("  hello ", "name"); err != nil || v != "hello" {
		t.Fatalf("Text = %q, %v", v, err)
	}
	if _, err := Text("   ", "name"); err == nil {
		t.Error("Text accepted a blank required value")
	}
	if v := OptionalText(nil); v != nil {
		t.Errorf("OptionalText(nil) = %v", v)
	}
	if v := OptionalText("  "); v != nil {
		t.Errorf("OptionalText(blank) = %v", v)
	}
	if v := OptionalText("x"); v == nil || *v != "x" {
		t.Errorf("OptionalText(x) = %v", v)
	}
}

func TestNepalOffsetIsStable(t *testing.T) {
	// Nepal has never observed daylight saving; a DST-aware basis would make the
	// historical reconstruction wrong half the year.
	if NepalOffset != 5*time.Hour+45*time.Minute {
		t.Fatalf("NepalOffset = %v", NepalOffset)
	}
}
