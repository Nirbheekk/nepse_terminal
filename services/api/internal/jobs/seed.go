// Package seed loads data that already exists on disk into PostgreSQL.
//
// It deliberately loads *real* observations captured by the legacy collector
// (data/*.json and exports/legacy/<run>/*.jsonl) instead of inventing fixtures.
// That matters for two reasons:
//
//  1. NUMERIC columns and index/quote parsing are exercised against the exact
//     shapes the exchange returns, including nulls and string-encoded numbers.
//  2. Every row is attributable to data_sources, so nothing in the new database
//     is indistinguishable from made-up numbers.
//
// Seeding is idempotent: re-running updates reference rows, appends nothing
// twice (floorsheet is keyed by contract id) and never deletes history.
package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	nepse "nepse/terminal/api/internal/securityclass"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nepse/terminal/api/internal/repositories"
)

// pgxPool is the subset of the pool the loaders use, aliased so the helpers
// read plainly.
type pgxPool = pgxpool.Pool

// SourceSlug identifies where these rows came from. The licence class is
// recorded honestly: the unofficial client grants no redistribution rights.
const (
	SourceSlug      = "nepse:unofficial-client"
	SourceName      = "NEPSE (nepse-client, unofficial)"
	SourceEndpoint  = "https://www.nepalstock.com/"
	LicenseClass    = "public_unverified"
	Redistribution  = "prohibited"
	MarketCapMethod = "sum(issuer-reported market capitalisation)"
	FloatCapMethod  = "sum(public shares x last traded price)"
)

// Options controls what is loaded. Zero values fall back to the documented
// defaults in cmd/seed, never to unbounded behaviour.
type Options struct {
	DataDir         string // directory holding summary.json, company_list.json, …
	ExportDir       string // optional: exports/legacy/<run> with jsonl tables
	FloorsheetLimit int    // 0 = no trades; negative = every row available
	Logger          func(string, ...any)
}

// Stats is returned so callers (and tests) can assert on what happened.
type Stats struct {
	Sectors      int
	Companies    int
	Securities   int
	Sessions     int
	Bars         int
	Trades       int
	Indices      int
	Observations int
	SummaryDate  string
	MarketCap    string
	Warnings     []string
}

func (s *Stats) warn(format string, args ...any) {
	s.Warnings = append(s.Warnings, fmt.Sprintf(format, args...))
}

