package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nepse/terminal/api/internal/marketdata"
)

// streamSpec describes how one stream becomes SQL.
//
// cols is the staging order; normalize must return exactly that many values and
// keyIdx names the positions that form the natural key within a chunk.
type streamSpec struct {
	stream    string
	stageDDL  string
	stageName string
	truncate  string
	cols      []string
	keyIdx    []int
	normalize func(map[string]any) ([]any, error)
	// prep runs before the merge in the same transaction, for the rows the merge
	// depends on (sectors, sessions, index definitions).
	prep func(ctx context.Context, tx pgx.Tx, rows [][]any) error
	// merge is the upsert. When usesSource is set, $1 is the resolved
	// data_source_id; tables without a provenance column take no argument.
	merge      string
	usesSource bool
	// post runs after each chunk's merge and returns how many rows it recomputed.
	post func(ctx context.Context, tx pgx.Tx, rows [][]any, srcID string) (int, error)
	// finalize runs once after the last chunk, for work that must see the whole
	// stream (an aggregate over every contract of a session, not one page of it).
	finalize func(ctx context.Context, tx pgx.Tx) (int, error)
	// orphanCheck returns rows the merge silently skipped because a join failed.
	// They are reported as rejections instead of vanishing.
	orphanCheck string
}

type fieldError struct {
	field string
	err   error
}

func (e fieldError) Error() string { return e.field + ": " + e.err.Error() }
func (e fieldError) Unwrap() error { return e.err }

func ferr(field, format string, args ...any) error {
	return fieldError{field: field, err: fmt.Errorf(format, args...)}
}

func fieldOf(err error) string {
	var fe fieldError
	if errors.As(err, &fe) {
		return fe.field
	}
	return ""
}

func reasonOf(err error) string {
	var fe fieldError
	if errors.As(err, &fe) {
		return fe.err.Error()
	}
	return err.Error()
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	out := nonSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "_")
	return strings.Trim(out, "_")
}

