package floorsheet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeArchive materialises a small archive on disk: a manifest plus one JSON
// page per entry, with the digests the real capture writes. Tests describe what
// the exchange would have served and assert what the verifier concludes.
type pageSpec struct {
	page    int
	trades  []map[string]any
	corrupt bool // tamper the file after the digest was recorded
	omit    bool // list the page in the manifest but write no file
}

func writeArchive(t *testing.T, dir, date string, totalPages int, trades int, specs []pageSpec, sweeps []int) string {
	t.Helper()
	root := filepath.Join(dir, date)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	man := map[string]any{
		"schema":            "floorsheet-pages/1",
		"business_date":     date,
		"session_id":        "test-session",
		"total_pages":       totalPages,
		"total_elements":    trades,
		"converged":         true,
		"pages_fetched":     totalPages,
		"unique_contracts":  trades,
		"missing_contracts": 0,
		"source": map[string]any{
			"endpoint":      "/api/nots/nepse-data/floorsheet",
			"sort":          "contractId,desc",
			"mode":          "operator_captured",
			"licence_class": "public_unverified",
		},
	}

	var detail []map[string]any
	for _, s := range sweeps {
		for _, spec := range specs {
			name := fmt.Sprintf("sweep%d-page%d.json", s, spec.page)
			rel := "floorsheet-pages/" + date + "/" + name
			body := map[string]any{
				"business_date":  date,
				"page":           spec.page,
				"total_pages":    totalPages,
				"total_elements": trades,
				"content":        spec.trades,
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(raw)
			if !spec.omit {
				out := raw
				if spec.corrupt {
					out = append(out, []byte(" ")...)
				}
				path := filepath.Join(dir, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, out, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			detail = append(detail, map[string]any{
				"page":   spec.page,
				"file":   rel,
				"rows":   len(spec.trades),
				"sha256": hex.EncodeToString(sum[:]),
				"bytes":  len(raw),
			})
		}
	}
	man["sweeps"] = []map[string]any{{
		"sweep": 1, "pages": len(specs), "total_pages": totalPages,
		"total_elements": trades, "rows": len(specs),
		"pages_detail": detail,
	}}
	body, err := json.Marshal(man)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func trade(id int64, syms string, at string) map[string]any {
	return map[string]any{
		"contractId":       id,
		"stockSymbol":      syms,
		"businessDate":     "2026-10-29",
		"contractQuantity": 10,
		"contractRate":     415,
		"contractAmount":   4150,
		"buyerMemberId":    58,
		"sellerMemberId":   67,
		"buyerBrokerName":  nil,
		"sellerBrokerName": nil,
		"tradeTime":        at,
	}
}

// A complete archive is the only state that may report Complete().
func TestLoadCompleteArchive(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "2026-10-29", 2, 4, []pageSpec{
		{page: 0, trades: []map[string]any{
			trade(4, "RAWA", "2026-10-29T15:00:00.000321"),
			trade(3, "NRM", "2026-10-29T14:59:59.979"),
		}},
		{page: 1, trades: []map[string]any{
			trade(2, "BHL", "2026-10-29T14:59:59"),
			trade(1, "GVL", "2026-10-29T14:59:58.1"),
		}},
	}, []int{1})

	d, err := Load(dir, "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Check.Complete() {
		t.Fatalf("expected complete, got errors=%v", d.Check.Errors)
	}
	if d.Check.UniqueContracts != 4 || d.Check.MissingContracts != 0 {
		t.Fatalf("contracts: unique=%d missing=%d", d.Check.UniqueContracts, d.Check.MissingContracts)
	}
	if d.Check.PagesPresent != 2 || d.Check.MissingPages != nil {
		t.Fatalf("pages: present=%d missing=%v", d.Check.PagesPresent, d.Check.MissingPages)
	}
	// Order is newest contract first, mirroring the exchange's sort.
	if got := d.Order[0].ContractID.String(); got != "4" {
		t.Fatalf("first contract = %s, want 4", got)
	}
	if got := d.Order[3].ContractID.String(); got != "1" {
		t.Fatalf("last contract = %s, want 1", got)
	}
}

// A page window that moved mid-capture loses contracts. That must be an error,
// not a smaller market.
func TestLoadMissingContractsFails(t *testing.T) {
	dir := t.TempDir()
	// The manifest claims three contracts; the pages carry two.
	writeArchive(t, dir, "2026-10-29", 1, 3, []pageSpec{
		{page: 0, trades: []map[string]any{
			trade(2, "RAWA", "2026-10-29T15:00:00"),
			trade(1, "NRM", "2026-10-29T14:59:59"),
		}},
	}, []int{1})

	d, err := Load(dir, "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	if d.Check.MissingContracts != 1 {
		t.Fatalf("missing = %d, want 1", d.Check.MissingContracts)
	}
	if d.Check.Complete() {
		t.Fatal("an under-filled archive must not be complete")
	}
}

func TestMissingPageIsReported(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "2026-10-29", 3, 2, []pageSpec{
		{page: 0, trades: []map[string]any{trade(2, "RAWA", "2026-10-29T15:00:00")}},
		{page: 1, omit: true},
		{page: 2, trades: []map[string]any{trade(1, "NRM", "2026-10-29T14:59:59")}},
	}, []int{1})

	d, err := Load(dir, "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Check.MissingPages) != 1 || d.Check.MissingPages[0] != 1 {
		t.Fatalf("missing pages = %v, want [1]", d.Check.MissingPages)
	}
	if d.Check.Complete() {
		t.Fatal("a missing page must not verify")
	}
}

// A byte that changed after capture is a digest failure, even if it still parses.
func TestDigestMismatchIsDetected(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "2026-10-29", 1, 1, []pageSpec{
		{page: 0, trades: []map[string]any{trade(1, "RAWA", "2026-10-29T15:00:00")}, corrupt: true},
	}, []int{1})

	d, err := Load(dir, "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	if d.Check.DigestFailures != 1 {
		t.Fatalf("digest failures = %d, want 1", d.Check.DigestFailures)
	}
	if d.Check.Complete() {
		t.Fatal("a tampered page must not verify")
	}
}

// Replaying sweeps duplicates rows; they are counted and collapsed, and the
// contract set still verifies.
func TestDuplicateContractsCollapse(t *testing.T) {
	dir := t.TempDir()
	specs := []pageSpec{
		{page: 0, trades: []map[string]any{
			trade(2, "RAWA", "2026-10-29T15:00:00"),
			trade(1, "NRM", "2026-10-29T14:59:59"),
		}},
	}
	writeArchive(t, dir, "2026-10-29", 1, 2, specs, []int{1, 2})

	d, err := Load(dir, "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	if d.Check.DuplicateContracts != 2 {
		t.Fatalf("duplicates = %d, want 2", d.Check.DuplicateContracts)
	}
	if len(d.Order) != 2 {
		t.Fatalf("order length = %d, want 2", len(d.Order))
	}
	if !d.Check.Complete() {
		t.Fatalf("duplicates are not a completeness failure: %v", d.Check.Errors)
	}
}

// Precision is reported from the digits actually supplied, per row.
func TestTimestampPrecisionIsMeasured(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "2026-10-29", 1, 4, []pageSpec{
		{page: 0, trades: []map[string]any{
			trade(4, "A", "2026-10-29T15:00:00.000321"), // microseconds
			trade(3, "B", "2026-10-29T15:00:00.123"),    // milliseconds
			trade(2, "C", "2026-10-29T15:00:00"),        // seconds
			trade(1, "D", ""),                           // none supplied
		}},
	}, []int{1})

	d, err := Load(dir, "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	c := d.Check
	if c.Microseconds != 1 || c.Milliseconds != 1 || c.SecondResolution != 1 {
		t.Fatalf("precision counts: us=%d ms=%d s=%d", c.Microseconds, c.Milliseconds, c.SecondResolution)
	}
	if c.MissingTradeTime != 1 {
		t.Fatalf("missing trade times = %d, want 1", c.MissingTradeTime)
	}
	want := "mixed: 1 microsecond, 1 millisecond, 1 second-resolution"
	if c.TimestampPrecision != want {
		t.Fatalf("label = %q, want %q", c.TimestampPrecision, want)
	}
}

func TestExecuteTimeKeepsSuppliedPrecision(t *testing.T) {
	got, ok := ExecuteTime("2026-10-29T15:00:00.000321")
	if !ok {
		t.Fatal("expected a time")
	}
	// 15:00:00 NPT is 09:15:00Z; the fraction must survive the conversion.
	if got.Format("2006-01-02T15:04:05.000000Z07:00") != "2026-10-29T09:15:00.000321Z" {
		t.Fatalf("converted = %s", got.Format(time.RFC3339Nano))
	}

	if _, ok := ExecuteTime(""); ok {
		t.Fatal("an absent time must not be invented")
	}
	if _, ok := ExecuteTime("not-a-time"); ok {
		t.Fatal("an unparsable time must not be accepted")
	}

	// An explicit offset is already unambiguous and must not be shifted again.
	withZone, ok := ExecuteTime("2026-10-29T15:00:00+05:45")
	if !ok {
		t.Fatal("expected a time with offset")
	}
	if want := "2026-10-29T09:15:00Z"; withZone.UTC().Format(time.RFC3339) != want {
		t.Fatalf("offset time = %s, want %s", withZone.UTC().Format(time.RFC3339), want)
	}
}

func TestAvailableDatesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "2026-10-28", 1, 1, []pageSpec{{page: 0, trades: []map[string]any{trade(1, "A", "2026-10-28T15:00:00")}}}, []int{1})
	writeArchive(t, dir, "2026-10-29", 1, 1, []pageSpec{{page: 0, trades: []map[string]any{trade(1, "A", "2026-10-29T15:00:00")}}}, []int{1})
	// A directory without a manifest is not a date, even if it is named like one.
	if err := os.MkdirAll(filepath.Join(dir, "2026-10-30"), 0o755); err != nil {
		t.Fatal(err)
	}

	dates, err := AvailableDates(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 2 || dates[0] != "2026-10-29" || dates[1] != "2026-10-28" {
		t.Fatalf("dates = %v", dates)
	}

	if _, err := AvailableDates(filepath.Join(dir, "nope")); err != nil {
		t.Fatalf("a missing archive directory should be empty, not an error: %v", err)
	}
}
