package ingestion

import (
	"errors"
	"strings"
	"testing"
	"time"

	"nepse/terminal/api/internal/marketdata"
)

func TestSpecsCoverEveryStream(t *testing.T) {
	// Every stream the adapters can emit must have somewhere to go: a stream
	// without a spec silently ingests nothing.
	for _, stream := range marketdata.AllStreams() {
		if stream == marketdata.StreamSummary {
			if !summaryUsesScalarPath(stream) {
				t.Errorf("summary must take the scalar path")
			}
			continue
		}
		if _, ok := specs[stream]; !ok {
			t.Errorf("no ingestion spec registered for stream %q", stream)
		}
	}
	// And no spec may exist for a stream nothing produces.
	for name := range specs {
		found := false
		for _, s := range marketdata.AllStreams() {
			if s == name {
				found = true
			}
		}
		if !found {
			t.Errorf("spec %q is not a known stream", name)
		}
	}
}

func TestSpecColumnCountsMatchNormalizers(t *testing.T) {
	cases := []struct {
		spec *streamSpec
		row  map[string]any
	}{
		{specs[marketdata.StreamCompanies], map[string]any{
			"symbol": "NABIL", "companyName": "Nabil Bank Limited", "sectorName": "Commercial Banks"}},
		{specs[marketdata.StreamSecurities], map[string]any{
			"symbol": "NABIL", "securityName": "Nabil Bank Limited", "activeStatus": "A"}},
		{specs[marketdata.StreamQuotes], map[string]any{
			"symbol": "NABIL", "business_date": "2026-10-29", "ltp": 550.0, "volume": 10.0}},
		{specs[marketdata.StreamMarketCap], map[string]any{
			"business_date": "2026-10-29", "market_cap": 4.8e12}},
		{specs[marketdata.StreamIndices], map[string]any{
			"kind": "headline", "index": "NEPSE Index", "currentValue": 2695.6,
			"generatedTime": "2026-10-29T14:59:59.817"}},
		{specs[marketdata.StreamFloorsheet], map[string]any{
			"contract_id": "2026102905004550", "business_date": "2026-10-29", "symbol": "DOLTI",
			"quantity": 10.0, "rate": 312.1, "amount": 3121.0, "trade_time": "2026-10-29 06:04:59.839663"}},
		{specs[marketdata.StreamMarketStatus], map[string]any{
			"isOpen": "CLOSE", "asOf": "2026-10-29T15:00:00",
			"captured_at": "2026-10-29T09:20:00Z"}},
	}
	for _, c := range cases {
		vals, err := c.spec.normalize(c.row)
		if err != nil {
			t.Errorf("%s: normalize failed: %v", c.spec.stream, err)
			continue
		}
		if len(vals) != len(c.spec.cols) {
			t.Errorf("%s: normalize returned %d values for %d columns (%v vs %v)",
				c.spec.stream, len(vals), len(c.spec.cols), vals, c.spec.cols)
		}
		for _, idx := range c.spec.keyIdx {
			if idx >= len(c.spec.cols) {
				t.Errorf("%s: key index %d is out of range", c.spec.stream, idx)
			}
		}
	}
}

func TestQuotesNormalizeRequiresAClose(t *testing.T) {
	spec := specs[marketdata.StreamQuotes]
	_, err := spec.normalize(map[string]any{"symbol": "NABIL", "business_date": "2026-10-29"})
	if err == nil {
		t.Fatal("a quote with no close and no open should be rejected")
	}
	if fieldOf(err) != "ltp" {
		t.Fatalf("rejection blamed field %q, want ltp", fieldOf(err))
	}

	// A zero price is the feed's way of saying "never traded". It is rejected
	// rather than stored as a quote that would rank as a 0% mover.
	if _, err := spec.normalize(map[string]any{
		"symbol": "NABILPNP", "business_date": "2026-10-29", "ltp": 0.0, "open": 0.0, "volume": 0.0,
	}); err == nil {
		t.Fatal("a zero-price observation was accepted")
	} else if fieldOf(err) != "ltp" {
		t.Fatalf("zero-price rejection blamed %q", fieldOf(err))
	}

	// A suspended scrip reports no last trade but does report an open price; that
	// bar is still a real observation.
	vals, err := spec.normalize(map[string]any{
		"symbol": "NABIL", "business_date": "2026-10-29", "open": 850.0, "volume": 0.0,
	})
	if err != nil {
		t.Fatalf("suspended scrip rejected: %v", err)
	}
	if closeIdx := indexOf(spec.cols, "close"); vals[closeIdx] != "850" {
		t.Fatalf("close fell back to %v, want the open price 850", vals[closeIdx])
	}
}