func str(row map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func pick(row map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func sampleOf(row map[string]any) map[string]any {
	out := make(map[string]any, 8)
	n := 0
	for k, v := range row {
		if n >= 6 {
			break
		}
		if s, ok := v.(string); ok && len(s) > 120 {
			v = s[:120] + "…"
		}
		out[k] = v
		n++
	}
	return out
}

// indexCode derives a stable code from an index name ("NEPSE Index" → "nepse",
// "Banking SubIndex" → "banking").
func indexCode(name string) string {
	base := strings.TrimSuffix(strings.TrimSpace(name), " Index")
	base = strings.ReplaceAll(base, "SubIndex", "")
	base = strings.ReplaceAll(base, "Sub-Index", "")
	if c := slug(base); c != "" {
		return c
	}
	return "index"
}

func indexKind(name string) string {
	n := strings.ToLower(name)
	if strings.Contains(n, "subindex") || strings.Contains(n, "sub-index") {
		return "sector"
	}
	return "headline"
}

// specs is the registry the engine dispatches on.
var specs = map[string]*streamSpec{
	marketdata.StreamCompanies:    companiesSpec(),
	marketdata.StreamSecurities:   securitiesSpec(),
	marketdata.StreamQuotes:       quotesSpec(),
	marketdata.StreamMarketCap:    marketCapSpec(),
	marketdata.StreamIndices:      indicesSpec(),
	marketdata.StreamFloorsheet:   floorsheetSpec(),
	marketdata.StreamMarketStatus: marketStatusSpec(),
}

// ---------------------------------------------------------------- companies

func companiesSpec() *streamSpec {
	return &streamSpec{
		stream:    marketdata.StreamCompanies,
		stageName: "st_companies",
		stageDDL: `CREATE TEMP TABLE st_companies (
			symbol text, name text, sector_code text, sector_name text,
			status text, instrument text) ON COMMIT DROP`,
		truncate: "TRUNCATE st_companies",
		cols:     []string{"symbol", "name", "sector_code", "sector_name", "status", "instrument"},
		keyIdx:   []int{0},
		normalize: func(row map[string]any) ([]any, error) {
			sym, err := marketdata.Symbol(str(row, "symbol"))
			if err != nil {
				return nil, ferr("symbol", "%v", err)
			}
			name := str(row, "companyName", "securityName", "name")
			if name == "" {
				name = sym
			}
			sector := str(row, "sectorName")
			if sector == "" {
				sector = "Unclassified"
			}
			status := "active"
			if s := str(row, "status", "activeStatus"); s != "" && s != "A" {
				status = "inactive"
			}
			instrument := strings.ToLower(str(row, "instrumentType"))
			if instrument == "" {
				instrument = "equity"
			}
			return []any{sym, name, slug(sector), sector, status, instrument}, nil
		},
		prep: func(ctx context.Context, tx pgx.Tx, rows [][]any) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO sectors (code, name)
				SELECT DISTINCT s.sector_code, s.sector_name FROM st_companies s
				ON CONFLICT (code) DO NOTHING`)
			return err
		},
		usesSource: false,
		merge: `
			INSERT INTO companies (symbol, name, sector_id, status, instrument)
			SELECT s.symbol, s.name, sec.id, s.status, s.instrument
			FROM st_companies s JOIN sectors sec ON sec.code = s.sector_code
			ON CONFLICT (symbol) DO UPDATE SET
				name = EXCLUDED.name, sector_id = EXCLUDED.sector_id,
				status = EXCLUDED.status, instrument = EXCLUDED.instrument,
				updated_at = now()
			WHERE (companies.name, companies.sector_id, companies.status, companies.instrument)
			      IS DISTINCT FROM
			      (EXCLUDED.name, EXCLUDED.sector_id, EXCLUDED.status, EXCLUDED.instrument)`,
		post: func(ctx context.Context, tx pgx.Tx, rows [][]any, srcID string) (int, error) {
			// Every company needs a tradable security row for FK integrity, even
			// before a quote arrives. Existing rows are left alone.
			_, err := tx.Exec(ctx, `
				INSERT INTO securities (company_id, symbol, security_type)
				SELECT c.id, s.symbol, 'ordinary'
				FROM st_companies s JOIN companies c ON c.symbol = s.symbol
				ON CONFLICT (symbol) DO NOTHING`)
			if err != nil {
				return 0, err
			}

			// A placeholder issuer is superseded the moment its registered company
			// arrives. Without this, a run that ingested securities before companies
			// would leave a duplicate issuer behind, and the company count would
			// drift by one per class for as long as the placeholder survived.
			if _, err := tx.Exec(ctx, `
				UPDATE securities sec SET company_id = r.id
				FROM companies r, companies ph
				WHERE ph.id = sec.company_id
				  AND ph.origin = 'listed_security'
				  AND r.origin = 'registry'
				  AND r.symbol = class_parent_symbol(sec.symbol)
				  AND sec.security_type IN ('promoter', 'preference')`); err != nil {
				return 0, err
			}
			if _, err := tx.Exec(ctx, `
				DELETE FROM companies ph
				WHERE ph.origin = 'listed_security'
				  AND NOT EXISTS (SELECT 1 FROM securities s WHERE s.company_id = ph.id)`); err != nil {
				return 0, err
			}
			return 0, nil
		},
	}
}

// ---------------------------------------------------------------- securities

func securitiesSpec() *streamSpec {
	return &streamSpec{
		stream:    marketdata.StreamSecurities,
		stageName: "st_securities",
		stageDDL: `CREATE TEMP TABLE st_securities (
			symbol text, name text, active boolean, security_type text) ON COMMIT DROP`,
		truncate: "TRUNCATE st_securities",
		cols:     []string{"symbol", "name", "active", "security_type"},
		keyIdx:   []int{0},
		normalize: func(row map[string]any) ([]any, error) {
			sym, err := marketdata.Symbol(str(row, "symbol"))
			if err != nil {
				return nil, ferr("symbol", "%v", err)
			}
			name := str(row, "securityName", "name")
			active := str(row, "activeStatus") == "A"
			return []any{sym, name, active, securityTypeOf(name)}, nil
		},
		prep: func(ctx context.Context, tx pgx.Tx, rows [][]any) error {
			// The exchange lists classes (promoter shares, preference shares,
			// debentures, mutual fund units) whose issuer is absent from the
			// company registry, which only carries ordinary equities. Dropping them
			// would strand real trades: captured contracts reference symbols in
			// this list. A placeholder issuer keeps the reference graph complete and
			// keeps their contracts attributable to a security.
			//
			// A placeholder is only created when the class-stripped symbol does not
			// resolve to a registered company: most promoter classes (…P, …PO) trade
			// under their issuer's ordinary symbol plus a suffix, and inventing a
			// second issuer for them would double-count the company.
			if _, err := tx.Exec(ctx, `
				INSERT INTO sectors (code, name) VALUES ('unclassified', 'Unclassified')
				ON CONFLICT (code) DO NOTHING`); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO companies (symbol, name, sector_id, status, instrument, origin)
				SELECT s.symbol,
				       COALESCE(NULLIF(s.name, ''), s.symbol),
				       (SELECT id FROM sectors WHERE code = 'unclassified'),
				       CASE WHEN s.active THEN 'active' ELSE 'inactive' END,
				       'equity',
				       'listed_security'
				FROM st_securities s
				LEFT JOIN companies c ON c.symbol = s.symbol
				LEFT JOIN companies par ON par.symbol = class_parent_symbol(s.symbol)
				       AND s.security_type IN ('promoter', 'preference')
				WHERE c.id IS NULL AND par.id IS NULL
				ON CONFLICT (symbol) DO NOTHING`)
			return err
		},
		usesSource: false,
		// A class security does not need an issuer row of its own symbol: it is
		// attached to the issuer of its ordinary class. Requiring c.id would drop
		// every promoter share whose own symbol is not itself a company.
		merge: `
			INSERT INTO securities (company_id, symbol, security_type)
			SELECT COALESCE(par.id, c.id), s.symbol, s.security_type
			FROM st_securities s
			LEFT JOIN companies c ON c.symbol = s.symbol
			LEFT JOIN companies par ON par.symbol = class_parent_symbol(s.symbol)
			       AND s.security_type IN ('promoter', 'preference')
			WHERE COALESCE(par.id, c.id) IS NOT NULL
			ON CONFLICT (symbol) DO UPDATE SET
				company_id = EXCLUDED.company_id,
				security_type = EXCLUDED.security_type
			WHERE (securities.company_id, securities.security_type)
			      IS DISTINCT FROM (EXCLUDED.company_id, EXCLUDED.security_type)`,
		orphanCheck: `
			SELECT count(*) FROM st_securities s
			LEFT JOIN companies c ON c.symbol = s.symbol
			LEFT JOIN companies par ON par.symbol = class_parent_symbol(s.symbol)
			       AND s.security_type IN ('promoter', 'preference')
			WHERE COALESCE(par.id, c.id) IS NULL`,
	}
}