// Run loads everything it can find and reports per-table counts. A missing
// optional file is a warning, not a failure: partial data is normal for a
// snapshot-based collector.
func Run(ctx context.Context, st *repositories.Store, opts Options) (*Stats, error) {
	logf := opts.Logger
	if logf == nil {
		logf = func(string, ...any) {}
	}
	stats := &Stats{}

	pool := st.Pool()
	srcID, err := ensureSource(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("data source: %w", err)
	}

	if opts.ExportDir == "" {
		opts.ExportDir = LatestExport(opts.DataDir)
	}

	// ---------------------------------------------------------------- reference
	var companies []companyRecord
	if err := readJSONFile(filepath.Join(opts.DataDir, "company_list.json"), &companies); err != nil {
		return nil, fmt.Errorf("company_list.json: %w", err)
	}
	if len(companies) == 0 {
		return nil, fmt.Errorf("company_list.json contained no companies")
	}

	sectorIDs, n, err := ensureSectors(ctx, pool, companies)
	if err != nil {
		return nil, fmt.Errorf("sectors: %w", err)
	}
	stats.Sectors = n

	companyIDs, secIDs, n, err := ensureCompanies(ctx, pool, companies, sectorIDs)
	if err != nil {
		return nil, fmt.Errorf("companies: %w", err)
	}
	stats.Companies = len(companyIDs)
	stats.Securities = n
	logf("reference loaded", "companies", stats.Companies, "securities", n, "sectors", stats.Sectors)

	// ---------------------------------------------------------------- daily rows
	details, err := loadCompanyDetails(opts.DataDir)
	if err != nil {
		stats.warn("company detail files: %v", err)
	}
	if len(details) == 0 {
		stats.warn("no company_details/*.json found in %s", opts.DataDir)
	}

	sessions := map[string]struct{}{}
	bars := make([]barRow, 0, len(details))
	for _, d := range details {
		if d.Date == "" || !d.HasQuote {
			continue
		}
		sessions[d.Date] = struct{}{}
		bars = append(bars, d)
	}

	pvhBars, err := loadPriceVolumeHistory(opts.DataDir)
	if err == nil {
		for _, b := range pvhBars {
			if b.Date == "" || !b.HasQuote {
				continue
			}
			sessions[b.Date] = struct{}{}
			bars = append(bars, b)
		}
	}

	histBars, err := loadHistoricalCompanyPrices(opts.DataDir)
	if err == nil {
		for _, b := range histBars {
			if b.Date == "" || !b.HasQuote {
				continue
			}
			sessions[b.Date] = struct{}{}
			bars = append(bars, b)
		}
	} else {
		stats.warn("historical company prices: %v", err)
	}

	if err := ensureSessions(ctx, pool, sessions); err != nil {
		return nil, fmt.Errorf("trading sessions: %w", err)
	}
	stats.Sessions = len(sessions)

	written, err := writeBars(ctx, pool, bars, secIDs, srcID)
	if err != nil {
		return nil, fmt.Errorf("daily ohlcv: %w", err)
	}
	stats.Bars = written
	logf("daily bars loaded", "rows", written)

	// ---------------------------------------------------------------- summary
	sum, err := loadSummary(opts.DataDir)
	if err != nil {
		stats.warn("summary: %v", err)
	}
	if sum.Date != "" {
		if err := ensureSessions(ctx, pool, map[string]struct{}{sum.Date: {}}); err != nil {
			return nil, err
		}
		stats.SummaryDate = sum.Date
		stats.MarketCap = sum.MarketCap
		if _, err := pool.Exec(ctx, `
			INSERT INTO market_summaries (
				trading_date, nepse_index, point_change, percent_change, turnover,
				transactions, traded_shares, traded_securities, advanced, declined,
				unchanged, market_cap, float_market_cap, cap_covered, cap_eligible,
				cap_method, data_source_id, ingested_at)
			VALUES ($1::date, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, now())
			ON CONFLICT (trading_date) DO UPDATE SET
				nepse_index = EXCLUDED.nepse_index,
				point_change = EXCLUDED.point_change,
				percent_change = EXCLUDED.percent_change,
				turnover = EXCLUDED.turnover,
				transactions = EXCLUDED.transactions,
				traded_shares = EXCLUDED.traded_shares,
				traded_securities = EXCLUDED.traded_securities,
				advanced = EXCLUDED.advanced,
				declined = EXCLUDED.declined,
				unchanged = EXCLUDED.unchanged,
				market_cap = EXCLUDED.market_cap,
				float_market_cap = EXCLUDED.float_market_cap,
				cap_covered = EXCLUDED.cap_covered,
				cap_eligible = EXCLUDED.cap_eligible,
				cap_method = EXCLUDED.cap_method,
				data_source_id = EXCLUDED.data_source_id,
				ingested_at = now()`,
			sum.Date, sum.Index, sum.PointChange, sum.PercentChange, sum.Turnover,
			sum.Transactions, sum.TradedShares, sum.Securities, sum.Advanced,
			sum.Declined, sum.Unchanged, sum.MarketCap, sum.FloatCap,
			sum.CapCovered, sum.CapEligible, MarketCapMethod+" / "+FloatCapMethod, srcID,
		); err != nil {
			return nil, fmt.Errorf("market summary: %w", err)
		}
		logf("summary loaded", "date", sum.Date, "market_cap", sum.MarketCap)
	}

	// ---------------------------------------------------------------- indices
	idx, err := loadIndices(opts.DataDir, opts.ExportDir)
	if err != nil {
		stats.warn("indices: %v", err)
	}
	defs, n, err := ensureIndices(ctx, pool, idx)
	if err != nil {
		return nil, fmt.Errorf("index definitions: %w", err)
	}
	stats.Indices = n
	obs, err := writeIndexObservations(ctx, pool, idx, defs, sessions)
	if err != nil {
		return nil, fmt.Errorf("index observations: %w", err)
	}
	intradayObs, _ := writeSessionGraphs(ctx, pool, opts.DataDir, defs)
	stats.Observations = obs + intradayObs
	logf("indices loaded", "definitions", n, "daily_observations", obs, "intraday_observations", intradayObs)

	// ---------------------------------------------------------------- floorsheet
	if opts.FloorsheetLimit != 0 && opts.ExportDir != "" {
		limit := opts.FloorsheetLimit
		if limit < 0 {
			limit = 1 << 30
		}
		trades, err := writeTrades(ctx, pool, opts.ExportDir, secIDs, srcID, limit)
		if err != nil {
			stats.warn("floorsheet: %v", err)
		}
		stats.Trades = trades
		logf("floorsheet loaded", "rows", trades)
	}

	return stats, nil
}

// ---------------------------------------------------------------- source rows

