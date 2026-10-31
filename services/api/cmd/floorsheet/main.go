// Command floorsheet verifies, ingests and reports on the trade-level archive.
//
// Usage:
//
//	go run ./cmd/floorsheet verify                  # verify every archived date
//	go run ./cmd/floorsheet verify -date 2026-10-29 -json
//	go run ./cmd/floorsheet ingest -date 2026-10-29  # one date, idempotent
//	go run ./cmd/floorsheet backfill                 # every archived date
//	go run ./cmd/floorsheet brokers -date 2026-10-29
//	go run ./cmd/floorsheet report                   # write the coverage report
//
// The exchange serves the trade tape only for the live session: the per-symbol
// date-filtered endpoint answers 403, and the bulk endpoint returns an empty
// sheet for any earlier date. There is therefore no date range to request. A
// backfill here means re-sweeping sessions already captured (to recover page
// drift) and folding each new session in as it is captured — never inventing
// history the source does not serve.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nepse/terminal/api/internal/config"
	"nepse/terminal/api/internal/floorsheet"
	"nepse/terminal/api/internal/ingestion"
	"nepse/terminal/api/internal/marketdata"
	"nepse/terminal/api/internal/repositories"
)

const sourceSlug = "floorsheet-pages"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "verify":
		err = runVerify(args)
	case "ingest":
		err = runIngest(args)
	case "backfill":
		err = runBackfill(args)
	case "brokers":
		err = runBrokers(args)
	case "report":
		err = runReport(args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `floorsheet — verify, ingest and report on the trade archive

  verify    [-date D] [-json]        check pagination, digests, contract counts
  ingest    [-date D] [-force]       fold one date into PostgreSQL
  backfill  [-from D] [-to D]        every archived date, idempotent
  brokers   [-date D] [-limit N]     rebuild and print broker summaries
  report    [-out PATH]              write the coverage report

Dates are YYYY-MM-DD. Exit status is non-zero when a date is not provably
complete, so a scheduler can treat "partial" as a failure.
`)
}

// ---------------------------------------------------------------- verify

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	date := fs.String("date", "", "trading date (default: every archived date)")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := pagesDir()
	if err != nil {
		return err
	}

	dates, err := selectDates(dir, *date)
	if err != nil {
		return err
	}
	if len(dates) == 0 {
		return fmt.Errorf("no archived floorsheet dates under %s", dir)
	}

	all := make([]*floorsheet.Date, 0, len(dates))
	complete := true
	for _, d := range dates {
		loaded, err := floorsheet.Load(dir, d)
		if err != nil {
			return err
		}
		all = append(all, loaded)
		if !loaded.Check.Complete() {
			complete = false
		}
	}

	if *asJSON {
		out := make([]floorsheet.Verification, 0, len(all))
		for _, d := range all {
			out = append(out, d.Check)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return err
		}
	} else {
		fmt.Printf("%-12s %7s %7s %9s %9s %9s %s\n",
			"DATE", "PAGES", "ROWS", "UNIQUE", "REPORTED", "MISSING", "VERDICT")
		for _, d := range all {
			c := d.Check
			verdict := "complete"
			if !c.Complete() {
				verdict = "INCOMPLETE"
			}
			fmt.Printf("%-12s %7d %7d %9d %9d %9d %s\n",
				c.BusinessDate, c.PagesPresent, c.RowsInPages, c.UniqueContracts,
				c.TotalElements, c.MissingContracts, verdict)
			if len(c.MissingPages) > 0 {
				fmt.Printf("    missing pages: %s\n", compactInts(c.MissingPages, 10))
			}
			for _, e := range c.Errors {
				fmt.Printf("    ! %s\n", e)
			}
			for _, f := range c.Findings {
				fmt.Printf("    - %s\n", f)
			}
			fmt.Printf("    timestamps: %s\n", c.TimestampPrecision)
			fmt.Printf("    basis: %s\n", c.TimezoneBasis)
		}
	}
	if !complete {
		return fmt.Errorf("at least one archived date is not provably complete")
	}
	return nil
}

