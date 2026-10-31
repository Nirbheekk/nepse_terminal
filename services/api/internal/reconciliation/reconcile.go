// Package reconcile compares what the legacy collector recorded with what the
// platform now stores, and reports the differences.
//
// A count comparison alone proves very little: it is possible to load 48,273
// rows and have every one of them wrong. So this package also walks a sample of
// contracts field by field, recomputes aggregates on both sides, and compares
// reference sets by membership rather than by size.
package reconciliation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "modernc.org/sqlite"
)

// Check is one comparable fact about a dataset.
type Check struct {
	Dataset  string `json:"dataset"`
	Field    string `json:"field"`
	Legacy   string `json:"legacy"`
	Platform string `json:"platform"`
	Match    bool   `json:"match"`
	// Expected marks a difference that is a known, documented consequence of the
	// design rather than a loss or a defect.
	Expected bool   `json:"expected"`
	Severity string `json:"severity"`
	Note     string `json:"note,omitempty"`
}

// DatasetResult is the headline count comparison for one dataset.
type DatasetResult struct {
	Name          string `json:"name"`
	LegacyCount   int64  `json:"legacy_count"`
	PlatformCount int64  `json:"platform_count"`
	Match         bool   `json:"match"`
	Expected      bool   `json:"expected"`
	Note          string `json:"note,omitempty"`
}

// RowMismatch is one field of one contract that disagrees.
type RowMismatch struct {
	ContractID string `json:"contract_id"`
	Field      string `json:"field"`
	Legacy     string `json:"legacy"`
	Platform   string `json:"platform"`
}

// Report is the full comparison.
type Report struct {
	CheckedAt       time.Time       `json:"checked_at"`
	LegacyPath      string          `json:"legacy_path"`
	Datasets        []DatasetResult `json:"datasets"`
	Checks          []Check         `json:"checks"`
	RowsCompared    int             `json:"rows_compared"`
	RowsMissing     int             `json:"rows_missing"`
	FieldMismatches int             `json:"field_mismatches"`
	MismatchSamples []RowMismatch   `json:"mismatch_samples"`
	Notes           []string        `json:"notes"`
}

// Totals summarises pass/fail for callers that only need a verdict.
func (r *Report) Totals() (matches, mismatches, expected int) {
	for _, c := range r.Checks {
		switch {
		case c.Match:
			matches++
		case c.Expected:
			expected++
		default:
			mismatches++
		}
	}
	return
}

// Discrepancies flattens every non-expected failure, for storage and alerting.
func (r *Report) Discrepancies() []map[string]string {
	out := []map[string]string{}
	for _, c := range r.Checks {
		if c.Match || c.Expected {
			continue
		}
		out = append(out, map[string]string{
			"dataset": c.Dataset, "field": c.Field,
			"legacy": c.Legacy, "platform": c.Platform,
			"severity": c.Severity, "note": c.Note,
		})
	}
	for _, m := range r.MismatchSamples {
		out = append(out, map[string]string{
			"dataset": "floorsheet", "field": "contract " + m.ContractID + " " + m.Field,
			"legacy": m.Legacy, "platform": m.Platform, "severity": "critical",
		})
	}
	return out
}

// Ledger is the per-dataset count comparison, kept small enough to store as jsonb.
func (r *Report) Ledger() []map[string]any {
	out := make([]map[string]any, 0, len(r.Datasets))
	for _, d := range r.Datasets {
		out = append(out, map[string]any{
			"dataset": d.Name, "legacy": d.LegacyCount, "platform": d.PlatformCount,
			"match": d.Match, "expected": d.Expected,
		})
	}
	return out
}

// Status is the overall verdict.
func (r *Report) Status() string {
	_, mismatches, _ := r.Totals()
	if r.RowsMissing > 0 || r.FieldMismatches > 0 || mismatches > 0 {
		return "mismatch"
	}
	return "match"
}

// Options locates both sides.
type Options struct {
	LegacyPath string
	Pool       *pgxpool.Pool
	DataDir    string
	SampleSize int
}