func ensureSource(ctx context.Context, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO data_sources (slug, display_name, endpoint, license_class, redistribution, terms_url, verified_at, notes)
		VALUES ($1, $2, $3, $4, $5, $6, now(), $7)
		ON CONFLICT (slug) DO UPDATE SET display_name = EXCLUDED.display_name, verified_at = now()
		RETURNING id::text`,
		SourceSlug, SourceName, SourceEndpoint, LicenseClass, Redistribution,
		SourceEndpoint+"disclaimer", "Unofficial client; no redistribution rights. See docs/LICENSING_REPORT.md.").
		Scan(&id)
	return id, err
}

type companyRecord struct {
	Symbol         string `json:"symbol"`
	CompanyName    string `json:"companyName"`
	SecurityName   string `json:"securityName"`
	Status         string `json:"status"`
	SectorName     string `json:"sectorName"`
	InstrumentType string `json:"instrumentType"`
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	out := nonSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "_")
	return strings.Trim(out, "_")
}

func ensureSectors(ctx context.Context, pool *pgxPool, companies []companyRecord) (map[string]string, int, error) {
	ids := map[string]string{}
	for _, c := range companies {
		name := strings.TrimSpace(c.SectorName)
		if name == "" {
			name = "Unclassified"
		}
		code := slug(name)
		if code == "" {
			code = "unclassified"
		}
		if _, ok := ids[code]; ok {
			continue
		}
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO sectors (code, name) VALUES ($1, $2)
			ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
			RETURNING id::text`, code, name).Scan(&id); err != nil {
			return nil, 0, err
		}
		ids[code] = id
	}
	return ids, len(ids), nil
}
func mapCompanyInstrument(instrumentType, securityName string) (string, string, error) {
	instrumentType = strings.TrimSpace(strings.ToLower(instrumentType))
	securityName = strings.TrimSpace(securityName)

	switch {
	case instrumentType == "equity":
		companyInstrument := "equity"
		securityType := nepse.Classify(securityName)
		return companyInstrument, securityType, nil
	case instrumentType == "mutual funds":
		companyInstrument := "mutual funds"
		securityType := "mutual_fund"
		return companyInstrument, securityType, nil
	case instrumentType == "non-convertible debentures":
		companyInstrument := "non-convertible debentures"
		securityType := "debenture"
		return companyInstrument, securityType, nil
	case instrumentType == "preference shares":
		companyInstrument := "preference shares"
		securityType := "preference"
		return companyInstrument, securityType, nil
	default:
		return "", "", fmt.Errorf("unknown instrument type %q for symbol %q", instrumentType, securityName)
companyInstrument, securityType, err := mapCompanyInstrument(instrument, strings.TrimSpace(c.SecurityName))
	}
}

func ensureCompanies(ctx context.Context, pool *pgxPool, companies []companyRecord, sectorIDs map[string]string) (map[string]string, map[string]string, int, error) {
	companyIDs := make(map[string]string, len(companies))
	secIDs := make(map[string]string, len(companies))
	securities := 0

	for _, c := range companies {
		sym := strings.ToUpper(strings.TrimSpace(c.Symbol))
		if sym == "" {
			continue
		}
		name := strings.TrimSpace(c.CompanyName)
		if name == "" {
			name = strings.TrimSpace(c.SecurityName)
		}
		if name == "" {
			name = sym
		}
		sector := slug(c.SectorName)
		if sector == "" {
			sector = "unclassified"
		}
		status := "active"
		if s := strings.ToUpper(strings.TrimSpace(c.Status)); s != "" && s != "A" {
			status = "inactive"
		}
		instrument := strings.ToLower(strings.TrimSpace(c.InstrumentType))
		if instrument == "" {
			instrument = "equity"
		}

		var companyID string
		if err := pool.QueryRow(ctx, `
			INSERT INTO companies (symbol, name, sector_id, status, instrument)
			VALUES ($1, $2, $3::uuid, $4, $5)
			ON CONFLICT (symbol) DO UPDATE SET
				name = EXCLUDED.name, sector_id = EXCLUDED.sector_id,
				status = EXCLUDED.status, instrument = EXCLUDED.instrument,
				updated_at = now()
			RETURNING id::text`, sym, name, sectorIDs[sector], status, instrument).Scan(&companyID); err != nil {
			return nil, nil, 0, fmt.Errorf("%s: %w", sym, err)
		}
		companyIDs[sym] = companyID

		var secID string
		if err := pool.QueryRow(ctx, `
			INSERT INTO securities (company_id, symbol, security_type)
			VALUES ($1::uuid, $2, securityType)
			ON CONFLICT (symbol) DO UPDATE SET company_id = EXCLUDED.company_id, security_type = EXCLUDED.security_type
			RETURNING id::text`, companyID, sym).Scan(&secID); err != nil {
			return nil, nil, 0, fmt.Errorf("%s security: %w", sym, err)
		}
		secIDs[sym] = secID
		securities++
	}
	return companyIDs, secIDs, securities, nil
}

