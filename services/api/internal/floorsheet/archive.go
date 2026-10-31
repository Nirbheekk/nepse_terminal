// Package floorsheet verifies and ingests the page-level floorsheet archive.
//
// The archive is written by the capture process as one JSON file per page
// response plus a manifest recording the pagination the exchange reported. Two
// properties of that response shape everything here:
//
//   - Pages are sorted by contract id descending while the session is open. A
//     trade published mid-sweep pushes every later row down one position, so the
//     boundary rows of consecutive pages shift and a single pass silently skips
//     some contracts. The archive therefore keeps every sweep, and completeness is
//     judged against the exchange's own totalElements — never against the number
//     of rows that happen to be on disk.
//   - A contract id is the exchange's unique contract identifier and is the
//     natural key. Re-ingesting the same page is a no-op, so recovery from a
//     failure is a re-run rather than a repair.
//
// Verification is deliberately separate from ingestion: it reads only the files
// and reports what is wrong with them, so the answer to "is this date complete?"
// does not depend on a database being reachable or on trusting a previous run.
package floorsheet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NPT is the exchange's wall clock. Trade times arrive without a zone marker and
// mean local Nepal time; reading them as UTC shifts every row by 5h45m.
var NPT = time.FixedZone("NPT", 5*60*60+45*60)

// Manifest is the archive's own account of a capture.
type Manifest struct {
	Schema           string   `json:"schema"`
	BusinessDate     string   `json:"business_date"`
	SessionID        string   `json:"session_id"`
	TotalPages       int      `json:"total_pages"`
	TotalElements    int      `json:"total_elements"`
	Converged        bool     `json:"converged"`
	PagesFetched     int      `json:"pages_fetched"`
	RowsRead         int      `json:"rows_read"`
	UniqueContracts  int      `json:"unique_contracts"`
	DuplicateRows    int      `json:"duplicate_rows"`
	MissingContracts int      `json:"missing_contracts"`
	Notes            []string `json:"notes"`
	Source           struct {
		Endpoint     string `json:"endpoint"`
		Sort         string `json:"sort"`
		Mode         string `json:"mode"`
		LicenceClass string `json:"licence_class"`
	} `json:"source"`
	Sweeps []Sweep `json:"sweeps"`
}

// Sweep is one pass over the page range.
type Sweep struct {
	Sweep             int         `json:"sweep"`
	Pages             int         `json:"pages"`
	TotalPages        int         `json:"total_pages"`
	TotalElements     int         `json:"total_elements"`
	Rows              int         `json:"rows"`
	UniqueNew         int         `json:"unique_new"`
	UnionTotal        int         `json:"union_total"`
	DuplicatedInSweep int         `json:"duplicated_in_sweep"`
	StartedAt         string      `json:"started_at"`
	FinishedAt        string      `json:"finished_at"`
	PagesDetail       []PageEntry `json:"pages_detail"`
}

// PageEntry is one archived page as the manifest describes it.
type PageEntry struct {
	Sweep         int    `json:"sweep"`
	Page          int    `json:"page"`
	Rows          int    `json:"rows"`
	File          string `json:"file"`
	SHA256        string `json:"sha256"`
	Bytes         int64  `json:"bytes"`
	FetchedAt     string `json:"fetched_at"`
	TotalPages    int    `json:"total_pages"`
	TotalElements int    `json:"total_elements"`
	FirstContract *int64 `json:"first_contract"`
	LastContract  *int64 `json:"last_contract"`
}

// Trade is one contract exactly as the exchange published it.
//
// Numeric fields stay as json.Number: a contract amount must not make a round
// trip through float64 on its way to a NUMERIC column.
type Trade struct {
	ContractID       json.Number  `json:"contractId"`
	StockSymbol      string       `json:"stockSymbol"`
	SecurityName     string       `json:"securityName"`
	BusinessDate     string       `json:"businessDate"`
	BuyerMemberID    *json.Number `json:"buyerMemberId"`
	SellerMemberID   *json.Number `json:"sellerMemberId"`
	BuyerBrokerName  *string      `json:"buyerBrokerName"`
	SellerBrokerName *string      `json:"sellerBrokerName"`
	ContractQuantity json.Number  `json:"contractQuantity"`
	ContractRate     json.Number  `json:"contractRate"`
	ContractAmount   *json.Number `json:"contractAmount"`
	TradeBookID      *json.Number `json:"tradeBookId"`
	TradeTime        string       `json:"tradeTime"`
}

// Page is one parsed raw response.
type Page struct {
	Entry  PageEntry
	Trades []Trade
	Raw    []byte
}

// Date is a fully loaded and verified capture.
type Date struct {
	Manifest Manifest
	Dir      string
	Pages    []Page
	Check    Verification
	// Order is the deduplicated contract sequence, one entry per contract id.
	Order []Trade
}