// ---------------------------------------------------------------- ingest

func runIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	date := fs.String("date", "", "trading date (default: newest archived)")
	force := fs.Bool("force", false, "re-apply even when the archive digest is unchanged")
	noBrokers := fs.Bool("no-brokers", false, "skip the broker summary rebuild")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	st, cfg, err := open(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.Migrate(ctx); err != nil {
		return err
	}

	dir, err := pagesDir()
	if err != nil {
		return err
	}
	dates, err := selectDates(dir, *date)
	if err != nil {
		return err
	}
	if len(dates) == 0 {
		return fmt.Errorf("no archived floorsheet dates under %s", dir)
	}

	ing := &ingestion.Ingester{
		Pool:    st.Pool(),
		Archive: &ingestion.Store{Dir: cfg.ArchiveDir, Pool: st.Pool()},
		Logger:  logger(),
	}

	var results []runResult
	failed := false
	for _, d := range dates {
		res, err := ingestDate(ctx, st, ing, dir, d, *force, *noBrokers)
		if err != nil {
			return err
		}
		results = append(results, res)
		if res.Status == "failed" {
			failed = true
		}
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			return err
		}
	} else {
		for _, r := range results {
			fmt.Printf("%s  %-8s  pages %d/%d  %d contracts  written %d  unchanged %d  brokers %d\n",
				r.TradingDate, r.Status, r.PagesPresent, r.PagesExpected,
				r.UniqueContracts, r.RowsWritten, r.RowsUnchanged, r.Brokers)
			for _, note := range r.Notes {
				fmt.Printf("    - %s\n", note)
			}
		}
	}
	if failed {
		return fmt.Errorf("at least one date failed to ingest")
	}
	return nil
}

// ---------------------------------------------------------------- backfill