// securityTypeOf classifies a listed security from the exchange's own name.
//
// The class is never inferred from a symbol suffix ("…P", "…PO"): those are
// inconsistent across issuers, and a wrong instrument class silently distorts
// any count grouped by it. An unrecognised name is recorded as unclassified
// rather than guessed as ordinary equity.
func securityTypeOf(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "promoter"), strings.Contains(n, "promotor"):
		// The exchange uses both spellings, inconsistently: "NABIL Bank Limited
		// Promotor Share" sits beside "Sanima Bank Ltd. Promoter Share".
		return "promoter"
	case strings.Contains(n, "preference"):
		return "preference"
	case strings.Contains(n, "debenture"):
		return "debenture"
	case strings.Contains(n, "mutual fund") || strings.Contains(n, "mutualfund"),
		strings.Contains(n, "investment fund"),
		strings.Contains(n, "scheme"):
		// Fund units are rarely named "mutual fund"; the exchange calls them
		// "Investment Fund - 1" or "Growth Scheme 2".
		return "mutual_fund"
	case strings.Contains(n, "bond"):
		return "bond"
	case strings.Contains(n, "right share") || strings.Contains(n, "rightshare"):
		return "right"
	case n == "":
		return "unclassified"
	default:
		return "ordinary"
	}
}

// ---------------------------------------------------------------- quotes