// Verification is the evidence behind the completeness verdict.
type Verification struct {
	BusinessDate string `json:"business_date"`
	SessionID    string `json:"session_id"`

	PagesExpected int   `json:"pages_expected"`
	PagesPresent  int   `json:"pages_present"`
	MissingPages  []int `json:"missing_pages,omitempty"`

	RowsInPages        int `json:"rows_in_pages"`
	UniqueContracts    int `json:"unique_contracts"`
	DuplicateContracts int `json:"duplicate_contracts"`
	TotalElements      int `json:"total_elements"`
	MissingContracts   int `json:"missing_contracts"`

	Converged      bool `json:"converged"`
	DateMismatches int  `json:"date_mismatches"`
	DigestFailures int  `json:"digest_failures"`

	MissingTradeTime int `json:"missing_trade_time"`
	Microseconds     int `json:"microsecond_times"`
	Milliseconds     int `json:"millisecond_times"`
	SecondResolution int `json:"second_times"`

	TimestampPrecision string   `json:"timestamp_precision"`
	TimezoneBasis      string   `json:"timezone_basis"`
	Findings           []string `json:"findings"`
	Errors             []string `json:"errors"`
}

// Complete reports whether the archive proves it holds every contract the
// exchange reported for the session.
func (v Verification) Complete() bool {
	return len(v.Errors) == 0 &&
		v.MissingContracts == 0 &&
		v.MissingPages == nil &&
		v.DigestFailures == 0 &&
		v.DateMismatches == 0
}