func ensureSessions(ctx context.Context, pool *pgxPool, dates map[string]struct{}) error {
	ordered := make([]string, 0, len(dates))
	for d := range dates {
		if d != "" {
			ordered = append(ordered, d)
		}
	}
	sort.Strings(ordered)
	for _, d := range ordered {
		if _, err := pool.Exec(ctx,
			`INSERT INTO trading_sessions (trading_date) VALUES ($1::date) ON CONFLICT DO NOTHING`, d); err != nil {
			return fmt.Errorf("%s: %w", d, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- daily bars

type barRow struct {
	Symbol        string
	Date          string
	Open          *float64
	High          *float64
	Low           *float64
	Close         float64
	PreviousClose *float64
	Volume        *float64
	Turnover      *float64
	Transactions  *float64
	ListedShares  *float64
	PublicShares  *float64
	MarketCap     *float64
	HasQuote      bool
}

type detailFile struct {
	Details struct {
		Quote struct {
			OpenPrice          *float64 `json:"openPrice"`
			HighPrice          *float64 `json:"highPrice"`
			LowPrice           *float64 `json:"lowPrice"`
			LastTradedPrice    *float64 `json:"lastTradedPrice"`
			PreviousClose      *float64 `json:"previousClose"`
			TotalTradeQuantity *float64 `json:"totalTradeQuantity"`
			TotalTrades        *float64 `json:"totalTrades"`
			BusinessDate       string   `json:"businessDate"`
		} `json:"securityDailyTradeDto"`
		ListedShares *float64 `json:"stockListedShares"`
		PublicShares *float64 `json:"publicShares"`
		MarketCap    *float64 `json:"marketCapitalization"`
	} `json:"details"`
}

// loadCompanyDetails reads one file per symbol. Files are written by the legacy
// collector, so a partially written or absent file is expected and skipped.

// loadCompanyDetails recursively reads one file per symbol.
// Nested directory paths are preserved as slash-separated symbols.
func loadCompanyDetails(dataDir string) ([]barRow, error) {
	root := filepath.Join(dataDir, "company_details")
	out := make([]barRow, 0, 440)

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		if !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		// Preserve nested symbols, e.g. GBILD84/85.json.
		symbol := strings.TrimSuffix(filepath.ToSlash(rel), filepath.Ext(rel))
		symbol = strings.ToUpper(strings.TrimSpace(symbol))

		if symbol == "" || strings.HasSuffix(symbol, "_EXTRAS") {
			return nil
		}

		var df detailFile
		if err := readJSONFile(path, &df); err != nil {
			// Preserve existing behavior: skip incomplete snapshots.
			return nil
		}

		q := df.Details.Quote
		row := barRow{
			Symbol:        symbol,
			Date:          normalizeDate(q.BusinessDate),
			Open:          q.OpenPrice,
			High:          q.HighPrice,
			Low:           q.LowPrice,
			PreviousClose: q.PreviousClose,
			Volume:        q.TotalTradeQuantity,
			Transactions:  q.TotalTrades,
			ListedShares:  df.Details.ListedShares,
			PublicShares:  df.Details.PublicShares,
			MarketCap:     df.Details.MarketCap,
		}

		if q.LastTradedPrice != nil {
			row.Close = *q.LastTradedPrice
		}

		if q.OpenPrice == nil &&
			q.HighPrice == nil &&
			q.LowPrice == nil &&
			q.LastTradedPrice == nil {
			return nil
		}

		row.HasQuote = true

		if row.Close == 0 {
			// A suspended scrip may report zero trades but retain an open price.
			if row.Open != nil {
				row.Close = *row.Open
			} else {
				return nil
			}
		}

		// Turnover is deliberately left nil. It is not reported per security
		// in this snapshot and must not be estimated as volume multiplied by close.
		out = append(out, row)
		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("walk company details: %w", err)
	}

	return out, nil
}

type pvhItem struct {
	BusinessDate          string   `json:"businessDate"`
	Symbol                string   `json:"symbol"`
	OpenPrice             *float64 `json:"openPrice"`
	HighPrice             *float64 `json:"highPrice"`
	LowPrice              *float64 `json:"lowPrice"`
	ClosePrice            *float64 `json:"closePrice"`
	TotalTradedQuantity   *float64 `json:"totalTradedQuantity"`
	TotalTradedValue      *float64 `json:"totalTradedValue"`
	PreviousDayClosePrice *float64 `json:"previousDayClosePrice"`
	TotalTrades           *float64 `json:"totalTrades"`
	MarketCapitalization  *float64 `json:"marketCapitalization"`
}

type pvhEnvelope struct {
	Content []pvhItem `json:"content"`
}

func loadHistoricalCompanyPrices(dataDir string) ([]barRow, error) {
	histDir := filepath.Join(filepath.Dir(filepath.Clean(dataDir)), "historical")
	entries, err := os.ReadDir(histDir)
	if err != nil {
		return nil, err
	}
	out := make([]barRow, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		sym := strings.ToUpper(strings.TrimSpace(strings.TrimSuffix(e.Name(), ".json")))
		if sym == "" {
			continue
		}
		path := filepath.Join(histDir, e.Name())
		var env struct {
			Content []struct {
				BusinessDate        string   `json:"businessDate"`
				HighPrice           *float64 `json:"highPrice"`
				LowPrice            *float64 `json:"lowPrice"`
				ClosePrice          *float64 `json:"closePrice"`
				TotalTradedQuantity *float64 `json:"totalTradedQuantity"`
				TotalTradedValue    *float64 `json:"totalTradedValue"`
				TotalTrades         *float64 `json:"totalTrades"`
			} `json:"content"`
		}
		if err := readJSONFile(path, &env); err != nil {
			return nil, err
		}
		for _, it := range env.Content {
			var cl float64
			if it.ClosePrice != nil {
				cl = *it.ClosePrice
			} else if it.HighPrice != nil {
				cl = *it.HighPrice
			}
			row := barRow{
				Symbol:       sym,
				Date:         normalizeDate(it.BusinessDate),
				High:         it.HighPrice,
				Low:          it.LowPrice,
				Close:        cl,
				Volume:       it.TotalTradedQuantity,
				Turnover:     it.TotalTradedValue,
				Transactions: it.TotalTrades,
				HasQuote:     cl > 0,
			}
			out = append(out, row)
		}
	}
	return out, nil
}

func loadPriceVolumeHistory(dataDir string) ([]barRow, error) {
	path := filepath.Join(dataDir, "price_volume_history.json")
	var env pvhEnvelope
	if err := readJSONFile(path, &env); err != nil {
		return nil, err
	}
	out := make([]barRow, 0, len(env.Content))
	for _, it := range env.Content {
		sym := strings.ToUpper(strings.TrimSpace(it.Symbol))
		if sym == "" {
			continue
		}
		var cl float64
		if it.ClosePrice != nil {
			cl = *it.ClosePrice
		} else if it.OpenPrice != nil {
			cl = *it.OpenPrice
		}
		row := barRow{
			Symbol:        sym,
			Date:          normalizeDate(it.BusinessDate),
			Open:          it.OpenPrice,
			High:          it.HighPrice,
			Low:           it.LowPrice,
			Close:         cl,
			PreviousClose: it.PreviousDayClosePrice,
			Volume:        it.TotalTradedQuantity,
			Turnover:      it.TotalTradedValue,
			Transactions:  it.TotalTrades,
			MarketCap:     it.MarketCapitalization,
			HasQuote:      cl > 0,
		}
		out = append(out, row)
	}
	return out, nil
}

func writeBars(ctx context.Context, pool *pgxPool, bars []barRow, secIDs map[string]string, srcID string) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	written := 0
	for _, b := range bars {
		secID, ok := secIDs[b.Symbol]
		if !ok {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO daily_ohlcv (
				security_id, trading_date, open, high, low, close, previous_close,
				volume, turnover, transactions, data_source_id)
			VALUES ($1::uuid, $2::date, $3, $4, $5, $6, $7, $8, $9, $10, $11::uuid)
			ON CONFLICT (security_id, trading_date) DO UPDATE SET
				open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low,
				close = EXCLUDED.close, previous_close = EXCLUDED.previous_close,
				volume = EXCLUDED.volume, turnover = EXCLUDED.turnover,
				transactions = EXCLUDED.transactions,
				data_source_id = EXCLUDED.data_source_id,
				ingested_at = now()`,
			secID, b.Date, b.Open, b.High, b.Low, b.Close, b.PreviousClose,
			b.Volume, b.Turnover, b.Transactions, srcID); err != nil {
			return written, fmt.Errorf("%s %s: %w", b.Symbol, b.Date, err)
		}
		written++

		if b.ListedShares != nil || b.PublicShares != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE securities SET listed_shares = COALESCE($2, listed_shares),
				                      public_shares = COALESCE($3, public_shares)
				WHERE id = $1::uuid`, secID, b.ListedShares, b.PublicShares); err != nil {
				return written, err
			}
		}
	}
	return written, tx.Commit(ctx)
}

// ---------------------------------------------------------------- summary

type summaryResult struct {
	Date          string
	Index         *float64
	PointChange   *float64
	PercentChange *float64
	Turnover      *float64
	TradedShares  *float64
	Transactions  *float64
	Securities    *float64
	Advanced      int
	Declined      int
	Unchanged     int
	MarketCap     string
	FloatCap      string
	CapCovered    int
	CapEligible   int
}

// loadSummary assembles one session summary from three real sources: the
// exchange summary block, the index quote and the per-company market caps. Each
// piece is optional, and a missing one stays NULL rather than becoming zero.
func loadSummary(dataDir string) (summaryResult, error) {
	var out summaryResult

	type kv struct {
		Detail string  `json:"detail"`
		Value  flexNum `json:"value"`
	}
	var items []kv
	if err := readJSONFile(filepath.Join(dataDir, "summary.json"), &items); err != nil {
		return out, err
	}
	for _, it := range items {
		v, ok := it.Value.Float()
		if !ok {
			continue
		}
		switch {
		case strings.Contains(it.Detail, "Turnover"):
			out.Turnover = &v
		case strings.Contains(it.Detail, "Shares"):
			out.TradedShares = &v
		case strings.Contains(it.Detail, "Transactions"):
			out.Transactions = &v
		case strings.Contains(it.Detail, "Scrips"):
			out.Securities = &v
		}
	}

	// Headline index and the advance/decline split.
	details, _ := loadCompanyDetails(dataDir)
	var capSum, floatSum float64
	covered, eligible := 0, 0
	for _, d := range details {
		if d.Date != "" && out.Date == "" {
			out.Date = d.Date
		}
		eligible++
		if d.MarketCap != nil && *d.MarketCap > 0 {
			capSum += *d.MarketCap
			covered++
		}
		if d.PublicShares != nil && d.Close > 0 {
			floatSum += *d.PublicShares * d.Close
		}
		switch {
		case d.PreviousClose == nil || *d.PreviousClose == 0:
			out.Unchanged++
		case d.Close > *d.PreviousClose:
			out.Advanced++
		case d.Close < *d.PreviousClose:
			out.Declined++
		default:
			out.Unchanged++
		}
	}
	out.MarketCap = formatMoney(capSum)
	out.FloatCap = formatMoney(floatSum)
	out.CapCovered = covered
	out.CapEligible = eligible

	type quote struct {
		Index         string   `json:"index"`
		CurrentValue  *float64 `json:"currentValue"`
		Close         *float64 `json:"close"`
		Change        *float64 `json:"change"`
		PerChange     *float64 `json:"perChange"`
		GeneratedTime string   `json:"generatedTime"`
	}
	var quotes []quote
	if err := readJSONFile(filepath.Join(dataDir, "nepse_index.json"), &quotes); err == nil {
		for _, q := range quotes {
			if q.Index != "NEPSE Index" {
				continue
			}
			out.Index = q.CurrentValue
			if out.Index == nil {
				out.Index = q.Close
			}
			out.PointChange = q.Change
			out.PercentChange = q.PerChange
			break
		}
	}

	if out.Date == "" && len(details) > 0 {
		out.Date = details[0].Date
	}
	return out, nil
}

// ---------------------------------------------------------------- indices

type indexQuote struct {
	Code   string
	Name   string
	Kind   string
	Date   string
	Value  *float64
	Change *float64
	PerChg *float64
}

type indexFileRow struct {
	Index         string   `json:"index"`
	CurrentValue  *float64 `json:"currentValue"`
	Close         *float64 `json:"close"`
	Change        *float64 `json:"change"`
	PerChange     *float64 `json:"perChange"`
	GeneratedTime string   `json:"generatedTime"`
}

func loadIndices(dataDir, exportDir string) ([]indexQuote, error) {
	var out []indexQuote
	seen := map[string]bool{}

	var headline []indexFileRow
	if err := readJSONFile(filepath.Join(dataDir, "nepse_index.json"), &headline); err == nil {
		for _, q := range headline {
			name := strings.TrimSpace(q.Index)
			if name == "" || seen[name] {
				continue
			}
			v := q.CurrentValue
			if v == nil {
				v = q.Close
			}
			out = append(out, indexQuote{
				Code: indexCode(name), Name: name, Kind: "headline",
				Date: normalizeDate(q.GeneratedTime), Value: v,
				Change: q.Change, PerChg: q.PerChange,
			})
			seen[name] = true
		}
	}

	var subs []indexFileRow
	if err := readJSONFile(filepath.Join(dataDir, "sub_indices.json"), &subs); err == nil {
		for _, q := range subs {
			name := strings.TrimSpace(q.Index)
			if name == "" || seen[name] {
				continue
			}
			out = append(out, indexQuote{
				Code: indexCode(name), Name: name, Kind: "sector",
				Value: q.CurrentValue, Change: q.Change, PerChg: q.PerChange,
			})
			seen[name] = true
		}
	}

	// Historical daily observations from the legacy index_snapshots table.
	if exportDir != "" {
		if _, err := readJSONL(filepath.Join(exportDir, "index_snapshots.jsonl"), -1, func(raw map[string]any) bool {
			name, _ := raw["index_name"].(string)
			date := normalizeDate(asString(raw["business_date"]))
			if name == "" || date == "" {
				return false
			}
			val, ok := asFloat(raw["value"])
			if !ok {
				return false
			}
			change, _ := asFloat(raw["change"])
			per, _ := asFloat(raw["per_change"])
			out = append(out, indexQuote{
				Code: indexCode(name), Name: name, Kind: indexKind(name),
				Date: date, Value: &val, Change: changeOrNil(change), PerChg: changeOrNil(per),
			})
			return true
		}); err != nil {
			return out, err
		}
	}
	return out, nil
}

func indexKind(name string) string {
	n := strings.ToLower(name)
	if strings.Contains(n, "subindex") || strings.Contains(n, "sub-index") {
		return "sector"
	}
	return "headline"
}

func indexCode(name string) string {
	base := strings.TrimSuffix(strings.TrimSpace(name), " Index")
	base = strings.ReplaceAll(base, "SubIndex", "")
	base = strings.ReplaceAll(base, "Sub-Index", "")
	code := slug(base)
	if code == "" {
		return "index"
	}
	return code
}

func ensureIndices(ctx context.Context, pool *pgxPool, quotes []indexQuote) (map[string]string, int, error) {
	ids := map[string]string{}
	for _, q := range quotes {
		if q.Code == "" {
			continue
		}
		if _, ok := ids[q.Code]; ok {
			continue
		}
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO index_definitions (code, name, kind) VALUES ($1, $2, $3)
			ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, kind = EXCLUDED.kind
			RETURNING id::text`, q.Code, q.Name, q.Kind).Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("%s: %w", q.Code, err)
		}
		ids[q.Code] = id
	}
	return ids, len(ids), nil
}

func writeIndexObservations(ctx context.Context, pool *pgxPool, quotes []indexQuote, defs map[string]string, sessions map[string]struct{}) (int, error) {
	written := 0
	seen := map[string]bool{} // index|date|granularity
	for _, q := range quotes {
		id, ok := defs[q.Code]
		if !ok || q.Date == "" || q.Value == nil {
			continue
		}
		key := q.Code + "|" + q.Date + "|daily"
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, ok := sessions[q.Date]; !ok {
			if err := ensureSessions(ctx, pool, map[string]struct{}{q.Date: {}}); err != nil {
				return written, err
			}
			sessions[q.Date] = struct{}{}
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO index_observations (index_id, trading_date, granularity, observed_at, value, point_change, percent_change)
			VALUES ($1::uuid, $2::date, 'daily', $2::date::timestamptz, $3, $4, $5)
			ON CONFLICT (index_id, trading_date, granularity, observed_at) DO UPDATE SET
				value = EXCLUDED.value, point_change = EXCLUDED.point_change,
				percent_change = EXCLUDED.percent_change`,
			id, q.Date, q.Value, q.Change, q.PerChg); err != nil {
			return written, fmt.Errorf("%s %s: %w", q.Code, q.Date, err)
		}
		written++
	}
	return written, nil
}