func TestQuotesStoreISINOnlyWhenItIsOne(t *testing.T) {
	spec := specs[marketdata.StreamQuotes]
	isinIdx := indexOf(spec.cols, "isin")

	vals, err := spec.normalize(map[string]any{
		"symbol": "NABIL", "business_date": "2026-10-29", "ltp": 550.0, "isin": "NPE001A00005",
	})
	if err != nil {
		t.Fatal(err)
	}
	if vals[isinIdx] != "NPE001A00005" {
		t.Fatalf("valid ISIN not stored: %v", vals[isinIdx])
	}

	// A short issuer code must not be written into the ISIN column: the column has
	// a unique index and a fake identifier would poison it.
	vals, err = spec.normalize(map[string]any{
		"symbol": "AVU", "business_date": "2026-10-29", "ltp": 10.0, "isin": "N211AVU",
	})
	if err != nil {
		t.Fatal(err)
	}
	if vals[isinIdx] != nil {
		t.Fatalf("issuer code stored as an ISIN: %v", vals[isinIdx])
	}
}

func TestFloorsheetRejectsUnusableContracts(t *testing.T) {
	spec := specs[marketdata.StreamFloorsheet]
	base := map[string]any{
		"contract_id": "2026102905004550", "business_date": "2026-10-29", "symbol": "DOLTI",
		"quantity": 10.0, "rate": 312.1,
	}

	if _, err := spec.normalize(base); err != nil {
		t.Fatalf("valid contract rejected: %v", err)
	}

	for name, mutate := range map[string]func(map[string]any){
		"missing contract id":     func(r map[string]any) { delete(r, "contract_id"); delete(r, "contract_no") },
		"non-numeric contract id": func(r map[string]any) { r["contract_id"] = "abc" },
		"zero quantity":           func(r map[string]any) { r["quantity"] = 0 },
		"negative rate":           func(r map[string]any) { r["rate"] = -1 },
		"missing date":            func(r map[string]any) { delete(r, "business_date") },
		"missing symbol":          func(r map[string]any) { delete(r, "symbol") },
	} {
		row := map[string]any{}
		for k, v := range base {
			row[k] = v
		}
		mutate(row)
		if _, err := spec.normalize(row); err == nil {
			t.Errorf("%s: contract was accepted", name)
		}
	}
}