func quotesSpec() *streamSpec {
	return &streamSpec{
		stream:    marketdata.StreamQuotes,
		stageName: "st_quotes",
		stageDDL: `CREATE TEMP TABLE st_quotes (
			symbol text, trading_date date,
			open numeric(18,4), high numeric(18,4), low numeric(18,4), close numeric(18,4),
			previous_close numeric(18,4), volume numeric(24,4), transactions bigint,
			isin text, isin_ok boolean, listed numeric(24,4), public numeric(24,4),
			reported_cap numeric(28,4)
		) ON COMMIT DROP`,
		truncate: "TRUNCATE st_quotes",
		cols: []string{"symbol", "trading_date", "open", "high", "low", "close",
			"previous_close", "volume", "transactions", "isin", "isin_ok", "listed", "public",
			"reported_cap"},
		keyIdx: []int{0, 1},
		normalize: func(row map[string]any) ([]any, error) {
			sym, err := marketdata.Symbol(str(row, "symbol"))
			if err != nil {
				return nil, ferr("symbol", "%v", err)
			}
			date, err := marketdata.Date(pick(row, "business_date", "businessDate"))
			if err != nil {
				return nil, ferr("business_date", "%v", err)
			}
			open, err := marketdata.OptionalDecimal(pick(row, "open", "openPrice"))
			if err != nil {
				return nil, ferr("open", "%v", err)
			}
			high, err := marketdata.OptionalDecimal(pick(row, "high", "highPrice"))
			if err != nil {
				return nil, ferr("high", "%v", err)
			}
			low, err := marketdata.OptionalDecimal(pick(row, "low", "lowPrice"))
			if err != nil {
				return nil, ferr("low", "%v", err)
			}
			prev, err := marketdata.OptionalDecimal(pick(row, "previous_close", "previousClose"))
			if err != nil {
				return nil, ferr("previous_close", "%v", err)
			}
			volume, err := marketdata.OptionalDecimal(pick(row, "volume", "totalTradeQuantity"))
			if err != nil {
				return nil, ferr("volume", "%v", err)
			}
			var transactions *int
			if v := pick(row, "trades", "totalTrades"); v != nil {
				if n, err := marketdata.Integer(v); err == nil {
					transactions = &n
				}
			}

			// A close is required: a bar with no close is not an observation. The
			// last traded price is used, falling back to the open of a suspended
			// scrip, which is what the exchange reports for it.
			closeVal := pick(row, "ltp", "lastTradedPrice")
			if closeVal == nil {
				// open has already been normalised, so unwrap it: Decimal takes a
				// scalar, not a *string.
				if open != nil {
					closeVal = *open
				}
			}
			if closeVal == nil {
				return nil, ferr("ltp", "no close price and no open price")
			}
			closeDec, err := marketdata.DecimalString(closeVal)
			if err != nil {
				return nil, ferr("ltp", "%v", err)
			}
			// A zero is an absence, not a price. The feed fills 0 for promoter
			// classes that have never traded; storing that bar would put a zero
			// close in the ranking and a zero capitalisation on the company page.
			// The security stays listed; it simply has no observation.
			if zeroDecimal(closeDec) {
				return nil, ferr("ltp", "price is zero; the security has no observation")
			}

			isin := ""
			isinOK := false
			if raw := str(row, "isin"); raw != "" {
				if v, ok := marketdata.NormalizeISIN(raw); ok {
					isin = v.Value
					isinOK = v.CheckDigit
				}
				// A short issuer code in the isin field is not an ISIN; storing it
				// would poison the unique index with a fake identifier.
			}

			listed, err := marketdata.OptionalDecimal(pick(row, "listed_shares"))
			if err != nil {
				return nil, ferr("listed_shares", "%v", err)
			}
			public, err := marketdata.OptionalDecimal(pick(row, "public_shares"))
			if err != nil {
				return nil, ferr("public_shares", "%v", err)
			}
			if listed == nil && public == nil {
				// Not fatal: reference data may simply be missing for this symbol.
			}

			// Capitalisation is published beside the trade block. Keeping the
			// reported value lets a company screen use a measurement instead of
			// multiplying close by shares and calling the result the same thing.
			reportedCap, err := marketdata.OptionalDecimal(pick(row, "market_cap", "marketCapitalization"))
			if err != nil {
				return nil, ferr("market_cap", "%v", err)
			}

			var trx any
			if transactions != nil {
				trx = *transactions
			}
			return []any{sym, date, open, high, low, closeDec, prev, volume, trx,
				nullString(isin), nullBool(isin, isinOK), listed, public, reportedCap}, nil
		},
		prep: func(ctx context.Context, tx pgx.Tx, rows [][]any) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO trading_sessions (trading_date)
				SELECT DISTINCT s.trading_date FROM st_quotes s
				ON CONFLICT DO NOTHING`)
			return err
		},
		usesSource: true,
		merge: `
			INSERT INTO daily_ohlcv (security_id, trading_date, open, high, low, close,
				previous_close, volume, transactions, market_cap, data_source_id)
			SELECT sec.id, s.trading_date, s.open, s.high, s.low, s.close,
			       s.previous_close, s.volume, s.transactions, s.reported_cap, $1::uuid
			FROM st_quotes s JOIN securities sec ON sec.symbol = s.symbol
			ON CONFLICT (security_id, trading_date) DO UPDATE SET
				open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low,
				close = EXCLUDED.close, previous_close = EXCLUDED.previous_close,
				volume = EXCLUDED.volume, transactions = EXCLUDED.transactions,
				market_cap = COALESCE(EXCLUDED.market_cap, daily_ohlcv.market_cap),
				data_source_id = EXCLUDED.data_source_id, ingested_at = now()
			WHERE (daily_ohlcv.open, daily_ohlcv.high, daily_ohlcv.low, daily_ohlcv.close,
			       daily_ohlcv.previous_close, daily_ohlcv.volume, daily_ohlcv.transactions,
			       daily_ohlcv.market_cap)
			      IS DISTINCT FROM
			      (EXCLUDED.open, EXCLUDED.high, EXCLUDED.low, EXCLUDED.close,
			       EXCLUDED.previous_close, EXCLUDED.volume, EXCLUDED.transactions,
			       COALESCE(EXCLUDED.market_cap, daily_ohlcv.market_cap))`,
		post: func(ctx context.Context, tx pgx.Tx, rows [][]any, srcID string) (int, error) {
			// Identifiers and share counts are reference data that arrives with the
			// quote; writing them here keeps securities self-describing.
			tag, err := tx.Exec(ctx, `
				UPDATE securities sec SET
					isin = COALESCE(NULLIF(s.isin, ''), sec.isin),
					isin_checksum_ok = COALESCE(s.isin_ok, sec.isin_checksum_ok),
					listed_shares = COALESCE(s.listed, sec.listed_shares),
					public_shares = COALESCE(s.public, sec.public_shares),
					isin_source_id = $1::uuid
				FROM st_quotes s
				WHERE sec.symbol = s.symbol
				  AND (sec.isin IS DISTINCT FROM COALESCE(NULLIF(s.isin, ''), sec.isin)
				    OR sec.isin_checksum_ok IS DISTINCT FROM COALESCE(s.isin_ok, sec.isin_checksum_ok)
				    OR sec.listed_shares IS DISTINCT FROM COALESCE(s.listed, sec.listed_shares)
				    OR sec.public_shares IS DISTINCT FROM COALESCE(s.public, sec.public_shares))`, srcID)
			if err != nil {
				return 0, err
			}
			return int(tag.RowsAffected()), nil
		},
		orphanCheck: `
			SELECT count(*) FROM st_quotes s
			LEFT JOIN securities sec ON sec.symbol = s.symbol
			WHERE sec.id IS NULL`,
	}
}

// ---------------------------------------------------------------- market cap

func marketCapSpec() *streamSpec {
	return &streamSpec{
		stream:    marketdata.StreamMarketCap,
		stageName: "st_market_cap",
		stageDDL: `CREATE TEMP TABLE st_market_cap (
			trading_date date, market_cap numeric(28,4), float_market_cap numeric(28,4),
			covered integer, eligible integer) ON COMMIT DROP`,
		truncate: "TRUNCATE st_market_cap",
		cols:     []string{"trading_date", "market_cap", "float_market_cap", "covered", "eligible"},
		keyIdx:   []int{0},
		normalize: func(row map[string]any) ([]any, error) {
			date, err := marketdata.Date(pick(row, "business_date", "trading_date"))
			if err != nil {
				return nil, ferr("business_date", "%v", err)
			}
			cap, err := marketdata.OptionalDecimal(pick(row, "market_cap"))
			if err != nil {
				return nil, ferr("market_cap", "%v", err)
			}
			if cap == nil {
				return nil, ferr("market_cap", "capitalisation is missing")
			}
			floatCap, err := marketdata.OptionalDecimal(pick(row, "float_market_cap"))
			if err != nil {
				return nil, ferr("float_market_cap", "%v", err)
			}
			var covered, eligible any
			if v := pick(row, "covered"); v != nil {
				if n, err := marketdata.Integer(v); err == nil {
					covered = n
				}
			}
			if v := pick(row, "total"); v != nil {
				if n, err := marketdata.Integer(v); err == nil {
					eligible = n
				}
			}
			return []any{date, cap, floatCap, covered, eligible}, nil
		},
		prep: func(ctx context.Context, tx pgx.Tx, rows [][]any) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO trading_sessions (trading_date)
				SELECT DISTINCT s.trading_date FROM st_market_cap s
				ON CONFLICT DO NOTHING`)
			return err
		},
		// The exchange's own reported figure replaces the derived one: an
		// observation outranks an estimate, and the method is recorded with it.
		usesSource: true,
		merge: `
			INSERT INTO market_summaries (trading_date, market_cap, float_market_cap,
				cap_covered, cap_eligible, cap_method, data_source_id)
			SELECT s.trading_date, s.market_cap, s.float_market_cap, s.covered, s.eligible,
			       'issuer-reported snapshot', $1::uuid
			FROM st_market_cap s
			ON CONFLICT (trading_date) DO UPDATE SET
				market_cap = EXCLUDED.market_cap,
				float_market_cap = EXCLUDED.float_market_cap,
				cap_covered = EXCLUDED.cap_covered,
				cap_eligible = EXCLUDED.cap_eligible,
				cap_method = EXCLUDED.cap_method,
				data_source_id = EXCLUDED.data_source_id,
				ingested_at = now()
			WHERE (market_summaries.market_cap, market_summaries.float_market_cap,
			       market_summaries.cap_covered, market_summaries.cap_eligible)
			      IS DISTINCT FROM
			      (EXCLUDED.market_cap, EXCLUDED.float_market_cap,
			       EXCLUDED.cap_covered, EXCLUDED.cap_eligible)`,
	}
}