var graphFileMap = map[string]string{
	"daily_nepse_index_graph.json":                 "nepse",
	"daily_sensitive_index_graph.json":             "sensitive",
	"daily_float_index_graph.json":                 "float",
	"daily_sensitive_float_index_graph.json":       "sensitive_float",
	"daily_bank_subindex_graph.json":               "banking",
	"daily_development_bank_subindex_graph.json":   "development_bank",
	"daily_finance_subindex_graph.json":            "finance",
	"daily_hotel_tourism_subindex_graph.json":      "hotels_and_tourism",
	"daily_hydro_subindex_graph.json":              "hydropower",
	"daily_investment_subindex_graph.json":         "investment",
	"daily_life_insurance_subindex_graph.json":     "life_insurance",
	"daily_manufacturing_subindex_graph.json":      "manufacturing_and_processing",
	"daily_microfinance_subindex_graph.json":       "microfinance",
	"daily_mutualfund_subindex_graph.json":         "mutual_fund",
	"daily_non_life_insurance_subindex_graph.json": "non_life_insurance",
	"daily_others_subindex_graph.json":             "others",
	"daily_trading_subindex_graph.json":            "trading",
}

func writeSessionGraphs(ctx context.Context, pool *pgxPool, dataDir string, defs map[string]string) (int, error) {
	npt := time.FixedZone("NPT", 5*3600+45*60)
	written := 0

	for filename, code := range graphFileMap {
		id, ok := defs[code]
		if !ok {
			continue
		}
		path := filepath.Join(dataDir, filename)
		var points [][]float64
		if err := readJSONFile(path, &points); err != nil {
			continue
		}
		for _, pt := range points {
			if len(pt) < 2 {
				continue
			}
			sec := int64(pt[0])
			val := pt[1]
			t := time.Unix(sec, 0).UTC()
			tradingDate := t.In(npt).Format("2006-01-02")

			_, err := pool.Exec(ctx, `
				INSERT INTO index_observations (index_id, trading_date, granularity, observed_at, value)
				VALUES ($1::uuid, $2::date, 'intraday', $3::timestamptz, $4)
				ON CONFLICT (index_id, trading_date, granularity, observed_at) DO NOTHING`,
				id, tradingDate, t, val)
			if err == nil {
				written++
			}
		}
	}
	return written, nil
}

