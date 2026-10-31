package marketdata

import (
	"encoding/json"
	"testing"
)

func TestDecimalParsingAndFormatting(t *testing.T) {
	tests := []struct {
		input    string
		scale    int
		expected string
	}{
		{"123.456", 2, "123.46"},
		{"123.454", 2, "123.45"},
		{"1000,500.25", 2, "1000500.25"},
		{"0.0001", 4, "0.0001"},
		{"-45.75", 1, "-45.8"},
		{"0", 2, "0.00"},
		{"4800000000000.50", 2, "4800000000000.50"}, // Rs 4.8 trillion
	}

	for _, tc := range tests {
		d, err := Parse(tc.input)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", tc.input, err)
		}
		formatted := d.Format(tc.scale)
		if formatted != tc.expected {
			t.Errorf("Format(%d) for %s = %s, expected %s", tc.scale, tc.input, formatted, tc.expected)
		}
	}
}

func TestDecimalArithmetic(t *testing.T) {
	a := MustParse("100.50")
	b := MustParse("50.25")

	sum := a.Add(b)
	if sum.Format(2) != "150.75" {
		t.Errorf("Add: expected 150.75, got %s", sum.Format(2))
	}

	diff := a.Sub(b)
	if diff.Format(2) != "50.25" {
		t.Errorf("Sub: expected 50.25, got %s", diff.Format(2))
	}

	prod := a.Mul(b)
	if prod.Format(4) != "5050.1250" {
		t.Errorf("Mul: expected 5050.1250, got %s", prod.Format(4))
	}

	quo, err := a.Div(b)
	if err != nil || quo.Format(2) != "2.00" {
		t.Errorf("Div: expected 2.00, got %s (err: %v)", quo.Format(2), err)
	}

	_, err = a.Div(Zero())
	if err != ErrDivideByZero {
		t.Errorf("expected ErrDivideByZero, got %v", err)
	}
}

func TestDecimalComparisons(t *testing.T) {
	a := MustParse("10.5")
	b := MustParse("10.500")
	c := MustParse("10.51")
	d := MustParse("10.49")

	if !a.Equal(b) {
		t.Errorf("expected 10.5 == 10.500")
	}
	if !c.GreaterThan(a) {
		t.Errorf("expected 10.51 > 10.5")
	}
	if !d.LessThan(a) {
		t.Errorf("expected 10.49 < 10.5")
	}
}

func TestJSONSerialization(t *testing.T) {
	type Sample struct {
		Price Decimal `json:"price"`
	}

	orig := Sample{Price: MustParse("123456.7890")}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed Sample
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if !parsed.Price.Equal(orig.Price) {
		t.Errorf("JSON roundtrip mismatch: expected %s, got %s", orig.Price, parsed.Price)
	}
}