// Run performs the comparison.
func Run(ctx context.Context, opts Options) (*Report, error) {
	if opts.SampleSize <= 0 {
		opts.SampleSize = 5000
	}
	rep := &Report{CheckedAt: time.Now().UTC(), LegacyPath: opts.LegacyPath}

	db, err := openLegacy(opts.LegacyPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := compareFloorsheet(ctx, db, opts, rep); err != nil {
		return nil, fmt.Errorf("floorsheet: %w", err)
	}
	if err := compareIndices(ctx, db, opts.Pool, rep); err != nil {
		return nil, fmt.Errorf("indices: %w", err)
	}
	if err := compareMarketCap(ctx, db, opts.Pool, rep); err != nil {
		return nil, fmt.Errorf("market cap: %w", err)
	}
	if err := compareReference(ctx, opts, rep); err != nil {
		return nil, fmt.Errorf("reference: %w", err)
	}
	if err := compareBars(ctx, opts, rep); err != nil {
		return nil, fmt.Errorf("daily bars: %w", err)
	}
	if err := compareCaptureCoverage(ctx, opts.Pool, rep); err != nil {
		return nil, fmt.Errorf("capture coverage: %w", err)
	}
	compareEmptyLegacyTables(ctx, db, opts.Pool, rep)
	return rep, nil
}

// ---------------------------------------------------------------- floorsheet

func compareFloorsheet(ctx context.Context, db *sql.DB, opts Options, rep *Report) error {
	var legacyCount, platformCount int64
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM floorsheet`).Scan(&legacyCount); err != nil {
		return err
	}
	if err := opts.Pool.QueryRow(ctx, `SELECT count(*) FROM floorsheet_trades`).Scan(&platformCount); err != nil {
		return err
	}
	rep.Datasets = append(rep.Datasets, DatasetResult{
		Name:          "floorsheet",
		LegacyCount:   legacyCount,
		PlatformCount: platformCount,
		Match:         legacyCount == platformCount,
		Note:          "contract-level trades captured while the market was open",
	})

	countCheck(rep, "floorsheet", "row count", legacyCount, platformCount)

	// Aggregates catch a whole class of quiet corruption: right number of rows,
	// wrong quantities.
	var legacyAmount, legacyQty float64
	var legacySymbols int64
	if err := db.QueryRowContext(ctx,
		`SELECT coalesce(sum(amount),0), coalesce(sum(quantity),0), count(DISTINCT symbol) FROM floorsheet`).
		Scan(&legacyAmount, &legacyQty, &legacySymbols); err != nil {
		return err
	}
	var platformAmount, platformQty float64
	var platformSymbols int64
	if err := opts.Pool.QueryRow(ctx, `
		SELECT coalesce(sum(amount),0)::float8, coalesce(sum(quantity),0)::float8,
		       count(DISTINCT symbol)
		FROM floorsheet_trades`).Scan(&platformAmount, &platformQty, &platformSymbols); err != nil {
		return err
	}

	approxCheck(rep, "floorsheet", "sum(amount)", legacyAmount, platformAmount, 0.01)
	approxCheck(rep, "floorsheet", "sum(quantity)", legacyQty, platformQty, 0.0001)
	countCheck(rep, "floorsheet", "distinct symbols", legacySymbols, platformSymbols)

	// Per-symbol contract counts, compared for the ten busiest symbols.
	rows, err := db.QueryContext(ctx, `
		SELECT symbol, count(*) FROM floorsheet GROUP BY symbol ORDER BY count(*) DESC, symbol LIMIT 10`)
	if err != nil {
		return err
	}
	type symbolCount struct {
		symbol string
		n      int64
	}
	var top []symbolCount
	for rows.Next() {
		var sc symbolCount
		if err := rows.Scan(&sc.symbol, &sc.n); err != nil {
			rows.Close()
			return err
		}
		top = append(top, sc)
	}
	rows.Close()

	for _, sc := range top {
		var n int64
		if err := opts.Pool.QueryRow(ctx,
			`SELECT count(*) FROM floorsheet_trades WHERE symbol = $1`, sc.symbol).Scan(&n); err != nil {
			return err
		}
		countCheck(rep, "floorsheet", "contracts for "+sc.symbol, sc.n, n)
	}

	// Field-level comparison over a sample.
	type legacyRow struct {
		contractID string
		symbol     string
		date       string
		qty        float64
		rate       float64
		amount     float64
		buyer      sql.NullString
		seller     sql.NullString
		tradeTime  sql.NullString
	}
	sampleRows, err := db.QueryContext(ctx, `
		SELECT contract_id, symbol, business_date, quantity, rate, amount,
		       buyer_broker, seller_broker, trade_time
		FROM floorsheet ORDER BY contract_id LIMIT ?`, opts.SampleSize)
	if err != nil {
		return err
	}
	var sample []legacyRow
	ids := make([]string, 0, opts.SampleSize)
	for sampleRows.Next() {
		var r legacyRow
		if err := sampleRows.Scan(&r.contractID, &r.symbol, &r.date, &r.qty, &r.rate,
			&r.amount, &r.buyer, &r.seller, &r.tradeTime); err != nil {
			sampleRows.Close()
			return err
		}
		sample = append(sample, r)
		ids = append(ids, r.contractID)
	}
	sampleRows.Close()
	if err := sampleRows.Err(); err != nil {
		return err
	}

	type platformRow struct {
		symbol   string
		date     string
		qty      float64
		rate     float64
		amount   float64
		buyer    *string
		seller   *string
		executed *time.Time
	}
	platform := map[string]platformRow{}
	prows, err := opts.Pool.Query(ctx, `
		SELECT contract_id::text, symbol, trading_date::text, quantity::float8, rate::float8,
		       amount::float8, buyer_broker, seller_broker, executed_at
		FROM floorsheet_trades WHERE contract_id = ANY($1::numeric[])`, ids)
	if err != nil {
		return err
	}
	for prows.Next() {
		var id string
		var r platformRow
		if err := prows.Scan(&id, &r.symbol, &r.date, &r.qty, &r.rate, &r.amount,
			&r.buyer, &r.seller, &r.executed); err != nil {
			prows.Close()
			return err
		}
		platform[id] = r
	}
	prows.Close()

	for _, l := range sample {
		rep.RowsCompared++
		p, ok := platform[l.contractID]
		if !ok {
			rep.RowsMissing++
			rep.addMismatch(RowMismatch{ContractID: l.contractID, Field: "row", Legacy: "present", Platform: "missing"})
			continue
		}
		if l.symbol != p.symbol {
			rep.addMismatch(RowMismatch{l.contractID, "symbol", l.symbol, p.symbol})
		}
		// The SQLite driver returns a DATE column as a timestamp, so the date part is
		// extracted before comparing it with the platform's calendar date.
		legacyDate := l.date
		if len(legacyDate) > 10 {
			legacyDate = legacyDate[:10]
		}
		if legacyDate != p.date {
			rep.addMismatch(RowMismatch{l.contractID, "trading_date", l.date, p.date})
		}
		if !closeEnough(l.qty, p.qty, 0.0001) {
			rep.addMismatch(RowMismatch{l.contractID, "quantity", ftoa(l.qty), ftoa(p.qty)})
		}
		if !closeEnough(l.rate, p.rate, 1e-6) {
			rep.addMismatch(RowMismatch{l.contractID, "rate", ftoa(l.rate), ftoa(p.rate)})
		}
		if !closeEnough(l.amount, p.amount, 0.01) {
			rep.addMismatch(RowMismatch{l.contractID, "amount", ftoa(l.amount), ftoa(p.amount)})
		}
		if nullStr(l.buyer) != deref(p.buyer) {
			rep.addMismatch(RowMismatch{l.contractID, "buyer_broker", nullStr(l.buyer), deref(p.buyer)})
		}
		if nullStr(l.seller) != deref(p.seller) {
			rep.addMismatch(RowMismatch{l.contractID, "seller_broker", nullStr(l.seller), deref(p.seller)})
		}
		// Legacy trade_time is UTC without a zone marker; the platform stores UTC.
		if l.tradeTime.Valid && p.executed != nil {
			want := parseLegacyUTC(l.tradeTime.String)
			if want == nil || !want.Equal(p.executed.UTC()) {
				rep.addMismatch(RowMismatch{l.contractID, "executed_at",
					l.tradeTime.String, p.executed.UTC().Format("2006-01-02 15:04:05.999999")})
			}
		}
	}

	rep.Checks = append(rep.Checks, Check{
		Dataset: "floorsheet", Field: "field-level sample",
		Legacy:   fmt.Sprintf("%d rows", rep.RowsCompared),
		Platform: fmt.Sprintf("%d field mismatches", rep.FieldMismatches+rep.RowsMissing),
		Match:    rep.FieldMismatches == 0 && rep.RowsMissing == 0,
		Severity: "critical",
		Note:     "symbol, date, quantity, rate, amount, both broker fields and execution time compared per contract",
	})
	return nil
}

func (r *Report) addMismatch(m RowMismatch) {
	r.FieldMismatches++
	if len(r.MismatchSamples) < 25 {
		r.MismatchSamples = append(r.MismatchSamples, m)
	}
}

// ---------------------------------------------------------------- indices

func compareIndices(ctx context.Context, db *sql.DB, pool *pgxpool.Pool, rep *Report) error {
	var legacyCount int64
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM index_snapshots`).Scan(&legacyCount); err != nil {
		return err
	}
	var platformCount int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM index_observations WHERE granularity = 'daily'`).Scan(&platformCount); err != nil {
		return err
	}
	rep.Datasets = append(rep.Datasets, DatasetResult{
		Name:          "index_snapshots",
		LegacyCount:   legacyCount,
		PlatformCount: platformCount,
		Match:         legacyCount == platformCount,
		Expected:      platformCount >= legacyCount,
		Note:          "legacy holds one daily row per index; the platform also stores the snapshot file's headline and sector quotes",
	})

	rows, err := db.QueryContext(ctx, `
		SELECT index_name, business_date, value, change, per_change, captured_at
		FROM index_snapshots ORDER BY index_name`)
	if err != nil {
		return err
	}
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var name, date string
		var value float64
		var change, per sql.NullFloat64
		var capturedAt sql.NullString
		if err := rows.Scan(&name, &date, &value, &change, &per, &capturedAt); err != nil {
			return err
		}
		// Observations are identified by when they were taken, so the comparison
		// targets the point the legacy snapshot captured, not just the day.
		var observed *time.Time
		if capturedAt.Valid {
			observed = parseLegacyUTC(capturedAt.String)
		}
		var pValue, pChange, pPer float64
		err := pool.QueryRow(ctx, `
			SELECT o.value::float8, COALESCE(o.point_change, 0)::float8,
			       COALESCE(o.percent_change, 0)::float8
			FROM index_observations o
			JOIN index_definitions d ON d.id = o.index_id
			WHERE d.name = $1 AND o.trading_date = $2::date AND o.granularity = 'daily'
			  AND ($3::timestamptz IS NULL OR o.observed_at = $3::timestamptz)
			ORDER BY o.observed_at DESC LIMIT 1`,
			name, date, observed).Scan(&pValue, &pChange, &pPer)
		if err != nil {
			rep.Checks = append(rep.Checks, Check{
				Dataset: "index_snapshots", Field: name + " " + date,
				Legacy: ftoa(value), Platform: "missing", Match: false, Severity: "critical",
				Note: "legacy snapshot has no corresponding platform observation",
			})
			continue
		}
		checked++
		approxCheck(rep, "index_snapshots", name+" value", value, pValue, 0.0001)
		if change.Valid {
			approxCheck(rep, "index_snapshots", name+" change", change.Float64, pChange, 0.0001)
		}
		if per.Valid {
			approxCheck(rep, "index_snapshots", name+" percent", per.Float64, pPer, 0.0001)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if checked == 0 && legacyCount > 0 {
		rep.Checks = append(rep.Checks, Check{
			Dataset: "index_snapshots", Field: "coverage",
			Legacy: fmt.Sprintf("%d", legacyCount), Platform: "0", Match: false, Severity: "critical",
		})
	}
	return nil
}

// ---------------------------------------------------------------- market cap

func compareMarketCap(ctx context.Context, db *sql.DB, pool *pgxpool.Pool, rep *Report) error {
	var date string
	var legacyCap, legacyFloat float64
	var covered, total int64
	err := db.QueryRowContext(ctx,
		`SELECT business_date, market_cap, float_market_cap, covered, total
		 FROM market_cap_snapshots ORDER BY captured_at DESC LIMIT 1`).
		Scan(&date, &legacyCap, &legacyFloat, &covered, &total)
	if err == sql.ErrNoRows {
		rep.Checks = append(rep.Checks, Check{
			Dataset: "market_cap", Field: "snapshot", Legacy: "none", Platform: "n/a",
			Match: true, Severity: "info", Note: "the legacy database holds no capitalisation snapshot",
		})
		return nil
	}
	if err != nil {
		return err
	}

	var pCap, pFloat float64
	var pCovered, pEligible int
	err = pool.QueryRow(ctx, `
		SELECT COALESCE(market_cap, 0)::float8, COALESCE(float_market_cap, 0)::float8,
		       COALESCE(cap_covered, 0), COALESCE(cap_eligible, 0)
		FROM market_summaries WHERE trading_date = $1::date`, date).
		Scan(&pCap, &pFloat, &pCovered, &pEligible)
	if err != nil {
		rep.Checks = append(rep.Checks, Check{
			Dataset: "market_cap", Field: "summary row " + date,
			Legacy: ftoa(legacyCap), Platform: "missing", Match: false, Severity: "critical",
		})
		return nil
	}

	approxCheck(rep, "market_cap", "market_cap "+date, legacyCap, pCap, 0.01)
	approxCheck(rep, "market_cap", "float_market_cap "+date, legacyFloat, pFloat, 0.01)
	countCheck(rep, "market_cap", "covered "+date, covered, int64(pCovered))
	countCheck(rep, "market_cap", "eligible "+date, total, int64(pEligible))
	return nil
}

// ---------------------------------------------------------------- reference

func compareReference(ctx context.Context, opts Options, rep *Report) error {
	if opts.DataDir == "" {
		return nil
	}

	// Companies: set membership, not just a count.
	body, err := os.ReadFile(filepath.Join(opts.DataDir, "company_list.json"))
	if err == nil {
		var companies []struct {
			Symbol string `json:"symbol"`
		}
		if jsonErr := json.Unmarshal(body, &companies); jsonErr == nil {
			want := map[string]bool{}
			for _, c := range companies {
				if s := strings.ToUpper(strings.TrimSpace(c.Symbol)); s != "" {
					want[s] = true
				}
			}
			// Placeholder issuers are not registry entries: they exist only so a
			// listed security has a parent. Comparing the registry against the
			// legacy list must exclude them, or every promoter class would look like
			// an invented company.
			var platformCount, placeholders int64
			if err := opts.Pool.QueryRow(ctx,
				`SELECT count(*) FILTER (WHERE origin = 'registry'),
				        count(*) FILTER (WHERE origin <> 'registry')
				 FROM companies`).Scan(&platformCount, &placeholders); err != nil {
				return err
			}
			countCheck(rep, "companies", "row count", int64(len(want)), platformCount)

			rep.Checks = append(rep.Checks, Check{
				Dataset: "companies", Field: "placeholder issuers",
				Legacy: "0", Platform: strconv.FormatInt(placeholders, 10),
				Match: true, Expected: true, Severity: "info",
				Note: "listed securities whose issuer is absent from the company registry; " +
					"created so their contracts stay attributable, excluded from the registry count",
			})

			rows, err := opts.Pool.Query(ctx, `SELECT symbol FROM companies WHERE origin = 'registry'`)
			if err != nil {
				return err
			}
			missing := 0
			extra := 0
			have := map[string]bool{}
			for rows.Next() {
				var s string
				if err := rows.Scan(&s); err != nil {
					rows.Close()
					return err
				}
				have[s] = true
				if !want[s] {
					extra++
				}
			}
			rows.Close()
			for s := range want {
				if !have[s] {
					missing++
				}
			}
			rep.Checks = append(rep.Checks, Check{
				Dataset: "companies", Field: "symbol set",
				Legacy:   fmt.Sprintf("%d symbols", len(want)),
				Platform: fmt.Sprintf("%d missing, %d extra", missing, extra),
				Match:    missing == 0 && extra == 0,
				Severity: "critical",
				Note:     "every symbol in company_list.json must exist exactly once in companies",
			})
		}
	}

	// ISIN coverage: how many securities carry a valid identifier.
	var withISIN, validISIN, badChecksum, totalSecurities int64
	if err := opts.Pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE isin IS NOT NULL),
		       count(*) FILTER (WHERE isin_checksum_ok),
		       count(*) FILTER (WHERE isin IS NOT NULL AND isin_checksum_ok IS FALSE),
		       count(*)
		FROM securities`).Scan(&withISIN, &validISIN, &badChecksum, &totalSecurities); err != nil {
		return err
	}
	rep.Checks = append(rep.Checks, Check{
		Dataset: "securities", Field: "ISIN coverage",
		Legacy: "see company_details",
		Platform: fmt.Sprintf("%d/%d with ISIN, %d valid, %d flagged",
			withISIN, totalSecurities, validISIN, badChecksum),
		Match:    badChecksum == 0,
		Severity: "warning",
		Note:     "invalid checksums are stored and flagged rather than silently dropped",
	})

	// Every symbol the exchange lists must be addressable on the platform. The
	// legacy system had no securities table at all, so the source of truth for
	// this check is the captured security list, not the database.
	body, err = os.ReadFile(filepath.Join(opts.DataDir, "security_list.json"))
	if err != nil {
		return nil
	}
	var lists []struct {
		Symbol string `json:"symbol"`
	}
	if jsonErr := json.Unmarshal(body, &lists); jsonErr != nil {
		return nil
	}
	wantSyms := map[string]bool{}
	for _, s := range lists {
		if sym := strings.ToUpper(strings.TrimSpace(s.Symbol)); sym != "" {
			wantSyms[sym] = true
		}
	}
	rows, err := opts.Pool.Query(ctx, `SELECT symbol FROM securities`)
	if err != nil {
		return err
	}
	haveSyms := map[string]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		haveSyms[s] = true
	}
	rows.Close()
	missingSyms := 0
	for s := range wantSyms {
		if !haveSyms[s] {
			missingSyms++
		}
	}
	rep.Checks = append(rep.Checks, Check{
		Dataset: "securities", Field: "symbol set",
		Legacy:   fmt.Sprintf("%d symbols in security_list.json", len(wantSyms)),
		Platform: fmt.Sprintf("%d missing", missingSyms),
		Match:    missingSyms == 0,
		Severity: "critical",
		Note: "every listed security, including promoter, preference, debenture and " +
			"mutual fund classes, must have a securities row",
	})

	// Contracts that could not be attributed to a security would vanish from any
	// per-security view while still existing in the table.
	var tradesTotal, tradesAttached int64
	if err := opts.Pool.QueryRow(ctx,
		`SELECT count(*), count(security_id) FROM floorsheet_trades`).Scan(&tradesTotal, &tradesAttached); err != nil {
		return err
	}
	rep.Checks = append(rep.Checks, Check{
		Dataset: "securities", Field: "contract attribution",
		Legacy:   strconv.FormatInt(tradesTotal, 10) + " contracts",
		Platform: strconv.FormatInt(tradesAttached, 10) + " attributed to a security",
		Match:    tradesAttached == tradesTotal,
		Severity: "critical",
		Note:     "a contract whose symbol has no security row is still stored but invisible to per-security analytics",
	})
	return nil
}