func TestFloorsheetDerivesAmountOnlyWhenAbsent(t *testing.T) {
	spec := specs[marketdata.StreamFloorsheet]
	amountIdx := indexOf(spec.cols, "amount")

	vals, err := spec.normalize(map[string]any{
		"contract_id": "1", "business_date": "2026-10-29", "symbol": "DOLTI",
		"quantity": 10, "rate": 312.1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if vals[amountIdx] != "3121" && !strings.HasPrefix(vals[amountIdx].(string), "3121") {
		t.Fatalf("derived amount = %v, want 3121", vals[amountIdx])
	}

	vals, err = spec.normalize(map[string]any{
		"contract_id": "2", "business_date": "2026-10-29", "symbol": "DOLTI",
		"quantity": 10, "rate": 312.1, "amount": 9999.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if vals[amountIdx] != "9999" {
		t.Fatalf("reported amount was overwritten: %v", vals[amountIdx])
	}
}

func TestIndicesNeedADateAndCarryAnObservationTime(t *testing.T) {
	spec := specs[marketdata.StreamIndices]
	dateIdx := indexOf(spec.cols, "trading_date")
	observedIdx := indexOf(spec.cols, "observed_at")

	vals, err := spec.normalize(map[string]any{
		"kind": "headline", "index": "NEPSE Index", "currentValue": 2695.6,
		"generatedTime": "2026-10-29T14:59:59.817",
	})
	if err != nil {
		t.Fatal(err)
	}
	if vals[dateIdx] != "2026-10-29" {
		t.Fatalf("trading date = %v", vals[dateIdx])
	}
	observed, ok := vals[observedIdx].(*time.Time)
	if !ok || observed == nil {
		t.Fatalf("observed_at = %v, want a timestamp", vals[observedIdx])
	}
	// 14:59:59 NPT is 09:14:59 UTC; storing the wall clock would be a 5h45m error.
	if got := observed.UTC().Format("15:04"); got != "09:14" {
		t.Fatalf("observed_at = %s UTC, want 09:14", got)
	}

	// Two captures of the same index on one day must not collide, or they would
	// overwrite each other on every ingest run.
	other, err := spec.normalize(map[string]any{
		"kind": "headline", "index": "NEPSE Index", "currentValue": 2695.6,
		"business_date": "2026-10-29", "captured_at": "2026-10-29 09:31:13.885504",
	})
	if err != nil {
		t.Fatal(err)
	}
	if *observed == *(other[observedIdx].(*time.Time)) {
		t.Fatal("two captures produced the same observation time")
	}

	if _, err := spec.normalize(map[string]any{"kind": "sector", "index": "Banking SubIndex", "currentValue": 1606.6}); err == nil {
		t.Fatal("an index row with no date and no observation time should be rejected")
	}
}

func TestMarketCapRequiresAValue(t *testing.T) {
	spec := specs[marketdata.StreamMarketCap]
	if _, err := spec.normalize(map[string]any{"business_date": "2026-10-29"}); err == nil {
		t.Fatal("a capitalisation row with no value was accepted")
	}
	vals, err := spec.normalize(map[string]any{
		"business_date": "2026-10-29", "market_cap": 4893040965845.699,
		"float_market_cap": 1580179643747.9, "covered": 403, "total": 650,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != len(spec.cols) {
		t.Fatalf("normalize returned %d values, want %d", len(vals), len(spec.cols))
	}
}

func TestMarketStatusKeepsTheSourceWordingAndNormalisesTheState(t *testing.T) {
	spec := specs[marketdata.StreamMarketStatus]
	stateIdx := indexOf(spec.cols, "status")
	reportedIdx := indexOf(spec.cols, "reported_status")
	asOfIdx := indexOf(spec.cols, "as_of")

	vals, err := spec.normalize(map[string]any{
		"isOpen": "CLOSE", "asOf": "2026-10-29T15:00:00", "captured_at": "2026-10-29T09:20:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if vals[stateIdx] != "closed" {
		t.Fatalf("state = %v, want closed", vals[stateIdx])
	}
	if vals[reportedIdx] != "CLOSE" {
		t.Fatalf("reported status was rewritten: %v", vals[reportedIdx])
	}
	// 15:00 NPT is 09:15 UTC; treating the wall clock as UTC would shift the
	// status by 5h45m and make a closed market look open.
	if got := vals[asOfIdx].(time.Time).UTC().Format("15:04"); got != "09:15" {
		t.Fatalf("as_of = %s UTC, want 09:15", got)
	}

	// An unrecognised value is recorded, not guessed.
	vals, err = spec.normalize(map[string]any{
		"isOpen": "HALT", "asOf": "2026-10-29T11:00:00", "captured_at": "2026-10-29T05:20:00Z",
	})
	if err != nil {
		t.Fatalf("an unknown state should still be stored: %v", err)
	}
	if vals[stateIdx] != "unknown" {
		t.Fatalf("unknown state normalised to %v", vals[stateIdx])
	}

	if _, err := spec.normalize(map[string]any{"asOf": "2026-10-29T11:00:00"}); err == nil {
		t.Fatal("a status row with no state was accepted")
	}
	if _, err := spec.normalize(map[string]any{"isOpen": "OPEN"}); err == nil {
		t.Fatal("a status row with no timestamp was accepted")
	}
}

func TestQuotesKeepTheReportedCapitalisation(t *testing.T) {
	spec := specs[marketdata.StreamQuotes]
	capIdx := indexOf(spec.cols, "reported_cap")
	vals, err := spec.normalize(map[string]any{
		"symbol": "ACLBSL", "business_date": "2026-10-29", "ltp": 850.0,
		"market_cap": 3120719750.0, "listed_shares": 3671435.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := vals[capIdx].(*string); !ok || got == nil || *got != "3120719750" {
		t.Fatalf("reported capitalisation = %v, want 3120719750", vals[capIdx])
	}

	vals, err = spec.normalize(map[string]any{
		"symbol": "NABIL", "business_date": "2026-10-29", "ltp": 550.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := vals[capIdx].(*string); !ok || v != nil {
		t.Fatalf("absent capitalisation became %v instead of NULL", vals[capIdx])
	}
}

func TestCompaniesDefaultsForMissingReferenceFields(t *testing.T) {
	spec := specs[marketdata.StreamCompanies]
	vals, err := spec.normalize(map[string]any{"symbol": "zzz", "companyName": "Test Ltd"})
	if err != nil {
		t.Fatal(err)
	}
	if vals[indexOf(spec.cols, "symbol")] != "ZZZ" {
		t.Errorf("symbol not upper-cased: %v", vals[0])
	}
	if vals[indexOf(spec.cols, "sector_code")] != "unclassified" {
		t.Errorf("missing sector should default, got %v", vals[indexOf(spec.cols, "sector_code")])
	}
	if vals[indexOf(spec.cols, "instrument")] != "equity" {
		t.Errorf("missing instrument should default to equity")
	}
}

// ---------------------------------------------------------------- engine units

func TestSecurityTypeComesFromTheNameNotTheSymbol(t *testing.T) {
	cases := map[string]string{
		"Nabil Bank Limited": "ordinary",
		"Aarambha Chautari Laghubitta Bittiya Sanstha Limited Promoter Share": "promoter", "Asian Life Insurance Co. Ltd Promoter Share": "promoter",
		// The source misspells "Promoter" for many issuers.
		"NABIL Bank Limited Promotor Share": "promoter",
		"Nepal Bank Limited Promotor share": "promoter",
		"10% Nabil Debenture 2088":          "debenture",
		"NMB Sulav Investment Fund - 1":     "mutual_fund",
		"Government Bond 2085":              "bond",
		"":                                  "unclassified",
	}
	for name, want := range cases {
		if got := securityTypeOf(name); got != want {
			t.Errorf("securityTypeOf(%q) = %q, want %q", name, got, want)
		}
	}
	// The suffix alone must never decide the class: a symbol ending in P can be
	// an ordinary equity of an issuer whose name says nothing about promoters.
	if got := securityTypeOf("Some Company Limited"); got != "ordinary" {
		t.Fatalf("plain name classified as %q", got)
	}
}

func TestDedupeKeepsLastAndCountsDuplicates(t *testing.T) {
	rows := [][]any{
		{"1", "first"},
		{"2", "other"},
		{"1", "second"},
	}
	out, dups := dedupe(rows, []int{0})
	if len(out) != 2 {
		t.Fatalf("dedupe left %d rows, want 2", len(out))
	}
	if dups != 1 {
		t.Fatalf("duplicates counted = %d, want 1", dups)
	}
	if out[0][1] != "second" {
		t.Fatalf("dedupe kept %v, want the last occurrence", out[0][1])
	}
}

func TestDedupeWithoutKeysIsIdentity(t *testing.T) {
	rows := [][]any{{"1"}, {"1"}}
	out, dups := dedupe(rows, nil)
	if len(out) != 2 || dups != 0 {
		t.Fatalf("dedupe with no key changed the batch: %d rows, %d duplicates", len(out), dups)
	}
}

func TestStatusFor(t *testing.T) {
	if got := statusFor(Summary{}); got != "success" {
		t.Errorf("empty run status = %q", got)
	}
	if got := statusFor(Summary{Streams: 3, Failed: 1, Results: make([]Result, 3)}); got != "partial" {
		t.Errorf("partially failed run = %q, want partial", got)
	}
	if got := statusFor(Summary{Streams: 2, Failed: 2, Results: make([]Result, 2)}); got != "failed" {
		t.Errorf("fully failed run = %q, want failed", got)
	}
}

func TestWants(t *testing.T) {
	if !wants(nil, "anything") {
		t.Error("an empty filter should match everything")
	}
	if !wants([]string{"a", "b"}, "b") {
		t.Error("listed value not matched")
	}
	if wants([]string{"a"}, "b") {
		t.Error("unlisted value matched")
	}
}

func TestFieldErrorCarriesTheFieldName(t *testing.T) {
	err := ferr("quantity", "must be positive, got %s", "-1")
	if fieldOf(err) != "quantity" {
		t.Fatalf("fieldOf = %q", fieldOf(err))
	}
	if !strings.Contains(reasonOf(err), "must be positive") {
		t.Fatalf("reasonOf = %q", reasonOf(err))
	}
	wrapped := error(err)
	if !errors.Is(wrapped, err) {
		t.Fatal("field errors should compare with errors.Is")
	}
}

func TestSampleOfTrimsLongValues(t *testing.T) {
	long := strings.Repeat("x", 500)
	sample := sampleOf(map[string]any{"raw": long, "symbol": "NABIL"})
	if s, _ := sample["raw"].(string); len(s) > 130 {
		t.Fatalf("sample kept %d characters of a long field", len(s))
	}
	if sample["symbol"] != "NABIL" {
		t.Fatal("sample dropped a short field")
	}
}

func indexOf(cols []string, name string) int {
	for i, c := range cols {
		if c == name {
			return i
		}
	}
	panic("column not found: " + name)
}