// ---------------------------------------------------------------- indices

func indicesSpec() *streamSpec {
	return &streamSpec{
		stream:    marketdata.StreamIndices,
		stageName: "st_indices",
		stageDDL: `CREATE TEMP TABLE st_indices (
			code text, name text, kind text, trading_date date, observed_at timestamptz,
			value numeric(18,4), point_change numeric(18,4), percent_change numeric(10,4)
		) ON COMMIT DROP`,
		truncate: "TRUNCATE st_indices",
		cols: []string{"code", "name", "kind", "trading_date", "observed_at",
			"value", "point_change", "percent_change"},
		keyIdx: []int{0, 4},
		normalize: func(row map[string]any) ([]any, error) {
			name := str(row, "index", "index_name")
			if name == "" {
				return nil, ferr("index", "index name is missing")
			}
			kind := indexKind(name)
			if k := str(row, "kind"); k == "headline" || k == "sector" {
				kind = k
			}

			// Snapshot rows carry an NPT wall clock; legacy rows carry a UTC date.
			var date string
			if raw := pick(row, "business_date"); raw != nil {
				d, err := marketdata.Date(raw)
				if err != nil {
					return nil, ferr("business_date", "%v", err)
				}
				date = d
			} else {
				t, err := marketdata.Timestamp(pick(row, "generatedTime", "captured_at"), marketdata.BasisNPT)
				if err != nil || t == nil {
					return nil, ferr("generatedTime", "no trading date on the row")
				}
				date = t.UTC().Format("2006-01-02")
			}

			// An observation's identity is when it was observed. Two adapters can
			// legitimately report the same index on the same day at different times;
			// collapsing them onto midnight would make them overwrite each other on
			// every run instead of being two points in a series.
			observed, _ := marketdata.Timestamp(pick(row, "captured_at"), marketdata.BasisUTC)
			if observed == nil {
				observed, _ = marketdata.Timestamp(pick(row, "generatedTime"), marketdata.BasisNPT)
			}
			if observed == nil {
				midnight, err := time.Parse("2006-01-02", date)
				if err != nil {
					return nil, ferr("observed_at", "no observation time and no trading date")
				}
				observed = &midnight
			}

			value := pick(row, "currentValue", "value", "close")
			dec, err := marketdata.DecimalString(value)
			if err != nil {
				return nil, ferr("value", "%v", err)
			}
			change, err := marketdata.OptionalDecimal(pick(row, "change", "point_change"))
			if err != nil {
				return nil, ferr("change", "%v", err)
			}
			per, err := marketdata.OptionalDecimal(pick(row, "perChange", "per_change"))
			if err != nil {
				return nil, ferr("perChange", "%v", err)
			}
			return []any{indexCode(name), name, kind, date, observed, dec, change, per}, nil
		},
		prep: func(ctx context.Context, tx pgx.Tx, rows [][]any) error {
			if _, err := tx.Exec(ctx, `
				INSERT INTO trading_sessions (trading_date)
				SELECT DISTINCT s.trading_date FROM st_indices s
				ON CONFLICT DO NOTHING`); err != nil {
				return err
			}
			// Kind is never downgraded. The same index is described by several
			// sources: sub_indices.json marks a sector index as `sector`, while a
			// symbol listing that omits the kind defaults it to `headline`. Letting
			// that default win reclassifies twelve sector indices as headline ones,
			// which then show up in the headline table on the overview.
			_, err := tx.Exec(ctx, `
				INSERT INTO index_definitions (code, name, kind)
				SELECT DISTINCT s.code, s.name, s.kind FROM st_indices s
				ON CONFLICT (code) DO UPDATE SET
					name = EXCLUDED.name,
					kind = CASE WHEN index_definitions.kind = 'sector' OR EXCLUDED.kind = 'sector'
					            THEN 'sector' ELSE 'headline' END`)
			return err
		},
		usesSource: false,
		merge: `
			INSERT INTO index_observations (index_id, trading_date, granularity, observed_at,
				value, point_change, percent_change)
			SELECT d.id, s.trading_date, 'daily', s.observed_at,
			       s.value, s.point_change, s.percent_change
			FROM st_indices s JOIN index_definitions d ON d.code = s.code
			ON CONFLICT (index_id, trading_date, granularity, observed_at) DO UPDATE SET
				value = EXCLUDED.value,
				point_change = EXCLUDED.point_change,
				percent_change = EXCLUDED.percent_change
			WHERE (index_observations.value, index_observations.point_change, index_observations.percent_change)
			      IS DISTINCT FROM
			      (EXCLUDED.value, EXCLUDED.point_change, EXCLUDED.percent_change)`,
	}
}