// ---------------------------------------------------------------- daily bars

func compareBars(ctx context.Context, opts Options, rep *Report) error {
	if opts.DataDir == "" {
		return nil
	}
	files, err := filepath.Glob(filepath.Join(opts.DataDir, "company_details", "*.json"))
	if err != nil {
		return err
	}
	captured := 0
	for _, f := range files {
		if strings.HasSuffix(strings.TrimSuffix(filepath.Base(f), ".json"), "_extras") {
			continue
		}
		captured++
	}

	var platformBars int64
	if err := opts.Pool.QueryRow(ctx, `SELECT count(*) FROM daily_ohlcv`).Scan(&platformBars); err != nil {
		return err
	}
	rep.Datasets = append(rep.Datasets, DatasetResult{
		Name:          "daily_ohlcv",
		LegacyCount:   0,
		PlatformCount: platformBars,
		Match:         false,
		Expected:      true,
		Note: fmt.Sprintf("the legacy database never stored daily prices (0 rows) while %d company detail snapshots were captured; "+
			"the platform derives one bar per snapshot", captured),
	})
	rep.Checks = append(rep.Checks, Check{
		Dataset: "daily_ohlcv", Field: "rows captured from snapshots",
		Legacy: "0 (daily_prices table empty)", Platform: strconv.FormatInt(platformBars, 10),
		Match:    platformBars >= int64(captured)-5,
		Expected: true, Severity: "info",
		Note: "platform-only dataset: the legacy system observed prices and discarded them",
	})

	// Close price and session volume must match the snapshot the bar was built
	// from: those two columns are the exchange's own figures.
	var checked, bad, badVolume int
	for i, f := range files {
		if i >= 200 {
			break
		}
		if strings.HasSuffix(strings.TrimSuffix(filepath.Base(f), ".json"), "_extras") {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var doc struct {
			Symbol  string `json:"symbol"`
			Details struct {
				Quote struct {
					LTP          *float64 `json:"lastTradedPrice"`
					Volume       *float64 `json:"totalTradeQuantity"`
					BusinessDate string   `json:"businessDate"`
				} `json:"securityDailyTradeDto"`
			} `json:"details"`
		}
		if err := json.Unmarshal(body, &doc); err != nil {
			continue
		}
		symbol := strings.ToUpper(strings.TrimSpace(doc.Symbol))
		if symbol == "" || doc.Details.Quote.LTP == nil {
			continue
		}
		checked++
		var closeVal, volumeVal float64
		err = opts.Pool.QueryRow(ctx, `
			SELECT o.close::float8, COALESCE(o.volume, 0)::float8 FROM daily_ohlcv o
			JOIN securities s ON s.id = o.security_id
			WHERE s.symbol = $1 AND o.trading_date = $2::date`,
			symbol, doc.Details.Quote.BusinessDate).Scan(&closeVal, &volumeVal)
		if err != nil || !closeEnough(*doc.Details.Quote.LTP, closeVal, 0.0001) {
			bad++
		}
		if doc.Details.Quote.Volume != nil && !closeEnough(*doc.Details.Quote.Volume, volumeVal, 0.0001) {
			badVolume++
		}
	}
	rep.Checks = append(rep.Checks, Check{
		Dataset: "daily_ohlcv", Field: "close price sample",
		Legacy:   fmt.Sprintf("%d snapshots", checked),
		Platform: fmt.Sprintf("%d mismatched closes", bad),
		Match:    bad == 0 && checked > 0,
		Severity: "critical",
		Note:     "each bar's close is compared with the last traded price in the snapshot it came from",
	})
	rep.Checks = append(rep.Checks, Check{
		Dataset: "daily_ohlcv", Field: "volume sample",
		Legacy:   fmt.Sprintf("%d snapshots", checked),
		Platform: fmt.Sprintf("%d mismatched volumes", badVolume),
		Match:    badVolume == 0 && checked > 0,
		Severity: "critical",
		Note:     "session volume is the snapshot's totalTradeQuantity; contracts are not used for it because their capture is incomplete",
	})
	return nil
}

// compareCaptureCoverage measures how much of the exchange's reported turnover is
// represented by captured contracts.
//
// This is not a migration defect: the legacy database holds exactly the same
// contracts. It is a property of the source — a once-per-second poll cannot see
// every trade — and it belongs in the report because a turnover-per-security
// figure derived from contracts is a lower bound, not a total.
func compareCaptureCoverage(ctx context.Context, pool *pgxpool.Pool, rep *Report) error {
	rows, err := pool.Query(ctx, `
		SELECT s.trading_date::text,
		       COALESCE(s.turnover, 0)::float8,
		       COALESCE((SELECT sum(f.amount)::float8 FROM floorsheet_trades f
		                 WHERE f.trading_date = s.trading_date), 0)
		FROM market_summaries s
		WHERE s.turnover IS NOT NULL AND s.turnover > 0
		ORDER BY s.trading_date`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var date string
		var reported, captured float64
		if err := rows.Scan(&date, &reported, &captured); err != nil {
			return err
		}
		ratio := 0.0
		if reported > 0 {
			ratio = captured / reported * 100
		}
		rep.add(Check{
			Dataset: "floorsheet", Field: "capture coverage " + date,
			Legacy:   fmt.Sprintf("exchange turnover %.2f", reported),
			Platform: fmt.Sprintf("captured contracts %.2f (%.1f%%)", captured, ratio),
			Match:    false,
			Expected: true,
			Severity: "warning",
			Note: "snapshot polling does not see every trade; per-security turnover is therefore a lower bound " +
				"and session turnover is taken from the exchange summary, not from contracts",
		})
	}
	return rows.Err()
}

// compareEmptyLegacyTables records tables that were empty on the legacy side, so
// their absence in the platform is not mistaken for data loss.
func compareEmptyLegacyTables(ctx context.Context, db *sql.DB, pool *pgxpool.Pool, rep *Report) {
	for _, table := range []string{"daily_prices", "index_intraday"} {
		var n int64
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			continue
		}
		if n != 0 {
			continue
		}
		rep.Datasets = append(rep.Datasets, DatasetResult{
			Name:          table,
			LegacyCount:   0,
			PlatformCount: 0,
			Match:         true,
			Expected:      true,
			Note:          "empty in the legacy database; nothing to migrate",
		})
	}
}

