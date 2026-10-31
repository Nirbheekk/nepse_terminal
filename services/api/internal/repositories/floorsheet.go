package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// TradeFilter narrows the floorsheet. Zero values mean "no constraint", so an
// empty filter returns the newest contracts of the archive.
//
// Money bounds are strings because they are compared as NUMERIC: parsing them to
// float64 to build a query would reintroduce the rounding the column avoids.
type TradeFilter struct {
	Date      string
	Symbol    string
	Broker    string // matches either side
	Side      string // "" | "buy" | "sell"; narrows Broker to one side
	MinAmount string
	MaxAmount string
	Sort      string // contract_desc (default) | amount_desc | amount_asc | time_desc | time_asc
	Cursor    string // contract id, exclusive, for contract_desc
	Limit     int
	Offset    int
}

// orderBy maps a sort key to SQL. Every ordering is total because contract_id is
// appended as a tiebreaker: without it, rows with equal amounts could repeat or
// vanish across pages.
//
// Every column is table-qualified on purpose. The projection casts amount to
// text for JSON, which also names the output column "amount"; an unqualified
// ORDER BY amount would therefore resolve to that text column and sort
// "8340" above "207500" — a silently wrong order, not an error.
func (f TradeFilter) orderBy() string {
	switch f.Sort {
	case "amount_desc":
		return "t.amount DESC, t.contract_id DESC"
	case "amount_asc":
		return "t.amount ASC, t.contract_id DESC"
	case "time_desc":
		return "t.executed_at DESC NULLS LAST, t.contract_id DESC"
	case "time_asc":
		return "t.executed_at ASC NULLS LAST, t.contract_id DESC"
	default:
		return "t.contract_id DESC"
	}
}

