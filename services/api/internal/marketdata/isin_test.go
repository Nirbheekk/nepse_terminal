package marketdata

import "testing"

func TestNormalizeISINValid(t *testing.T) {
	cases := map[string]string{
		"US0378331005":   "US0378331005", // Apple, the canonical worked example
		"npe322a00005":   "NPE322A00005",
		" NPE001A00005 ": "NPE001A00005",
		"NPE-001A-00005": "NPE001A00005",
	}
	for in, want := range cases {
		got, ok := NormalizeISIN(in)
		if !ok {
			t.Errorf("NormalizeISIN(%q) rejected a valid ISIN", in)
			continue
		}
		if got.Value != want {
			t.Errorf("NormalizeISIN(%q).Value = %q, want %q", in, got.Value, want)
		}
		if !got.CheckDigit {
			t.Errorf("NormalizeISIN(%q) failed a valid checksum", in)
		}
		if got.CountryCode != want[:2] {
			t.Errorf("country code = %q, want %q", got.CountryCode, want[:2])
		}
	}
}

func TestNormalizeISINRejectsMalformed(t *testing.T) {
	for _, bad := range []string{
		"",
		"N211AVU", // issuer-internal code, not an ISIN
		"BSL",
		"US037833100",   // too short
		"US03783310055", // too long
		"1S0378331005",  // must start with two letters
		"US03783310!5",  // punctuation
	} {
		if v, ok := NormalizeISIN(bad); ok {
			t.Errorf("NormalizeISIN(%q) accepted a malformed value as %q", bad, v.Value)
		}
	}
}

// A well-formed ISIN with a wrong check digit is still returned, flagged: the
// identifier is useful for research even when the issuer got the checksum wrong.
func TestNormalizeISINFlagsBadChecksum(t *testing.T) {
	got, ok := NormalizeISIN("US0378331004") // last digit altered
	if !ok {
		t.Fatal("a structurally valid ISIN should be returned")
	}
	if got.CheckDigit {
		t.Fatal("an incorrect check digit was reported as valid")
	}
	if got.Value != "US0378331004" {
		t.Fatalf("value = %q", got.Value)
	}
}

func TestLuhnKnownVectors(t *testing.T) {
	valid := []string{
		"US0378331005",
		"GB0002634946",
		"DE0008430026",
		"NPE322A00005",
		"NPE001A11010",
	}
	for _, v := range valid {
		if !luhnValid(v) {
			t.Errorf("luhnValid(%q) = false, want true", v)
		}
	}
	// One altered character in each position class must fail.
	invalid := []string{"US0378331006", "GB0002634947", "NPE322A00006"}
	for _, v := range invalid {
		if luhnValid(v) {
			t.Errorf("luhnValid(%q) = true, want false", v)
		}
	}
}
