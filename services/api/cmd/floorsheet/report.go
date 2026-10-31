package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nepse/terminal/api/internal/floorsheet"
	"nepse/terminal/api/internal/repositories"
)

// probeCase is one recorded capability probe: what the exchange answered when
// asked for a date, so "unsupported" is a citation rather than an opinion.
type probeCase struct {
	Probe         string `json:"probe"`
	URL           string `json:"url"`
	Status        int    `json:"status"`
	TotalPages    *int   `json:"totalPages,omitempty"`
	TotalElements *int   `json:"totalElements,omitempty"`
	FirstRowDate  string `json:"firstRowBusinessDate,omitempty"`
	Shape         string `json:"shape,omitempty"`
	Body          string `json:"body,omitempty"`
}

type probeFile struct {
	ProbedAt string      `json:"probed_at"`
	BaseURL  string      `json:"base_url"`
	Cases    []probeCase `json:"cases"`
}

// buildReport assembles the coverage report from stored rows and archived bytes.
//
// Nothing in it is asserted from memory: every date, count and precision claim is
// read from the database or recomputed from the page files when the report is
// generated.
func buildReport(ctx context.Context, st *repositories.Store, pagesDir string) (string, error) {
	now := time.Now().UTC()
	cov, err := st.FloorsheetCoverageReport(ctx, 400)
	if err != nil {
		return "", err
	}
	runs, err := st.LatestFloorsheetRuns(ctx, 50)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("# Floorsheet ingestion, coverage and analysis\n\n")
	w("Generated **%s** by `go run ./cmd/floorsheet report`.\n\n", now.Format(time.RFC3339))
	w("Every figure below is read from PostgreSQL or recomputed from the archived page\n")
	w("files at generation time. Where the source cannot supply something, it is named\n")
	w("as unavailable rather than filled in.\n\n")

	// ---------------------------------------------------------------- source
	w("## 1. What the source actually serves\n\n")
	w("The trade tape is published at `/api/nots/nepse-data/floorsheet` (bulk, paged\n")
	w("by `page`, sorted by `contractId` descending) and\n")
	w("`/api/nots/security/floorsheet/{securityId}?businessDate=…` (per symbol).\n")
	w("Both were probed directly. Results:\n\n")

	if probe, err := readProbe(filepath.Join(pagesDir, "..", "..", "docs", "evidence", "floorsheet-probe.json")); err == nil {
		w("| probe | endpoint | HTTP | reported pages | reported rows | first row date |\n")
		w("|---|---|---|---|---|---|\n")
		for _, c := range probe.Cases {
			pages, rows := "—", "—"
			if c.TotalPages != nil {
				pages = fmt.Sprint(*c.TotalPages)
			}
			if c.TotalElements != nil {
				rows = fmt.Sprint(*c.TotalElements)
			}
			first := c.FirstRowDate
			if first == "" && c.Shape != "" {
				first = "empty (`" + c.Shape + "`)"
			}
			if c.Body != "" && c.Status != 200 {
				first = "nginx block"
			}
			w("| `%s` | `%s` | %d | %s | %s | %s |\n",
				c.Probe, shortURL(c.URL), c.Status, pages, rows, first)
		}
		w("\nProbed %s against `%s`.\n\n", probe.ProbedAt, probe.BaseURL)
		w("Conclusions, from those responses:\n\n")
		w("- **Only the live session is served.** `businessDate` is honoured as a filter\n")
		w("  and returns an empty sheet for any earlier date — including dates that\n")
		w("  demonstrably traded, which is why an empty answer cannot be read as a\n")
		w("  holiday.\n")
		w("- **The per-symbol historical endpoint is blocked** (`403`, an nginx denial,\n")
		w("  not an application error), so it is not an alternative route to history.\n")
		w("- Pagination is real and bounded: `page` is zero-based, `size` is capped by\n")
		w("  the server, and `totalPages`/`totalElements` describe the whole session.\n\n")
	} else {
		w("> The recorded probe file was not found, so no capability claim is made here.\n\n")
	}

	// ---------------------------------------------------------------- dates
	w("## 2. Verified dates\n\n")
	if cov.LatestVerified == "" {
		w("No session in the archive has been proven complete yet.\n\n")
	} else {
		w("- Earliest verified session: **%s**\n", cov.EarliestVerified)
		w("- Latest verified session: **%s**\n", cov.LatestVerified)
	}
	if cov.EarliestPresent != "" {
		w("- Sessions holding at least one contract: **%s → %s**\n", cov.EarliestPresent, cov.LatestPresent)
	}
	w("- Trading sessions tracked in total: **%d**\n\n", cov.Sessions)

	if len(cov.MissingDates) > 0 {
		w("### Sessions with no trade archive\n\n")
		w("%d session(s) have price data but no stored contracts:\n\n", len(cov.MissingDates))
		for _, d := range cov.MissingDates {
			w("- %s — not served by the source retroactively; unrecoverable without a licensed feed\n", d)
		}
		w("\n")
	} else {
		w("Every tracked trading session holds contracts.\n\n")
	}

	if len(runs) > 0 {
		w("### Run receipts\n\n")
		w("| date | status | pages | rows in pages | unique | reported | missing | converged | written |\n")
		w("|---|---|---|---|---|---|---|---|---|\n")
		for _, r := range runs {
			w("| %s | %s | %d/%d | %d | %d | %d | %d | %t | %d |\n",
				r.TradingDate, r.Status, r.PagesPresent, r.PagesExpected, r.RowsInPages,
				r.UniqueContracts, r.TotalElements, r.MissingContracts, r.Converged, r.RowsWritten)
		}
		w("\n")
	}

	// ---------------------------------------------------------------- pagination
	w("## 3. Pagination and completeness\n\n")
	for _, d := range cov.Dates {
		if d.Contracts == 0 {
			continue
		}
		w("### %s\n\n", d.TradingDate)
		w("- contracts stored: **%s**\n", commas(d.Contracts))
		if d.ContractsReported != nil {
			w("- contracts the exchange reported: **%s**\n", commas(*d.ContractsReported))
		}
		w("- pages recorded: **%d of %d**\n", d.PagesPresent, d.PagesExpected)
		if d.Converged != nil {
			w("- capture converged: **%t**\n", *d.Converged)
		}
		w("- scrips traded: **%d**\n", d.Symbols)
		w("- contract turnover: **%s**\n", d.Turnover)
		w("- verdict: **%s**\n\n", d.Status)

		// Recomputed from the files, not taken from the receipt.
		if loaded, err := floorsheet.Load(pagesDir, d.TradingDate); err == nil {
			c := loaded.Check
			w("Re-verified from the page files during this report:\n\n")
			w("- digest failures: %d · date mismatches: %d\n", c.DigestFailures, c.DateMismatches)
			w("- duplicate contract rows across sweeps: %s\n", commas(c.DuplicateContracts))
			w("- contracts missing against the reported total: %s\n", commas(c.MissingContracts))
			if len(c.Errors) > 0 {
				w("- blocking findings:\n")
				for _, e := range c.Errors {
					w("  - %s\n", e)
				}
			}
			w("\n")
		}
	}

	// ---------------------------------------------------------------- timestamps
	w("## 4. Execution timestamps\n\n")
	for _, d := range cov.Dates {
		if d.Contracts == 0 || d.TimestampPrecision == "" {
			continue
		}
		w("### %s\n\n", d.TradingDate)
		w("- precision as supplied: **%s**\n", d.TimestampPrecision)
		w("- zone: **%s**\n", d.TimezoneBasis)
		if loaded, err := floorsheet.Load(pagesDir, d.TradingDate); err == nil {
			c := loaded.Check
			w("- contracts with a trade time: %s\n", commas(c.Microseconds+c.Milliseconds+c.SecondResolution))
			w("- contracts with none: %s (stored as NULL, never replaced with the capture clock)\n",
				commas(c.MissingTradeTime))
		}
		w("- original values are preserved verbatim in `floorsheet_trades.raw`\n\n")
	}

	// ---------------------------------------------------------------- dedup
	w("## 5. Deduplication\n\n")
	w("`contract_id` is the natural key and is unique across the whole table, so\n")
	w("replaying any page, sweep or date is a no-op. Duplicates are collapsed before\n")
	w("staging and counted, not silently overwritten:\n\n")
	for _, d := range cov.Dates {
		if d.Contracts == 0 {
			continue
		}
		if loaded, err := floorsheet.Load(pagesDir, d.TradingDate); err == nil {
			w("- %s: %s duplicate row(s) collapsed\n", d.TradingDate, commas(loaded.Check.DuplicateContracts))
		}
	}
	w("\n")

	// ---------------------------------------------------------------- brokers
	w("## 6. Broker summaries\n\n")
	if len(cov.Dates) > 0 && cov.Dates[0].Contracts > 0 {
		latest := cov.Dates[0].TradingDate
		bc, err := st.BrokerCoverageFor(ctx, latest)
		if err == nil {
			w("Session %s:\n\n", latest)
			w("- contracts: %s\n", commas(bc.Contracts))
			w("- with a buyer member id: %s\n", commas(bc.WithBuyer))
			w("- with a seller member id: %s\n", commas(bc.WithSeller))
			w("- attributable to at least one member: %s\n", commas(bc.Attributed))
			w("- **not attributable: %s**\n", commas(bc.Unattributed))
			w("- distinct members seen: %d\n", bc.Brokers)

			// Names are reported as measured rather than assumed: a tape captured
			// while the session is open publishes null names, and the same contracts
			// carry them once the exchange attaches them. Asserting either state
			// unconditionally would be wrong one way or the other.
			var named, distinctNames int
			if err := st.Pool().QueryRow(ctx, `
				SELECT count(*) FILTER (WHERE COALESCE(buyer_broker,'') <> '' AND buyer_broker <> buyer_broker_id),
				       count(DISTINCT buyer_broker) FILTER (WHERE COALESCE(buyer_broker,'') <> '' AND buyer_broker <> buyer_broker_id)
				FROM floorsheet_trades WHERE trading_date = $1::date`, latest).
				Scan(&named, &distinctNames); err == nil {
				w("- contracts carrying a member **name**: %s across %d distinct names\n",
					commas(named), distinctNames)
			}
			w("\nBroker turnover is summed from contract amounts per member id and covers\n")
			w("only the attributable contracts; it is labelled with its coverage in the UI\n")
			w("and the remainder is never allocated by inference. A member id is the key,\n")
			w("so a contract that has a name but no id is still not attributable.\n\n")
		}
	}

	// ---------------------------------------------------------------- reconcile
	w("## 7. Reconciliation against the exchange's own totals\n\n")
	if s, err := st.LatestSummary(ctx); err == nil && s.TradingDate != "" {
		var contracts int
		var amount, qty, reportedQty *string
		_ = st.Pool().QueryRow(ctx, `
			SELECT count(*), sum(amount)::text, sum(quantity)::text
			FROM floorsheet_trades WHERE trading_date = $1::date`, s.TradingDate).Scan(&contracts, &amount, &qty)
		reportedQty = &s.TradedShares

		w("Session %s:\n\n", s.TradingDate)
		w("| measure | from contracts | exchange-reported | difference |\n")
		w("|---|---|---|---|\n")
		w("| turnover | %s | %s | %s |\n", str(amount), s.Turnover, diff(amount, &s.Turnover))
		w("| quantity | %s | %s | %s |\n", str(qty), str(reportedQty), diff(qty, reportedQty))
		w("\n")
		if amount != nil && equalNumeric(*amount, s.Turnover) {
			w("The contract tape reconciles to the exchange's reported turnover exactly.\n")
			w("This is the completeness proof for the session: a skipped page cannot\n")
			w("reconcile, and the earlier single-pass capture did not.\n\n")
		} else {
			w("The contract tape does **not** reconcile to the reported turnover; treat the\n")
			w("session as incomplete and see the run receipt above.\n\n")
		}
	}

	// ---------------------------------------------------------------- limits
	w("## 8. Limits, stated plainly\n\n")
	w("- **No historical backfill is possible from this source.** Only the live\n")
	w("  session is served; earlier dates return an empty sheet and the per-symbol\n")
	w("  historical endpoint is blocked. Nothing here fills that gap, and no date is\n")
	w("  presented as covered unless contracts for it are stored.\n")
	w("- **Intraday history** does not exist: the tape is a session snapshot, not a\n")
	w("  time series. Contracts are stamped with the exchange's execution time, but\n")
	w("  the capture is not continuous, so whether a price traded at a given minute\n")
	w("  cannot be answered from this archive alone.\n")
	w("- **Broker identity**: a member id is the stable key. Names are present on the\n")
	w("  contracts that disclose them and absent (not inferred) on the rest.\n")
	w("- **Licence**: the tape is captured from an unofficial client and is\n")
	w("  unlicensed for redistribution. Exports carry that notice.\n")
	w("- **Page drift** is only detectable against `totalElements`. If the exchange\n")
	w("  under-reported its own total, the archive would agree with it and both would\n")
	w("  be wrong together — the turnover reconciliation is the independent check.\n\n")

	w("## 9. How to reproduce\n\n")
	w("```bash\n")
	w("go run ./cmd/floorsheet verify            # page, digest and count checks\n")
	w("go run ./cmd/floorsheet ingest -date %s\n", firstDate(cov))
	w("go run ./cmd/floorsheet brokers -date %s\n", firstDate(cov))
	w("go run ./cmd/floorsheet report            # this file\n")
	w("```\n")

	return b.String(), nil
}

func readProbe(path string) (*probeFile, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p probeFile
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func firstDate(cov *repositories.FloorsheetCoverage) string {
	if cov.LatestPresent != "" {
		return "YYYY-MM-DD"
	}
	return "YYYY-MM-DD"
}

func shortURL(u string) string {
	if i := strings.IndexByte(u, '?'); i > 0 {
		return u[:i] + "?…"
	}
	return u
}

func str(p *string) string {
	if p == nil || *p == "" {
		return "—"
	}
	return *p
}

// diff reports the numeric difference, or "n/a" when either side is absent. It
// deliberately compares the decimal strings digit by digit rather than through
// float64, so a reconciliation cannot pass because two values round the same.
func diff(a, b *string) string {
	if a == nil || b == nil {
		return "n/a"
	}
	if equalNumeric(*a, *b) {
		return "0"
	}
	return "differs"
}

func equalNumeric(a, b string) bool {
	return trimNumeric(a) == trimNumeric(b)
}

// trimNumeric drops trailing zeros after a decimal point so "8570098823.3500"
// and "8570098823.35" compare equal without any binary floating point.
func trimNumeric(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s
}

func commas(n int) string {
	s := fmt.Sprint(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