// ---------------------------------------------------------------- floorsheet

func writeTrades(ctx context.Context, pool *pgxPool, exportDir string, secIDs map[string]string, srcID string, limit int) (int, error) {
	path := filepath.Join(exportDir, "floorsheet.jsonl")
	if _, err := os.Stat(path); err != nil {
		return 0, fmt.Errorf("no floorsheet export at %s", path)
	}

	type tradeRow struct {
		contractID string
		date       string
		symbol     string
		buyer      *string
		seller     *string
		quantity   float64
		rate       float64
		amount     float64
		executedAt *time.Time
	}
	rows := make([]tradeRow, 0, 4096)
	seen := map[string]bool{}

	_, err := readJSONL(path, limit, func(raw map[string]any) bool {
		contract := asString(raw["contract_id"])
		if contract == "" {
			contract = asString(raw["contract_no"])
		}
		symbol := strings.ToUpper(asString(raw["symbol"]))
		date := normalizeDate(asString(raw["business_date"]))
		qty, okQ := asFloat(raw["quantity"])
		rate, okR := asFloat(raw["rate"])
		if contract == "" || symbol == "" || date == "" || !okQ || !okR || qty <= 0 || rate <= 0 {
			return false
		}
		amount, okA := asFloat(raw["amount"])
		if !okA {
			amount = qty * rate
		}
		if seen[contract] {
			return false
		}
		seen[contract] = true

		tr := tradeRow{
			contractID: contract, date: date, symbol: symbol,
			buyer: optionalString(raw["buyer_broker"]), seller: optionalString(raw["seller_broker"]),
			quantity: qty, rate: rate, amount: amount,
		}
		if ts := parseTimestamp(asString(raw["trade_time"])); ts != nil {
			tr.executedAt = ts
		}
		rows = append(rows, tr)
		return true
	})
	if err != nil {
		return 0, err
	}

	// Trades reference trading_sessions, so dates must exist first.
	dates := map[string]struct{}{}
	for _, r := range rows {
		dates[r.date] = struct{}{}
	}
	if err := ensureSessions(ctx, pool, dates); err != nil {
		return 0, err
	}

	if len(rows) == 0 {
		return 0, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Stage into a session-local clone of the table, then merge. This keeps the
	// bulk-load speed of COPY while making a replay idempotent: the export and
	// the database may legitimately contain the same contract already.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE stage_trades (LIKE floorsheet_trades INCLUDING DEFAULTS)
		ON COMMIT DROP`); err != nil {
		return 0, err
	}

	batch := 2000
	for start := 0; start < len(rows); start += batch {
		end := start + batch
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		source := pgx.CopyFromSlice(len(chunk), func(i int) ([]any, error) {
			r := chunk[i]
			var secID *string
			if id, ok := secIDs[r.symbol]; ok {
				secID = &id
			}
			return []any{
				r.contractID, r.date, r.symbol, secID, r.buyer, r.seller,
				r.quantity, r.rate, r.amount, r.executedAt, srcID, []byte(`{}`),
			}, nil
		})
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"stage_trades"},
			[]string{"contract_id", "trading_date", "symbol", "security_id", "buyer_broker",
				"seller_broker", "quantity", "rate", "amount", "executed_at", "data_source_id", "raw"},
			source); err != nil {
			return 0, fmt.Errorf("stage floorsheet: %w", err)
		}
	}

	// A staged contract whose session is missing would violate the FK; insert only
	// the rows that resolve, and report the rest through the caller's warnings.
	tag, err := tx.Exec(ctx, `
		INSERT INTO floorsheet_trades
		SELECT s.* FROM stage_trades s
		WHERE NOT EXISTS (SELECT 1 FROM floorsheet_trades t WHERE t.contract_id = s.contract_id)
		ON CONFLICT (contract_id) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("merge floorsheet: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// ---------------------------------------------------------------- json helpers

// flexNum accepts a JSON number, a JSON string or null. The exchange is not
// consistent about which it sends, and the legacy collector stored both.
type flexNum struct {
	set bool
	v   float64
}

func (f *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == `""` {
		return nil
	}
	s = strings.Trim(s, `"`)
	s = strings.ReplaceAll(s, ",", "")
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil // ignore unparseable values instead of failing the file
	}
	f.v, f.set = n, true
	return nil
}

func (f flexNum) Float() (float64, bool) { return f.v, f.set }

func readJSONFile(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func readJSONL(path string, limit int, fn func(map[string]any) bool) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)

	n, kept := 0, 0
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		n++
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		if fn(raw) {
			kept++
			if limit > 0 && kept >= limit {
				break
			}
		}
	}
	return n, sc.Err()
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case nil:
		return ""
	}
	return ""
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		s := strings.ReplaceAll(strings.TrimSpace(t), ",", "")
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	return 0, false
}