// where builds the predicate list and its arguments. It is shared by the page,
// count and export paths so a filter cannot mean one thing in a table and
// another in a CSV.
func (f TradeFilter) where() (string, []any) {
	var (
		clauses []string
		args    []any
	)
	add := func(clause string, v any) {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	if f.Date != "" {
		add("t.trading_date = $%d::date", f.Date)
	}
	if f.Symbol != "" {
		add("t.symbol = $%d", f.Symbol)
	}
	if f.Broker != "" {
		// A broker is identified by its member id (what the broker summary lists
		// and what the exchange publishes per contract). Matching only the name
		// column made `broker=67` silently return nothing, so both the id and the
		// name are accepted and one argument fills both placeholders.
		switch f.Side {
		case "buy", "sell":
			args = append(args, f.Broker)
			n := len(args)
			col, nameCol := "buyer_broker_id", "buyer_broker"
			if f.Side == "sell" {
				col, nameCol = "seller_broker_id", "seller_broker"
			}
			clauses = append(clauses, fmt.Sprintf("(t.%s = $%d OR t.%s = $%d)", col, n, nameCol, n))
		default:
			// One argument, four placeholders: the same broker on either side.
			args = append(args, f.Broker)
			n := len(args)
			clauses = append(clauses, fmt.Sprintf(
				"(t.buyer_broker_id = $%d OR t.seller_broker_id = $%d"+
					" OR t.buyer_broker = $%d OR t.seller_broker = $%d)", n, n, n, n))
		}
	}
	if f.MinAmount != "" {
		add("t.amount >= $%d::numeric", f.MinAmount)
	}
	if f.MaxAmount != "" {
		add("t.amount <= $%d::numeric", f.MaxAmount)
	}
	if f.Cursor != "" {
		add("t.contract_id < $%d::numeric", f.Cursor)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

const tradeColumns = `t.contract_id::text, t.trading_date::text, t.symbol,
	t.buyer_broker, t.seller_broker, t.quantity::text, t.rate::text,
	t.amount::text, t.executed_at`

// ListTradesPage returns one page of filtered trades plus the unpaged total, so
// a table can show "1–50 of 69,619" without a second round trip guessing.
func (s *Store) ListTradesPage(ctx context.Context, f TradeFilter) ([]Trade, int, error) {
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 50
	}
	where, args := f.where()

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM floorsheet_trades t`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + tradeColumns + ` FROM floorsheet_trades t` + where +
		` ORDER BY ` + f.orderBy() +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	rows, err := s.pool.Query(ctx, q, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]Trade, 0, f.Limit)
	for rows.Next() {
		var t Trade
		if err := rows.Scan(&t.ContractID, &t.TradingDate, &t.Symbol, &t.BuyerBroker,
			&t.SellerBroker, &t.Quantity, &t.Rate, &t.Amount, &t.ExecutedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// EachTrade streams every matching contract without materialising it, so an
// export of a whole session does not have to fit in memory.
func (s *Store) EachTrade(ctx context.Context, f TradeFilter, fn func(Trade) error) (int, error) {
	where, args := f.where()
	q := `SELECT ` + tradeColumns + ` FROM floorsheet_trades t` + where + ` ORDER BY ` + f.orderBy()

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var t Trade
		if err := rows.Scan(&t.ContractID, &t.TradingDate, &t.Symbol, &t.BuyerBroker,
			&t.SellerBroker, &t.Quantity, &t.Rate, &t.Amount, &t.ExecutedAt); err != nil {
			return n, err
		}
		if err := fn(t); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

// ---------------------------------------------------------------- brokers

// BrokerSummary is one member's activity for a session, summed from contracts.
type BrokerSummary struct {
	BrokerCode    string  `json:"broker_code"`
	BrokerName    *string `json:"broker_name"`
	BuyContracts  int     `json:"buy_contracts"`
	SellContracts int     `json:"sell_contracts"`
	BuyQuantity   string  `json:"buy_quantity"`
	SellQuantity  string  `json:"sell_quantity"`
	BuyTurnover   string  `json:"buy_turnover"`
	SellTurnover  string  `json:"sell_turnover"`
	NetTurnover   string  `json:"net_turnover"`
}

// BrokerCoverage states how much of the session the summaries describe.
//
// The exchange publishes member ids on only a share of contracts and no names at
// all, so a broker table without this context would look like a complete
// picture of the market's participants.
type BrokerCoverage struct {
	TradingDate string `json:"trading_date"`
	Contracts   int    `json:"contracts"`
	WithBuyer   int    `json:"with_buyer"`
	WithSeller  int    `json:"with_seller"`
	// WithBuyerID/WithSellerID count contracts carrying the exchange's member
	// id, as opposed to only a broker name. The id is the stable key.
	WithBuyerID   int    `json:"with_buyer_id"`
	WithSellerID  int    `json:"with_seller_id"`
	Attributed    int    `json:"attributed"`
	Unattributed  int    `json:"unattributed"`
	Turnover      string `json:"turnover"`
	Brokers       int    `json:"brokers"`
	CoverageRatio string `json:"coverage_ratio"`
}

// RebuildBrokerSummaries recomputes one session's broker table from contracts.
//
// It replaces the session's rows rather than upserting them, so a contract that
// was re-ingested with a corrected member id cannot leave a stale total behind.
func (s *Store) RebuildBrokerSummaries(ctx context.Context, date string) (*BrokerCoverage, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`DELETE FROM floorsheet_broker_summaries WHERE trading_date = $1::date`, date); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO floorsheet_broker_summaries (
			trading_date, broker_code, broker_name, buy_contracts, sell_contracts,
			buy_quantity, sell_quantity, buy_turnover, sell_turnover, net_turnover)
		SELECT $1::date, COALESCE(b.code, s.code), COALESCE(b.name, s.name),
		       COALESCE(b.contracts, 0), COALESCE(s.contracts, 0),
		       COALESCE(b.quantity, 0), COALESCE(s.quantity, 0),
		       COALESCE(b.turnover, 0), COALESCE(s.turnover, 0),
		       COALESCE(s.turnover, 0) - COALESCE(b.turnover, 0)
		FROM (
			SELECT COALESCE(buyer_broker_id, buyer_broker) AS code,
			       min(buyer_broker) AS name, count(*) AS contracts,
			       sum(quantity) AS quantity, sum(amount) AS turnover
			FROM floorsheet_trades
			WHERE trading_date = $1::date
			  AND COALESCE(buyer_broker_id, buyer_broker) IS NOT NULL
			GROUP BY 1
		) b
		FULL OUTER JOIN (
			SELECT COALESCE(seller_broker_id, seller_broker) AS code,
			       min(seller_broker) AS name, count(*) AS contracts,
			       sum(quantity) AS quantity, sum(amount) AS turnover
			FROM floorsheet_trades
			WHERE trading_date = $1::date
			  AND COALESCE(seller_broker_id, seller_broker) IS NOT NULL
			GROUP BY 1
		) s ON s.code = b.code
		WHERE COALESCE(b.code, s.code) IS NOT NULL`, date); err != nil {
		return nil, err
	}

	cov, err := scanBrokerCoverage(ctx, tx, date)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return cov, nil
}

// rowQuerier is satisfied by both a pool and a transaction, so the coverage
// query has exactly one definition.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const brokerCoverageSQL = `
	SELECT count(*),
	       count(*) FILTER (WHERE COALESCE(buyer_broker_id, buyer_broker) IS NOT NULL),
	       count(*) FILTER (WHERE COALESCE(seller_broker_id, seller_broker) IS NOT NULL),
	       count(*) FILTER (WHERE buyer_broker_id IS NOT NULL),
	       count(*) FILTER (WHERE seller_broker_id IS NOT NULL),
	       count(*) FILTER (WHERE COALESCE(buyer_broker_id, buyer_broker) IS NOT NULL
	                          OR COALESCE(seller_broker_id, seller_broker) IS NOT NULL),
	       sum(amount)::text,
	       (SELECT count(*) FROM floorsheet_broker_summaries WHERE trading_date = $1::date)
	FROM floorsheet_trades WHERE trading_date = $1::date`

func scanBrokerCoverage(ctx context.Context, q rowQuerier, date string) (*BrokerCoverage, error) {
	cov := &BrokerCoverage{TradingDate: date}
	var turnover *string
	if err := q.QueryRow(ctx, brokerCoverageSQL, date).Scan(
		&cov.Contracts, &cov.WithBuyer, &cov.WithSeller, &cov.WithBuyerID,
		&cov.WithSellerID, &cov.Attributed, &turnover, &cov.Brokers); err != nil {
		return nil, err
	}
	cov.Unattributed = cov.Contracts - cov.Attributed
	if turnover != nil {
		cov.Turnover = *turnover
	}
	if cov.Contracts > 0 {
		cov.CoverageRatio = fmt.Sprintf("%.4f", float64(cov.Attributed)/float64(cov.Contracts))
	} else {
		cov.CoverageRatio = "0"
	}
	return cov, nil
}

// BrokerSummaries reads a session's broker table, highest buy turnover first.
func (s *Store) BrokerSummaries(ctx context.Context, date string, limit int) ([]BrokerSummary, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT broker_code, broker_name, buy_contracts, sell_contracts,
		       buy_quantity::text, sell_quantity::text,
		       buy_turnover::text, sell_turnover::text, net_turnover::text
		FROM floorsheet_broker_summaries
		WHERE trading_date = $1::date
		ORDER BY buy_turnover DESC, broker_code
		LIMIT $2`, date, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]BrokerSummary, 0, limit)
	for rows.Next() {
		var b BrokerSummary
		if err := rows.Scan(&b.BrokerCode, &b.BrokerName, &b.BuyContracts, &b.SellContracts,
			&b.BuyQuantity, &b.SellQuantity, &b.BuyTurnover, &b.SellTurnover,
			&b.NetTurnover); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BrokerCoverageFor reports how much of a session carries member identity.
func (s *Store) BrokerCoverageFor(ctx context.Context, date string) (*BrokerCoverage, error) {
	return scanBrokerCoverage(ctx, s.pool, date)
}

// ---------------------------------------------------------------- coverage

// DateCoverage is the evidence for one trading session: how many contracts the
// archive holds, whether a run proved it complete, and what the capture covered.
type DateCoverage struct {
	TradingDate        string `json:"trading_date"`
	Contracts          int    `json:"contracts"`
	ContractsReported  *int   `json:"contracts_reported,omitempty"`
	MissingContracts   *int   `json:"missing_contracts,omitempty"`
	Turnover           string `json:"turnover"`
	Symbols            int    `json:"symbols"`
	PagesPresent       int    `json:"pages_present"`
	PagesExpected      int    `json:"pages_expected"`
	Converged          *bool  `json:"converged,omitempty"`
	RunStatus          string `json:"run_status,omitempty"`
	Verified           bool   `json:"verified"`
	Status             string `json:"status"` // complete | incomplete | missing
	TimestampPrecision string `json:"timestamp_precision,omitempty"`
	TimezoneBasis      string `json:"timezone_basis,omitempty"`
	Unattributed       int    `json:"unattributed_contracts"`
}

// FloorsheetCoverage answers "which sessions does the trade archive actually
// cover, and how completely?".
//
// Status is decided from evidence, in this order: a session with no contracts is
// missing; a session whose latest run did not converge, or whose contract count
// falls short of what the exchange reported, is incomplete; anything else is
// complete. A date is never called complete because rows exist.
type FloorsheetCoverage struct {
	EarliestVerified string         `json:"earliest_verified"`
	LatestVerified   string         `json:"latest_verified"`
	EarliestPresent  string         `json:"earliest_present"`
	LatestPresent    string         `json:"latest_present"`
	Sessions         int            `json:"sessions_tracked"`
	MissingDates     []string       `json:"missing_dates"`
	Dates            []DateCoverage `json:"dates"`
}

// FloorsheetCoverageReport computes coverage across every trading session the
// platform knows about, newest first.
func (s *Store) FloorsheetCoverageReport(ctx context.Context, limit int) (*FloorsheetCoverage, error) {
	if limit <= 0 || limit > 500 {
		limit = 400
	}
	rows, err := s.pool.Query(ctx, `
		WITH t AS (
			SELECT trading_date, count(*) AS contracts, sum(amount) AS turnover,
			       count(DISTINCT symbol) AS symbols,
			       count(*) FILTER (WHERE (buyer_broker IS NULL OR buyer_broker = '')
			                          AND (seller_broker IS NULL OR seller_broker = '')) AS unattributed
			FROM floorsheet_trades GROUP BY 1
		), r AS (
			SELECT DISTINCT ON (trading_date) trading_date, status, converged,
			       missing_contracts, total_elements, timestamp_precision, timezone_basis,
			       pages_present, pages_expected
			FROM floorsheet_runs ORDER BY trading_date, started_at DESC
		)
		SELECT s.trading_date::text,
		       COALESCE(t.contracts, 0),
		       r.total_elements,
		       r.missing_contracts,
		       COALESCE(t.turnover, 0)::text,
		       COALESCE(t.symbols, 0),
		       COALESCE(r.pages_present, 0),
		       COALESCE(r.pages_expected, 0),
		       r.converged,
		       r.status,
		       r.timestamp_precision,
		       r.timezone_basis,
		       COALESCE(t.unattributed, 0)
		FROM trading_sessions s
		LEFT JOIN t ON t.trading_date = s.trading_date
		LEFT JOIN r ON r.trading_date = s.trading_date
		ORDER BY s.trading_date DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	report := &FloorsheetCoverage{}
	for rows.Next() {
		var d DateCoverage
		// Every run-derived column is nullable, because a session can trade
		// without any ingestion receipt (the LEFT JOIN yields NULL). Scanning
		// them into plain strings fails the whole report on the first such
		// session, so each is read through a pointer and normalised below.
		var runStatus, precision, basis *string
		if err := rows.Scan(&d.TradingDate, &d.Contracts, &d.ContractsReported,
			&d.MissingContracts, &d.Turnover, &d.Symbols, &d.PagesPresent,
			&d.PagesExpected, &d.Converged, &runStatus, &precision,
			&basis, &d.Unattributed); err != nil {
			return nil, err
		}
		d.RunStatus = derefString(runStatus)
		d.TimestampPrecision = derefString(precision)
		d.TimezoneBasis = derefString(basis)
		switch {
		case d.Contracts == 0:
			d.Status = "missing"
			report.MissingDates = append(report.MissingDates, d.TradingDate)
		case d.Converged != nil && !*d.Converged:
			d.Status = "incomplete"
		case d.MissingContracts != nil && *d.MissingContracts > 0:
			d.Status = "incomplete"
		default:
			d.Status = "complete"
		}
		if d.Status == "complete" {
			d.Verified = true
			if report.LatestVerified == "" {
				report.LatestVerified = d.TradingDate
			}
			report.EarliestVerified = d.TradingDate
		}
		if d.Contracts > 0 {
			if report.LatestPresent == "" {
				report.LatestPresent = d.TradingDate
			}
			report.EarliestPresent = d.TradingDate
		}
		report.Dates = append(report.Dates, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	report.Sessions = len(report.Dates)
	return report, nil
}

// derefString flattens a nullable text column: absence becomes the empty
// string, which the JSON omits, rather than the four characters "<nil>".
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ---------------------------------------------------------------- receipts

// PageRecord is one archived page as it is stored for audit.
type PageRecord struct {
	Sweep         int
	Page          int
	Rows          int
	FirstContract *int64
	LastContract  *int64
	TotalPages    int
	TotalElements int
	SHA256        string
	RelPath       string
	Bytes         int64
	FetchedAt     *time.Time
}

// RecordFloorsheetPages stores the pages a run read, replacing an earlier
// recording of the same page so the table always describes the bytes on disk.
func (s *Store) RecordFloorsheetPages(ctx context.Context, date, sessionID string, pages []PageRecord) (int, error) {
	if len(pages) == 0 {
		return 0, nil
	}
	batch := &pgx.Batch{}
	for _, p := range pages {
		batch.Queue(`
			INSERT INTO floorsheet_pages (trading_date, session_id, sweep, page_no, page_rows,
				first_contract, last_contract, total_pages, total_elements, sha256, rel_path,
				bytes, fetched_at)
			VALUES ($1::date, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (trading_date, session_id, sweep, page_no) DO UPDATE SET
				page_rows = EXCLUDED.page_rows,
				first_contract = EXCLUDED.first_contract,
				last_contract = EXCLUDED.last_contract,
				total_pages = EXCLUDED.total_pages,
				total_elements = EXCLUDED.total_elements,
				sha256 = EXCLUDED.sha256,
				rel_path = EXCLUDED.rel_path,
				bytes = EXCLUDED.bytes,
				fetched_at = EXCLUDED.fetched_at,
				ingested_at = now()`,
			date, sessionID, p.Sweep, p.Page, p.Rows, p.FirstContract, p.LastContract,
			p.TotalPages, p.TotalElements, p.SHA256, p.RelPath, p.Bytes, p.FetchedAt)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	n := 0
	for range pages {
		if _, err := br.Exec(); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// FloorsheetRun is one ingestion of one date, with the verdict that run reached.
type FloorsheetRun struct {
	ID                 string     `json:"id"`
	TradingDate        string     `json:"trading_date"`
	Source             string     `json:"source"`
	Status             string     `json:"status"`
	PagesExpected      int        `json:"pages_expected"`
	PagesPresent       int        `json:"pages_present"`
	PagesMissing       int        `json:"pages_missing"`
	RowsInPages        int        `json:"rows_in_pages"`
	UniqueContracts    int        `json:"unique_contracts"`
	DuplicateContracts int        `json:"duplicate_contracts"`
	TotalElements      int64      `json:"total_elements"`
	MissingContracts   int        `json:"missing_contracts"`
	Converged          bool       `json:"converged"`
	DateMismatches     int        `json:"date_mismatches"`
	DigestFailures     int        `json:"digest_failures"`
	RowsWritten        int        `json:"rows_written"`
	RowsUnchanged      int        `json:"rows_unchanged"`
	TimestampPrecision string     `json:"timestamp_precision"`
	TimezoneBasis      string     `json:"timezone_basis"`
	StartedAt          time.Time  `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
}

// SaveFloorsheetRun inserts a completed run receipt and returns its id.
func (s *Store) SaveFloorsheetRun(ctx context.Context, r *FloorsheetRun) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO floorsheet_runs (
			trading_date, source, status, pages_expected, pages_present, pages_missing,
			rows_in_pages, unique_contracts, duplicate_contracts, total_elements,
			missing_contracts, converged, date_mismatches, digest_failures,
			rows_written, rows_unchanged, timestamp_precision, timezone_basis,
			started_at, finished_at)
		VALUES ($1::date, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
		        $15, $16, $17, $18, $19, $20)
		RETURNING id::text`,
		r.TradingDate, r.Source, r.Status, r.PagesExpected, r.PagesPresent, r.PagesMissing,
		r.RowsInPages, r.UniqueContracts, r.DuplicateContracts, r.TotalElements,
		r.MissingContracts, r.Converged, r.DateMismatches, r.DigestFailures,
		r.RowsWritten, r.RowsUnchanged, r.TimestampPrecision, r.TimezoneBasis,
		r.StartedAt, r.FinishedAt).Scan(&id)
	if err != nil {
		return "", err
	}
	r.ID = id
	return id, nil
}

// LatestFloorsheetRuns returns the most recent run per date, newest date first.
func (s *Store) LatestFloorsheetRuns(ctx context.Context, limit int) ([]FloorsheetRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (trading_date)
		       id::text, trading_date::text, source, status, pages_expected, pages_present,
		       pages_missing, rows_in_pages, unique_contracts, duplicate_contracts,
		       total_elements, missing_contracts, converged, date_mismatches,
		       digest_failures, rows_written, rows_unchanged, timestamp_precision,
		       timezone_basis, started_at, finished_at
		FROM floorsheet_runs
		ORDER BY trading_date DESC, started_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FloorsheetRun
	for rows.Next() {
		var r FloorsheetRun
		if err := rows.Scan(&r.ID, &r.TradingDate, &r.Source, &r.Status, &r.PagesExpected,
			&r.PagesPresent, &r.PagesMissing, &r.RowsInPages, &r.UniqueContracts,
			&r.DuplicateContracts, &r.TotalElements, &r.MissingContracts, &r.Converged,
			&r.DateMismatches, &r.DigestFailures, &r.RowsWritten, &r.RowsUnchanged,
			&r.TimestampPrecision, &r.TimezoneBasis, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FloorsheetSessionDates lists dates that already hold contracts, so a backfill
// can skip work that is already done.
func (s *Store) FloorsheetSessionDates(ctx context.Context) (map[string]int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT trading_date::text, count(*) FROM floorsheet_trades GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		out[d] = n
	}
	return out, rows.Err()
}