// Load reads one date's archive, verifies every page against its recorded
// digest, and returns the deduplicated contract sequence.
//
// A missing or corrupt page is recorded and the load continues: a partial
// archive still has value, and refusing to read it would hide which pages are
// absent.
func Load(dir, date string) (*Date, error) {
	root := filepath.Join(dir, date)
	manifestPath := filepath.Join(root, "manifest.json")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest for %s: %w", date, err)
	}
	var man Manifest
	if err := json.Unmarshal(body, &man); err != nil {
		return nil, fmt.Errorf("parse manifest for %s: %w", date, err)
	}
	if man.BusinessDate == "" {
		man.BusinessDate = date
	}

	d := &Date{Manifest: man, Dir: root}
	check := Verification{
		BusinessDate:  man.BusinessDate,
		SessionID:     man.SessionID,
		PagesExpected: man.TotalPages,
		TotalElements: man.TotalElements,
		Converged:     man.Converged,
	}

	// Present pages are counted from the manifest entries that exist on disk,
	// not from the sweep's reported count: a truncated copy is a missing page.
	presentByPage := map[int]bool{}
	seen := map[string]int{} // contract id -> first page that carried it
	var order []Trade

	for _, sweep := range man.Sweeps {
		for _, entry := range sweep.PagesDetail {
			// Manifest paths are relative to the archive root, not to the date
			// directory. An older manifest that wrote them relative to the date is
			// still readable, so both are tried before a page is called missing.
			path := filepath.Join(dir, filepath.FromSlash(entry.File))
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				alt := filepath.Join(root, filepath.FromSlash(entry.File))
				if b, err := os.ReadFile(alt); err == nil {
					path, raw, readErr = alt, b, nil
				}
			}
			if readErr != nil {
				check.Errors = append(check.Errors,
					fmt.Sprintf("page %d (sweep %d) is listed in the manifest but missing on disk: %v",
						entry.Page, entry.Sweep, readErr))
				continue
			}
			if entry.SHA256 != "" {
				sum := sha256.Sum256(raw)
				if got := hex.EncodeToString(sum[:]); got != entry.SHA256 {
					check.DigestFailures++
					check.Errors = append(check.Errors,
						fmt.Sprintf("page %d (sweep %d) digest mismatch: manifest %s, file %s",
							entry.Page, entry.Sweep, entry.SHA256[:12], got[:12]))
					continue
				}
			}

			var doc struct {
				BusinessDate string  `json:"business_date"`
				Page         int     `json:"page"`
				TotalPages   int     `json:"total_pages"`
				Content      []Trade `json:"content"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				check.Errors = append(check.Errors,
					fmt.Sprintf("page %d (sweep %d) is not valid JSON: %v", entry.Page, entry.Sweep, err))
				continue
			}
			if doc.BusinessDate != "" && doc.BusinessDate != man.BusinessDate {
				check.DateMismatches++
				check.Errors = append(check.Errors,
					fmt.Sprintf("page %d (sweep %d) belongs to %s, not %s",
						entry.Page, entry.Sweep, doc.BusinessDate, man.BusinessDate))
			}

			presentByPage[entry.Page] = true
			check.RowsInPages += len(doc.Content)

			for _, t := range doc.Content {
				id := t.ContractID.String()
				if id == "" {
					check.Errors = append(check.Errors,
						fmt.Sprintf("page %d (sweep %d) has a contract row with no contractId",
							entry.Page, entry.Sweep))
					continue
				}
				if t.BusinessDate != "" && t.BusinessDate != man.BusinessDate {
					check.DateMismatches++
				}
				classifyTime(&check, t)
				if _, dup := seen[id]; dup {
					check.DuplicateContracts++
					continue
				}
				seen[id] = entry.Page
				order = append(order, t)
			}

			d.Pages = append(d.Pages, Page{Entry: entry, Trades: doc.Content, Raw: raw})
		}
	}

	check.PagesPresent = len(presentByPage)
	for page := 0; page < man.TotalPages; page++ {
		if !presentByPage[page] {
			check.MissingPages = append(check.MissingPages, page)
		}
	}
	sort.Ints(check.MissingPages)

	check.UniqueContracts = len(seen)
	check.MissingContracts = man.TotalElements - len(seen)
	if check.MissingContracts < 0 {
		// More contracts than the exchange reported is not "complete plus a
		// bonus": it means the page set spans more than one session.
		check.Errors = append(check.Errors, fmt.Sprintf(
			"archive holds %d contracts but the exchange reported %d for this session",
			len(seen), man.TotalElements))
		check.MissingContracts = 0
	}
	check.TimestampPrecision = precisionLabel(check)
	check.TimezoneBasis = "naive local time read as Asia/Kathmandu (UTC+05:45); stored as UTC"

	switch {
	case len(check.MissingPages) > 0:
		check.Errors = append(check.Errors, fmt.Sprintf(
			"%d of %d pages absent (first missing page %d)",
			len(check.MissingPages), man.TotalPages, check.MissingPages[0]))
	case check.MissingContracts > 0:
		check.Errors = append(check.Errors, fmt.Sprintf(
			"%d contracts missing: %d unique of %d reported (the page window moved while it was being read)",
			check.MissingContracts, len(seen), man.TotalElements))
	case !man.Converged:
		check.Findings = append(check.Findings,
			"all reported contracts are present, but the capture did not record convergence")
	}
	if check.DuplicateContracts > 0 {
		check.Findings = append(check.Findings, fmt.Sprintf(
			"%d duplicate contract rows across sweeps, collapsed by contract id",
			check.DuplicateContracts))
	}
	if check.MissingTradeTime > 0 {
		check.Findings = append(check.Findings, fmt.Sprintf(
			"%d contracts have no tradeTime; their executed_at stays null rather than being imputed",
			check.MissingTradeTime))
	}

	// Newest contract first, matching the exchange's own ordering, so the API's
	// keyset cursor walks the same direction as the source.
	sort.SliceStable(order, func(i, j int) bool {
		return compareNumeric(order[i].ContractID.String(), order[j].ContractID.String()) > 0
	})
	d.Order = order
	d.Check = check
	return d, nil
}

// AvailableDates lists the trading dates with a manifest, newest first.
func AvailableDates(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "manifest.json")); err != nil {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
}

func classifyTime(check *Verification, t Trade) {
	if strings.TrimSpace(t.TradeTime) == "" {
		check.MissingTradeTime++
		return
	}
	// Precision is measured from the fraction actually supplied, not assumed
	// from the column type that will store it.
	if i := strings.IndexByte(t.TradeTime, '.'); i >= 0 {
		frac := t.TradeTime[i+1:]
		if j := strings.IndexAny(frac, "+-Z"); j >= 0 {
			frac = frac[:j]
		}
		switch {
		case len(frac) >= 6:
			check.Microseconds++
		case len(frac) >= 3:
			check.Milliseconds++
		default:
			check.SecondResolution++
		}
		return
	}
	check.SecondResolution++
}

func precisionLabel(check Verification) string {
	total := check.Microseconds + check.Milliseconds + check.SecondResolution
	switch {
	case total == 0:
		return "not supplied"
	case check.Microseconds == total:
		return fmt.Sprintf("microsecond (6 fractional digits) throughout, %d contracts", total)
	case check.Milliseconds == total:
		return fmt.Sprintf("millisecond (3 fractional digits) throughout, %d contracts", total)
	case check.SecondResolution == total:
		return fmt.Sprintf("second resolution only, %d contracts", total)
	default:
		// The exchange does not publish one precision. Saying so is more useful
		// than a single word that describes none of the rows.
		return fmt.Sprintf("mixed: %s microsecond, %s millisecond, %s second-resolution",
			commaInt(check.Microseconds), commaInt(check.Milliseconds),
			commaInt(check.SecondResolution))
	}
}

func commaInt(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

// ExecuteTime converts a published trade time to UTC.
//
// The value is returned only when the exchange supplied one. A missing time is
// reported as absent; it is never replaced with the capture time, because a
// fabricated execution instant cannot be told apart from a real one later.
func ExecuteTime(raw string) (time.Time, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, false
	}
	// With an explicit offset the value is already unambiguous.
	for _, layout := range []string{time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	layouts := []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, NPT); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func compareNumeric(a, b string) int {
	ai, aerr := strconv.ParseInt(a, 10, 64)
	bi, berr := strconv.ParseInt(b, 10, 64)
	switch {
	case aerr == nil && berr == nil:
		switch {
		case ai < bi:
			return -1
		case ai > bi:
			return 1
		}
		return 0
	case len(a) != len(b):
		if len(a) < len(b) {
			return -1
		}
		return 1
	default:
		return strings.Compare(a, b)
	}
}