func runBackfill(args []string) error {
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	from := fs.String("from", "", "earliest date (inclusive)")
	to := fs.String("to", "", "latest date (inclusive)")
	force := fs.Bool("force", false, "re-apply every date")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	st, cfg, err := open(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.Migrate(ctx); err != nil {
		return err
	}

	dir, err := pagesDir()
	if err != nil {
		return err
	}
	dates, err := floorsheet.AvailableDates(dir)
	if err != nil {
		return err
	}
	if len(dates) == 0 {
		return fmt.Errorf("no archived floorsheet dates under %s", dir)
	}

	ing := &ingestion.Ingester{
		Pool:    st.Pool(),
		Archive: &ingestion.Store{Dir: cfg.ArchiveDir, Pool: st.Pool()},
		Logger:  logger(),
	}

	var results []runResult
	for _, d := range dates {
		if *from != "" && d < *from {
			continue
		}
		if *to != "" && d > *to {
			continue
		}
		res, err := ingestDate(ctx, st, ing, dir, d, *force, false)
		if err != nil {
			return err
		}
		results = append(results, res)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	if len(results) == 0 {
		fmt.Println("no archived date matched the range")
		return nil
	}
	fmt.Printf("%-12s %-9s %10s %10s %8s %8s\n",
		"DATE", "STATUS", "REPORTED", "CONTRACTS", "WRITTEN", "BROKERS")
	for _, r := range results {
		fmt.Printf("%-12s %-9s %10d %10d %8d %8d\n",
			r.TradingDate, r.Status, r.TotalElements, r.UniqueContracts,
			r.RowsWritten, r.Brokers)
	}
	return nil
}

// ---------------------------------------------------------------- brokers

func runBrokers(args []string) error {
	fs := flag.NewFlagSet("brokers", flag.ContinueOnError)
	date := fs.String("date", "", "trading date (default: newest session)")
	limit := fs.Int("limit", 25, "rows to print")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	st, _, err := open(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.Migrate(ctx); err != nil {
		return err
	}

	d := *date
	if d == "" {
		cov, err := st.FloorsheetCoverageReport(ctx, 30)
		if err != nil {
			return err
		}
		if cov.LatestPresent == "" {
			return fmt.Errorf("no trades stored")
		}
		d = cov.LatestPresent
	}

	coverage, err := st.RebuildBrokerSummaries(ctx, d)
	if err != nil {
		return err
	}
	rows, err := st.BrokerSummaries(ctx, d, *limit)
	if err != nil {
		return err
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"coverage": coverage, "brokers": rows})
	}

	fmt.Printf("%s: %d contracts, %d attributable to a member (%.1f%%), %d brokers\n\n",
		d, coverage.Contracts, coverage.Attributed,
		100*ratio(coverage.Attributed, coverage.Contracts), coverage.Brokers)
	fmt.Printf("%-10s %10s %10s %18s %18s %18s\n",
		"BROKER", "BUY CNT", "SELL CNT", "BUY TURNOVER", "SELL TURNOVER", "NET")
	for _, b := range rows {
		fmt.Printf("%-10s %10d %10d %18s %18s %18s\n",
			b.BrokerCode, b.BuyContracts, b.SellContracts,
			buy(b.BuyTurnover), b.SellTurnover, b.NetTurnover)
	}
	fmt.Printf("\n%d of %d contracts carry no member id and are excluded from every broker total.\n",
		coverage.Unattributed, coverage.Contracts)
	return nil
}

// ---------------------------------------------------------------- report

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	out := fs.String("out", "", "output path (default: docs/FLOORSHEET_REPORT.md)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	st, cfg, err := open(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.Migrate(ctx); err != nil {
		return err
	}

	path := *out
	if path == "" {
		path = filepath.Join(cfg.ReportDir, "FLOORSHEET_REPORT.md")
	}

	dir, err := pagesDir()
	if err != nil {
		return err
	}
	body, err := buildReport(ctx, st, dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	return nil
}

// ---------------------------------------------------------------- plumbing

type runResult struct {
	TradingDate     string   `json:"trading_date"`
	Status          string   `json:"status"`
	PagesExpected   int      `json:"pages_expected"`
	PagesPresent    int      `json:"pages_present"`
	RowsInPages     int      `json:"rows_in_pages"`
	UniqueContracts int      `json:"unique_contracts"`
	TotalElements   int      `json:"total_elements"`
	Missing         int      `json:"missing_contracts"`
	Converged       bool     `json:"converged"`
	RowsWritten     int      `json:"rows_written"`
	RowsUnchanged   int      `json:"rows_unchanged"`
	RowsRejected    int      `json:"rows_rejected"`
	Brokers         int      `json:"brokers"`
	DuplicateRows   int      `json:"duplicate_contracts"`
	Notes           []string `json:"notes,omitempty"`
}

func ingestDate(ctx context.Context, st *repositories.Store, ing *ingestion.Ingester,
	dir, date string, force, noBrokers bool) (runResult, error) {
	started := time.Now().UTC()

	loaded, err := floorsheet.Load(dir, date)
	if err != nil {
		return runResult{}, err
	}
	c := loaded.Check

	// The page ledger is written before ingestion: if the load then fails, the
	// receipt still shows which bytes the attempt read.
	recs := make([]repositories.PageRecord, 0, len(loaded.Pages))
	for _, p := range loaded.Pages {
		var fetched *time.Time
		if t, ok := floorsheet.ExecuteTime(p.Entry.FetchedAt); ok {
			fetched = &t
		}
		recs = append(recs, repositories.PageRecord{
			Sweep: p.Entry.Sweep, Page: p.Entry.Page, Rows: p.Entry.Rows,
			FirstContract: p.Entry.FirstContract, LastContract: p.Entry.LastContract,
			TotalPages: p.Entry.TotalPages, TotalElements: p.Entry.TotalElements,
			SHA256: p.Entry.SHA256, RelPath: p.Entry.File, Bytes: p.Entry.Bytes,
			FetchedAt: fetched,
		})
	}
	if _, err := st.RecordFloorsheetPages(ctx, date, loaded.Manifest.SessionID, recs); err != nil {
		return runResult{}, err
	}

	adapter := &pageAdapter{date: loaded}
	sum, err := ing.Run(ctx, []marketdata.Adapter{adapter}, ingestion.Options{
		Streams: []string{marketdata.StreamFloorsheet},
		Sources: []string{sourceSlug},
		Force:   force,
		Trigger: "floorsheet-cli",
	})
	if err != nil {
		return runResult{}, err
	}

	res := runResult{
		TradingDate:     date,
		PagesExpected:   c.PagesExpected,
		PagesPresent:    c.PagesPresent,
		RowsInPages:     c.RowsInPages,
		UniqueContracts: c.UniqueContracts,
		TotalElements:   c.TotalElements,
		Missing:         c.MissingContracts,
		Converged:       c.Converged,
		DuplicateRows:   c.DuplicateContracts,
		Status:          "success",
		Notes:           append([]string{}, c.Findings...),
	}
	for _, r := range sum.Results {
		res.RowsWritten += r.RowsWritten
		res.RowsUnchanged += r.RowsUnchanged
		res.RowsRejected += r.RowsRejected
		if r.Error != "" {
			res.Notes = append(res.Notes, r.Error)
			res.Status = "failed"
		}
		if r.Skipped {
			res.Notes = append(res.Notes, "archive digest unchanged; rows already applied")
		}
	}

	if !noBrokers && res.Status != "failed" {
		cov, err := st.RebuildBrokerSummaries(ctx, date)
		if err != nil {
			return res, err
		}
		res.Brokers = cov.Brokers
		if cov.Unattributed > 0 {
			res.Notes = append(res.Notes, fmt.Sprintf(
				"%d of %d contracts carry no member id; excluded from broker totals",
				cov.Unattributed, cov.Contracts))
		}
	}

	// A date is only "success" when the archive proves it holds every reported
	// contract. Writing fewer rows than the exchange counted is a partial load,
	// not a smaller market.
	if res.Status != "failed" && !c.Complete() {
		res.Status = "partial"
	}

	finished := time.Now().UTC()
	_, err = st.SaveFloorsheetRun(ctx, &repositories.FloorsheetRun{
		TradingDate:        date,
		Source:             sourceSlug,
		Status:             res.Status,
		PagesExpected:      c.PagesExpected,
		PagesPresent:       c.PagesPresent,
		PagesMissing:       len(c.MissingPages),
		RowsInPages:        c.RowsInPages,
		UniqueContracts:    c.UniqueContracts,
		DuplicateContracts: c.DuplicateContracts,
		TotalElements:      int64(c.TotalElements),
		MissingContracts:   c.MissingContracts,
		Converged:          c.Converged,
		DateMismatches:     c.DateMismatches,
		DigestFailures:     c.DigestFailures,
		RowsWritten:        res.RowsWritten,
		RowsUnchanged:      res.RowsUnchanged,
		TimestampPrecision: c.TimestampPrecision,
		TimezoneBasis:      c.TimezoneBasis,
		StartedAt:          started,
		FinishedAt:         &finished,
	})
	if err != nil {
		return res, err
	}
	return res, nil
}

// pageAdapter feeds the verified, deduplicated contract sequence to the shared
// ingestion pipeline, so trades from the page archive go through the same
// staging, validation and merge as every other stream.
type pageAdapter struct {
	date *floorsheet.Date
}

func (a *pageAdapter) Slug() string { return sourceSlug }

func (a *pageAdapter) Authorization() marketdata.Authorization {
	return marketdata.Authorization{
		Mode:           marketdata.ModeOperatorCaptured,
		LicenceClass:   marketdata.LicenceUnverified,
		Redistribution: marketdata.RedistributeNo,
		Note:           "Contract tape captured from the exchange's own bulk endpoint and archived page by page.",
	}
}

func (a *pageAdapter) Streams() []string { return []string{marketdata.StreamFloorsheet} }

func (a *pageAdapter) Each(ctx context.Context, stream string, fn func(map[string]any) error) (marketdata.Batch, error) {
	batch := marketdata.Batch{
		Stream:     stream,
		CapturedAt: time.Now().UTC(),
		Meta: map[string]any{
			"adapter":        sourceSlug,
			"trading_date":   a.date.Manifest.BusinessDate,
			"session_id":     a.date.Manifest.SessionID,
			"pages":          a.date.Check.PagesPresent,
			"total_pages":    a.date.Manifest.TotalPages,
			"total_elements": a.date.Manifest.TotalElements,
			"converged":      a.date.Manifest.Converged,
		},
	}
	if stream != marketdata.StreamFloorsheet {
		return batch, fmt.Errorf("page adapter does not provide stream %q", stream)
	}

	for _, t := range a.date.Order {
		row, err := tradeRow(t)
		if err != nil {
			return batch, err
		}
		if err := fn(row); err != nil {
			return batch, err
		}
		batch.Rows++
		batch.Cursor = t.ContractID.String()
	}
	return batch, nil
}

// tradeRow maps one published contract onto the normaliser's field names.
//
// The published tradeTime has no zone marker but is Nepal local time; it is
// converted here so both adapters agree that trade_time is UTC. A missing value
// stays absent, which leaves executed_at null instead of inventing an instant.
func tradeRow(t floorsheet.Trade) (map[string]any, error) {
	row := map[string]any{
		"contract_id":   t.ContractID.String(),
		"business_date": t.BusinessDate,
		"symbol":        t.StockSymbol,
		"quantity":      t.ContractQuantity.String(),
		"rate":          t.ContractRate.String(),
	}
	if t.ContractAmount != nil && t.ContractAmount.String() != "" {
		row["amount"] = t.ContractAmount.String()
	}
	if t.BuyerMemberID != nil {
		row["buyer_member"] = t.BuyerMemberID.String()
	}
	if t.SellerMemberID != nil {
		row["seller_member"] = t.SellerMemberID.String()
	}
	if t.BuyerBrokerName != nil && strings.TrimSpace(*t.BuyerBrokerName) != "" {
		row["buyer_broker"] = *t.BuyerBrokerName
	} else if t.BuyerMemberID != nil {
		row["buyer_broker"] = t.BuyerMemberID.String()
	}
	if t.SellerBrokerName != nil && strings.TrimSpace(*t.SellerBrokerName) != "" {
		row["seller_broker"] = *t.SellerBrokerName
	} else if t.SellerMemberID != nil {
		row["seller_broker"] = t.SellerMemberID.String()
	}
	if ts, ok := floorsheet.ExecuteTime(t.TradeTime); ok {
		row["trade_time"] = ts.Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	row["raw"] = string(raw)
	return row, nil
}

func open(ctx context.Context) (*repositories.Store, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	st, err := repositories.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	return st, cfg, nil
}

func pagesDir() (string, error) {
	if v := os.Getenv("FLOORSHEET_PAGES_DIR"); v != "" {
		return v, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.FloorsheetPagesDir, nil
}

func selectDates(dir, date string) ([]string, error) {
	if date != "" {
		if _, err := os.Stat(filepath.Join(dir, date, "manifest.json")); err != nil {
			return nil, fmt.Errorf("no archive for %s under %s: %w", date, dir, err)
		}
		return []string{date}, nil
	}
	return floorsheet.AvailableDates(dir)
}

func logger() *slog.Logger {
	l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(l)
	return l
}

func compactInts(v []int, max int) string {
	if len(v) <= max {
		return strings.Trim(strings.Join(strings.Fields(fmt.Sprint(v)), " "), "[]")
	}
	return fmt.Sprintf("%v … (%d total)", v[:max], len(v))
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func buy(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// sortedUnique keeps report ordering deterministic without depending on map
// iteration order.
func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