// ---------------------------------------------------------------- helpers

func (r *Report) add(c Check) { r.Checks = append(r.Checks, c) }

func countCheck(rep *Report, dataset, field string, legacy, platform int64) {
	rep.add(Check{
		Dataset: dataset, Field: field,
		Legacy:   strconv.FormatInt(legacy, 10),
		Platform: strconv.FormatInt(platform, 10),
		Match:    legacy == platform,
		Severity: "critical",
	})
}

func approxCheck(rep *Report, dataset, field string, legacy, platform, tolerance float64) {
	rep.add(Check{
		Dataset: dataset, Field: field,
		Legacy:   strconv.FormatFloat(legacy, 'f', 4, 64),
		Platform: strconv.FormatFloat(platform, 'f', 4, 64),
		Match:    closeEnough(legacy, platform, tolerance),
		Severity: "critical",
	})
}

func closeEnough(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func openLegacy(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("no legacy database path configured")
	}
	abs := path
	if !filepath.IsAbs(abs) {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		abs = filepath.Join(wd, path)
	}
	q := url.Values{}
	q.Set("mode", "ro")
	q.Set("_pragma", "query_only(1)")
	db, err := sql.Open("sqlite", "file:"+abs+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open legacy database: %w", err)
	}
	return db, nil
}

func parseLegacyUTC(s string) *time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05", time.RFC3339Nano, time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}

func nullStr(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return strings.TrimSpace(v.String)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 4, 64) }

// sortedKeys is used by report rendering.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