// ---------------------------------------------------------------- floorsheet

func floorsheetSpec() *streamSpec {
	return &streamSpec{
		stream:    marketdata.StreamFloorsheet,
		stageName: "st_floorsheet",
		stageDDL: `CREATE TEMP TABLE st_floorsheet (
			contract_id numeric(30,0), trading_date date, symbol text,
			buyer text, seller text, buyer_id text, seller_id text,
			quantity numeric(20,4), rate numeric(18,6),
			amount numeric(24,4), executed_at timestamptz, raw jsonb
		) ON COMMIT DROP`,
		truncate: "TRUNCATE st_floorsheet",
		cols: []string{"contract_id", "trading_date", "symbol", "buyer", "seller",
			"buyer_id", "seller_id", "quantity", "rate", "amount", "executed_at", "raw"},
		keyIdx: []int{0},
		normalize: func(row map[string]any) ([]any, error) {
			contract := str(row, "contract_id", "contract_no")
			if contract == "" {
				return nil, ferr("contract_id", "contract id is missing")
			}
			if _, err := strconv.ParseUint(contract, 10, 64); err != nil {
				return nil, ferr("contract_id", "%q is not numeric", contract)
			}
			date, err := marketdata.Date(pick(row, "business_date"))
			if err != nil {
				return nil, ferr("business_date", "%v", err)
			}
			sym := strings.ToUpper(str(row, "symbol"))
			if sym == "" {
				return nil, ferr("symbol", "symbol is missing")
			}
			qty, err := marketdata.PositiveDecimal(pick(row, "quantity", "contractQuantity"), "quantity")
			if err != nil {
				return nil, err
			}
			rate, err := marketdata.PositiveDecimal(pick(row, "rate", "contractRate"), "rate")
			if err != nil {
				return nil, err
			}
			amount, err := marketdata.OptionalDecimal(pick(row, "amount", "contractAmount"))
			if err != nil {
				return nil, ferr("amount", "%v", err)
			}
			if amount == nil {
				// The exchange omits contractAmount on some captures; quantity × rate
				// is exact for a single contract, unlike a session-level estimate.
				q, _ := strconv.ParseFloat(qty, 64)
				r, _ := strconv.ParseFloat(rate, 64)
				derived := strconv.FormatFloat(q*r, 'f', 4, 64)
				amount = &derived
			}
			// Legacy timestamps are stored in UTC despite having no zone marker.
			executedAt, err := marketdata.Timestamp(pick(row, "trade_time", "tradeTime"), marketdata.BasisUTC)
			if err != nil {
				return nil, ferr("trade_time", "%v", err)
			}
			raw := "{}"
			if s := str(row, "raw"); s != "" && json.Valid([]byte(s)) {
				raw = s
			}
			return []any{contract, date, sym,
				marketdata.OptionalText(pick(row, "buyer_broker")),
				marketdata.OptionalText(pick(row, "seller_broker")),
				marketdata.OptionalText(pick(row, "buyer_member", "buyerMemberId")),
				marketdata.OptionalText(pick(row, "seller_member", "sellerMemberId")),
				qty, rate, *amount, executedAt, raw}, nil
		},
		prep: func(ctx context.Context, tx pgx.Tx, rows [][]any) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO trading_sessions (trading_date)
				SELECT DISTINCT s.trading_date FROM st_floorsheet s
				ON CONFLICT DO NOTHING`)
			return err
		},
		usesSource: true,
		// Member identity is never erased by a thinner capture. A tape taken while
		// the session is open publishes null buyer/seller ids and names for most
		// contracts; the same contracts carry both once the exchange attaches them.
		// Overwriting with null would delete a verified attribution and there is no
		// way to recover it from the thinner payload, so a null leaves the stored
		// value alone while a real value always wins.
		merge: `
			INSERT INTO floorsheet_trades (contract_id, trading_date, symbol, security_id,
				buyer_broker, seller_broker, buyer_broker_id, seller_broker_id,
				quantity, rate, amount, executed_at, data_source_id, raw)
			SELECT s.contract_id, s.trading_date, s.symbol, sec.id,
			       s.buyer, s.seller, s.buyer_id, s.seller_id,
			       s.quantity, s.rate, s.amount, s.executed_at, $1::uuid, s.raw
			FROM st_floorsheet s LEFT JOIN securities sec ON sec.symbol = s.symbol
			ON CONFLICT (contract_id) DO UPDATE SET
				trading_date = EXCLUDED.trading_date,
				symbol = EXCLUDED.symbol,
				security_id = COALESCE(EXCLUDED.security_id, floorsheet_trades.security_id),
				buyer_broker = COALESCE(EXCLUDED.buyer_broker, floorsheet_trades.buyer_broker),
				seller_broker = COALESCE(EXCLUDED.seller_broker, floorsheet_trades.seller_broker),
				buyer_broker_id = COALESCE(EXCLUDED.buyer_broker_id, floorsheet_trades.buyer_broker_id),
				seller_broker_id = COALESCE(EXCLUDED.seller_broker_id, floorsheet_trades.seller_broker_id),
				quantity = EXCLUDED.quantity,
				rate = EXCLUDED.rate,
				amount = EXCLUDED.amount,
				executed_at = COALESCE(EXCLUDED.executed_at, floorsheet_trades.executed_at),
				data_source_id = EXCLUDED.data_source_id
			-- The left side is what the row would become, the right side is what it is
			-- now. Comparing the coalesced value with itself would be a tautology and
			-- would silently treat a newly arriving member id as "unchanged".
			WHERE (EXCLUDED.trading_date, EXCLUDED.symbol,
			       COALESCE(EXCLUDED.security_id, floorsheet_trades.security_id),
			       COALESCE(EXCLUDED.buyer_broker, floorsheet_trades.buyer_broker),
			       COALESCE(EXCLUDED.seller_broker, floorsheet_trades.seller_broker),
			       COALESCE(EXCLUDED.buyer_broker_id, floorsheet_trades.buyer_broker_id),
			       COALESCE(EXCLUDED.seller_broker_id, floorsheet_trades.seller_broker_id),
			       EXCLUDED.quantity, EXCLUDED.rate, EXCLUDED.amount,
			       COALESCE(EXCLUDED.executed_at, floorsheet_trades.executed_at))
			      IS DISTINCT FROM
			      (floorsheet_trades.trading_date, floorsheet_trades.symbol,
			       floorsheet_trades.security_id, floorsheet_trades.buyer_broker,
			       floorsheet_trades.seller_broker, floorsheet_trades.buyer_broker_id,
			       floorsheet_trades.seller_broker_id, floorsheet_trades.quantity,
			       floorsheet_trades.rate, floorsheet_trades.amount,
			       floorsheet_trades.executed_at)`,
		// Volume and turnover are aggregated from the contracts themselves. That is
		// a measurement, not an estimate: multiplying volume by close produces a
		// number that looks like turnover but disagrees with the exchange's total.
		//
		// This runs once for the whole stream rather than per chunk, because a
		// per-chunk aggregate would be recomputed against a partial contract set
		// and rewrite the same bars repeatedly with intermediate totals.
		finalize: func(ctx context.Context, tx pgx.Tx) (int, error) {
			// Only turnover is taken from the contracts. Volume is left to the quote
			// snapshot's session total: the collector polls once per second and its
			// contract list is provably incomplete (sum of contracts is roughly 69% of
			// the exchange's reported turnover), so summing contracts for volume would
			// understate a figure the exchange actually reported. Turnover per security
			// is not reported anywhere else, so the contract sum is the only measurement
			// available — and it is recorded as such, not presented as complete.
			tag, err := tx.Exec(ctx, `
				WITH agg AS (
					SELECT t.security_id, t.trading_date, sum(t.amount) AS amt
					FROM floorsheet_trades t
					WHERE t.security_id IS NOT NULL
					GROUP BY 1, 2
				)
				UPDATE daily_ohlcv o SET turnover = agg.amt
				FROM agg
				WHERE o.security_id = agg.security_id AND o.trading_date = agg.trading_date
				  AND o.turnover IS DISTINCT FROM agg.amt`)
			if err != nil {
				return 0, err
			}
			// Bars for securities with no captured contracts must not keep a turnover
			// value derived earlier from volume x close: that number was an estimate
			// and it disagrees with the contracts that do exist.
			if _, err := tx.Exec(ctx, `
				UPDATE daily_ohlcv o SET turnover = NULL
				WHERE o.turnover IS NOT NULL
				  AND o.trading_date IN (SELECT DISTINCT trading_date FROM floorsheet_trades)
				  AND NOT EXISTS (
					SELECT 1 FROM floorsheet_trades t
					WHERE t.security_id = o.security_id AND t.trading_date = o.trading_date)`); err != nil {
				return 0, err
			}
			return int(tag.RowsAffected()), nil
		},
	}
}

// ---------------------------------------------------------------- market status

func marketStatusSpec() *streamSpec {
	return &streamSpec{
		stream:    marketdata.StreamMarketStatus,
		stageName: "st_market_status",
		stageDDL: `CREATE TEMP TABLE st_market_status (
			status text, reported_status text, as_of timestamptz, observed_at timestamptz
		) ON COMMIT DROP`,
		truncate: "TRUNCATE st_market_status",
		cols:     []string{"status", "reported_status", "as_of", "observed_at"},
		keyIdx:   []int{0, 2},
		normalize: func(row map[string]any) ([]any, error) {
			reported := str(row, "isOpen", "status", "marketStatus")
			if reported == "" {
				return nil, ferr("isOpen", "market state is missing")
			}
			// The feed's vocabulary is its own ("CLOSE", "OPEN", …). It is kept
			// verbatim beside the normalised value so a UI can show what was
			// actually said, and an unknown value is recorded as unknown rather
			// than assumed closed.
			status := "unknown"
			switch strings.ToLower(reported) {
			case "open":
				status = "open"
			case "preopen", "pre-open", "pre_open":
				status = "preopen"
			case "close", "closed":
				status = "closed"
			}

			// asOf is the exchange's wall clock in Nepal time; it has no zone
			// marker. Reading it as UTC would shift the status by 5h45m.
			asOf, err := marketdata.Timestamp(pick(row, "asOf", "as_of"), marketdata.BasisNPT)
			if err != nil || asOf == nil {
				return nil, ferr("asOf", "no status timestamp on the row")
			}
			observed, _ := marketdata.Timestamp(pick(row, "captured_at"), marketdata.BasisUTC)
			if observed == nil {
				now := time.Now().UTC()
				observed = &now
			}
			return []any{status, reported, asOf.UTC(), observed.UTC()}, nil
		},
		usesSource: true,
		// The same state re-read at the same exchange timestamp is the same
		// observation; only a new asOf creates a new row.
		merge: `
			INSERT INTO market_status_snapshots (status, reported_status, as_of, observed_at, data_source_id)
			SELECT s.status, s.reported_status, s.as_of, s.observed_at, $1::uuid
			FROM st_market_status s
			ON CONFLICT (status, as_of) DO NOTHING`,
	}
}

// ---------------------------------------------------------------- helpers

// zeroDecimal reports whether a normalised decimal is numerically zero. It is
// used as a predicate only: the value itself is never converted to a float for
// storage.
func zeroDecimal(s string) bool {
	f, err := strconv.ParseFloat(s, 64)
	return err == nil && f == 0
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullBool(isin string, ok bool) any {
	if isin == "" {
		return nil
	}
	return ok
}