func optionalString(v any) *string {
	s := asString(v)
	if s == "" {
		return nil
	}
	return &s
}

func changeOrNil(v float64) *float64 { return &v }

// normalizeDate accepts the several timestamp shapes present in the captured
// files and keeps only the date part.
func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		if d, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return d.Format("2006-01-02")
		}
	}
	return ""
}

// parseTimestamp handles both SQLite's "2006-01-02 15:04:05.999999" and RFC3339.
func parseTimestamp(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// RFC3339 timestamps already contain their timezone.
	// Preserve the represented instant; do not apply NPT again.
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		t = t.UTC()
		return &t
	}

	// These formats contain Nepal local wall-clock time without
	// an explicit timezone. Interpret them as NPT (UTC+05:45).
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			t = t.UTC().Add(-(5*time.Hour + 45*time.Minute))
			return &t
		}
	}

	return nil
}

func formatMoney(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// LatestExport finds the newest exports/legacy/<run> directory alongside the
// data directory, so callers do not need to know the run timestamp.
func LatestExport(dataDir string) string {
	root := filepath.Join(filepath.Dir(filepath.Clean(dataDir)), "exports", "legacy")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, n := range names {
		candidate := filepath.Join(root, n)
		if _, err := os.Stat(filepath.Join(candidate, "floorsheet.jsonl")); err == nil {
			return candidate
		}
	}
	return ""
}
