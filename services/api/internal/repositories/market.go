package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned when a lookup has no row. Handlers map it to 404.
var ErrNotFound = errors.New("not found")

// Summary is one trading session's headline numbers. Money and index values are
// carried as strings because the database stores NUMERIC and JSON has no exact
// decimal type: converting through float64 would lose precision.
type Summary struct {
	TradingDate     string    `json:"trading_date"`
	NepseIndex      string    `json:"nepse_index"`
	PointChange     string    `json:"point_change"`
	PercentChange   string    `json:"percent_change"`
	Turnover        string    `json:"turnover"`
	Transactions    int64     `json:"transactions"`
	TradedShares    string    `json:"traded_shares"`
	TradedSecurites int       `json:"traded_securities"`
	Advanced        int       `json:"advanced"`
	Declined        int       `json:"declined"`
	Unchanged       int       `json:"unchanged"`
	MarketCap       string    `json:"market_cap"`
	FloatMarketCap  string    `json:"float_market_cap"`
	CapCovered      int       `json:"cap_covered"`
	CapEligible     int       `json:"cap_eligible"`
	CapMethod       string    `json:"cap_method"`
	IngestedAt      time.Time `json:"ingested_at"`
}

// LatestSummary returns the most recent session summary.
func (s *Store) LatestSummary(ctx context.Context) (*Summary, error) {
	const q = `
		SELECT trading_date::text,
		       COALESCE(nepse_index, 0)::text,
		       COALESCE(point_change, 0)::text,
		       COALESCE(percent_change, 0)::text,
		       COALESCE(turnover, 0)::text,
		       COALESCE(transactions, 0),
		       COALESCE(traded_shares, 0)::text,
		       COALESCE(traded_securities, 0),
		       COALESCE(advanced, 0), COALESCE(declined, 0), COALESCE(unchanged, 0),
		       COALESCE(market_cap, 0)::text,
		       COALESCE(float_market_cap, 0)::text,
		       COALESCE(cap_covered, 0), COALESCE(cap_eligible, 0),
		       COALESCE(cap_method, ''),
		       ingested_at
		FROM market_summaries
		ORDER BY trading_date DESC
		LIMIT 1`

	var out Summary
	err := s.pool.QueryRow(ctx, q).Scan(
		&out.TradingDate, &out.NepseIndex, &out.PointChange, &out.PercentChange,
		&out.Turnover, &out.Transactions, &out.TradedShares, &out.TradedSecurites,
		&out.Advanced, &out.Declined, &out.Unchanged,
		&out.MarketCap, &out.FloatMarketCap, &out.CapCovered, &out.CapEligible,
		&out.CapMethod, &out.IngestedAt,
	)
	if err != nil {
		return nil, wrapNoRows(err)
	}
	return &out, nil
}

// Mover is one row of a movers/volume/turnover ranking.
type Mover struct {
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	Close         string `json:"close"`
	PreviousClose string `json:"previous_close"`
	Change        string `json:"change"`
	PercentChange string `json:"percent_change"`
	Volume        string `json:"volume"`
	Turnover      string `json:"turnover"`
}

var moverOrder = map[string]string{
	"gainers":  "((o.close - o.previous_close) / o.previous_close) DESC NULLS LAST",
	"losers":   "((o.close - o.previous_close) / o.previous_close) ASC NULLS LAST",
	"turnover": "o.turnover DESC NULLS LAST",
	"volume":   "o.volume DESC NULLS LAST",
}

// TopMovers returns the ranking for the latest stored session. The ORDER BY is
// selected from a fixed map, never interpolated from caller input.
func (s *Store) TopMovers(ctx context.Context, kind string, limit int) ([]Mover, error) {
	order, ok := moverOrder[kind]
	if !ok {
		return nil, errors.New("unknown mover kind")
	}

	q := `
		WITH latest AS (SELECT max(trading_date) AS dt FROM daily_ohlcv)
		SELECT c.symbol, c.name,
		       COALESCE(o.close, 0)::text,
		       COALESCE(o.previous_close, 0)::text,
		       COALESCE(o.close - o.previous_close, 0)::text,
		       COALESCE(round(((o.close - o.previous_close) / NULLIF(o.previous_close, 0)) * 100, 2), 0)::text,
		       COALESCE(o.volume, 0)::text,
		       COALESCE(o.turnover, 0)::text
		FROM daily_ohlcv o
		JOIN securities sec ON sec.id = o.security_id
		JOIN companies c ON c.id = sec.company_id
		JOIN latest l ON o.trading_date = l.dt
		WHERE o.previous_close IS NOT NULL AND o.previous_close > 0
		ORDER BY ` + order + `
		LIMIT $1`

	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Mover, 0, limit)
	for rows.Next() {
		var m Mover
		if err := rows.Scan(&m.Symbol, &m.Name, &m.Close, &m.PreviousClose,
			&m.Change, &m.PercentChange, &m.Volume, &m.Turnover); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Company is a list row or profile.
//
// Market capitalisation is carried with the method that produced it. A figure
// the exchange reported and a figure derived as close x listed shares are not
// interchangeable, and a client that cannot tell them apart will eventually
// present an estimate as an observation.
type Company struct {
	Symbol          string `json:"symbol"`
	Name            string `json:"name"`
	Sector          string `json:"sector"`
	SectorCode      string `json:"sector_code,omitempty"`
	Status          string `json:"status"`
	Instrument      string `json:"instrument"`
	Close           string `json:"close,omitempty"`
	PreviousClose   string `json:"previous_close,omitempty"`
	Change          string `json:"change,omitempty"`
	PercentChange   string `json:"percent_change,omitempty"`
	Volume          string `json:"volume,omitempty"`
	Turnover        string `json:"turnover,omitempty"`
	Transactions    string `json:"transactions,omitempty"`
	ListedShares    string `json:"listed_shares,omitempty"`
	PublicShares    string `json:"public_shares,omitempty"`
	MarketCap       string `json:"market_cap,omitempty"`
	MarketCapMethod string `json:"market_cap_method"`
	// MarketCapReported is the exchange's own field. It is computed at the
	// previous close, so it is carried beside the current figure rather than
	// presented as one.
	MarketCapReported string     `json:"market_cap_reported,omitempty"`
	MarketCapBasis    string     `json:"market_cap_reported_basis,omitempty"`
	FloatMarketCap    string     `json:"float_market_cap,omitempty"`
	FloatCapMethod    string     `json:"float_market_cap_method"`
	TradingDate       string     `json:"trading_date,omitempty"`
	IngestedAt        *time.Time `json:"ingested_at,omitempty"`
}

const companySelect = `
	SELECT c.symbol, c.name, COALESCE(s.name, 'Unclassified'), COALESCE(s.code, ''),
	       c.status, c.instrument,
	       COALESCE(o.close, 0)::text,
	       COALESCE(o.previous_close, 0)::text,
	       COALESCE(o.close - o.previous_close, 0)::text,
	       COALESCE(round(((o.close - o.previous_close) / NULLIF(o.previous_close, 0)) * 100, 2), 0)::text,
	       COALESCE(o.volume, 0)::text,
	       COALESCE(o.turnover, 0)::text,
	       COALESCE(o.transactions, 0)::text,
	       COALESCE(sec.listed_shares, 0)::text,
	       COALESCE(sec.public_shares, 0)::text,
	       COALESCE(COALESCE(NULLIF(o.close * sec.listed_shares, 0), NULLIF(o.market_cap, 0))::text, ''),
	       CASE WHEN NULLIF(o.close * sec.listed_shares, 0) IS NOT NULL THEN 'close_x_listed_shares'
	            WHEN NULLIF(o.market_cap, 0) IS NOT NULL THEN 'reported_at_previous_close'
	            ELSE 'unavailable' END,
	       COALESCE(NULLIF(o.market_cap, 0)::text, ''),
	       CASE WHEN NULLIF(o.market_cap, 0) IS NOT NULL THEN 'previous_close' ELSE '' END,
	       COALESCE(NULLIF(o.close * sec.public_shares, 0)::text, ''),
	       CASE WHEN NULLIF(o.close * sec.public_shares, 0) IS NOT NULL THEN 'close_x_public_shares'
	            ELSE 'unavailable' END,
	       COALESCE(o.trading_date::text, ''),
	       o.ingested_at
	FROM companies c
	LEFT JOIN sectors s ON s.id = c.sector_id
	-- An issuer can now own several securities: its ordinary line plus any
	-- promoter or preference class that trades under its own symbol. Joining
	-- them directly would return the issuer once per class, so the ordinary
	-- line (the security whose symbol is the issuer's own) is selected.
	LEFT JOIN LATERAL (
		SELECT x.id, x.listed_shares, x.public_shares
		FROM securities x
		WHERE x.company_id = c.id
		ORDER BY (x.symbol = c.symbol) DESC, x.symbol
		LIMIT 1
	) sec ON true
	LEFT JOIN LATERAL (
		SELECT d.trading_date, d.close, d.previous_close, d.volume, d.turnover,
		       d.transactions, d.market_cap, d.ingested_at
		FROM daily_ohlcv d
		WHERE d.security_id = sec.id
		ORDER BY d.trading_date DESC
		LIMIT 1
	) o ON true`

// CompanyFilter is the validated company-screen query. Sorting is chosen from a
// fixed map, never interpolated from caller input.
type CompanyFilter struct {
	Query      string
	Sector     string
	Instrument string
	Status     string
	Sort       string
	Descending bool
	Limit      int
	Offset     int
}

var companySorts = map[string]string{
	"symbol":       "c.symbol",
	"name":         "lower(c.name)",
	"sector":       "COALESCE(s.name, '')",
	"close":        "o.close",
	"change":       "((o.close - o.previous_close) / NULLIF(o.previous_close, 0))",
	"volume":       "o.volume",
	"turnover":     "o.turnover",
	"transactions": "o.transactions", "market_cap": "COALESCE(NULLIF(o.close * sec.listed_shares, 0), NULLIF(o.market_cap, 0))",
	"float_market_cap": "NULLIF(o.close * sec.public_shares, 0)",
	"trading_date":     "o.trading_date",
}

// CompanySorts lists the sortable columns so the API can reject an unknown one
// instead of silently ordering by something else.
func CompanySorts() []string {
	out := make([]string, 0, len(companySorts))
	for k := range companySorts {
		out = append(out, k)
	}
	return out
}

// ListCompanies returns a filtered page plus the total matching row count.
func (s *Store) ListCompanies(ctx context.Context, f CompanyFilter) ([]Company, int, error) {
	// Placeholder issuers exist only so a listed class security has a parent; they
	// are not listed companies and must not appear in an issuer screen.
	const where = ` WHERE c.origin = 'registry'` +
		` AND ($1 = '' OR c.symbol ILIKE '%' || $1 || '%' OR c.name ILIKE '%' || $1 || '%')` +
		` AND ($2 = '' OR s.code = $2)` +
		` AND ($3 = '' OR c.instrument = $3)` +
		` AND ($4 = '' OR c.status = $4)`

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM companies c LEFT JOIN sectors s ON s.id = c.sector_id`+where,
		f.Query, f.Sector, f.Instrument, f.Status).Scan(&total); err != nil {
		return nil, 0, err
	}

	order, ok := companySorts[f.Sort]
	if !ok {
		order = companySorts["symbol"]
	}
	dir := "ASC"
	if f.Descending {
		dir = "DESC"
	}

	rows, err := s.pool.Query(ctx,
		companySelect+where+` ORDER BY `+order+` `+dir+` NULLS LAST, c.symbol ASC LIMIT $5 OFFSET $6`,
		f.Query, f.Sector, f.Instrument, f.Status, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]Company, 0, f.Limit)
	for rows.Next() {
		var c Company
		if err := rows.Scan(&c.Symbol, &c.Name, &c.Sector, &c.SectorCode, &c.Status, &c.Instrument,
			&c.Close, &c.PreviousClose, &c.Change, &c.PercentChange,
			&c.Volume, &c.Turnover, &c.Transactions, &c.ListedShares, &c.PublicShares,
			&c.MarketCap, &c.MarketCapMethod, &c.MarketCapReported, &c.MarketCapBasis,
			&c.FloatMarketCap, &c.FloatCapMethod, &c.TradingDate, &c.IngestedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// GetCompany returns one company profile.
func (s *Store) GetCompany(ctx context.Context, symbol string) (*Company, error) {
	var c Company
	err := s.pool.QueryRow(ctx, companySelect+` WHERE c.symbol = $1`, symbol).Scan(
		&c.Symbol, &c.Name, &c.Sector, &c.SectorCode, &c.Status, &c.Instrument,
		&c.Close, &c.PreviousClose, &c.Change, &c.PercentChange,
		&c.Volume, &c.Turnover, &c.Transactions, &c.ListedShares, &c.PublicShares,
		&c.MarketCap, &c.MarketCapMethod, &c.MarketCapReported, &c.MarketCapBasis,
		&c.FloatMarketCap, &c.FloatCapMethod, &c.TradingDate, &c.IngestedAt)
	if err != nil {
		return nil, wrapNoRows(err)
	}
	return &c, nil
}

// CapitalisationCoverage reports how many registered issuers have a stored
// capitalisation. It is separate from the summary's coverage because the two
// describe different things: the summary's figure is the exchange's own total,
// while this counts the issuer rows the company screen can actually show.
func (s *Store) CapitalisationCoverage(ctx context.Context) (covered, eligible int, err error) {
	const q = `
		SELECT count(*) FILTER (WHERE d.market_cap IS NOT NULL), count(*)
		FROM companies c
		LEFT JOIN LATERAL (
			SELECT COALESCE(NULLIF(o.close * sec.listed_shares, 0), NULLIF(o.market_cap, 0)) AS market_cap
			FROM securities sec
			JOIN daily_ohlcv o ON o.security_id = sec.id
			WHERE sec.company_id = c.id
			ORDER BY (sec.symbol = c.symbol) DESC, o.trading_date DESC
			LIMIT 1
		) d ON true
		WHERE c.origin = 'registry'`
	err = s.pool.QueryRow(ctx, q).Scan(&covered, &eligible)
	return covered, eligible, err
}

// Sector is one filter entry for the company screen.
type Sector struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Companies int    `json:"companies"`
}

// ListSectors returns sectors that actually have registered issuers.
func (s *Store) ListSectors(ctx context.Context) ([]Sector, error) {
	const q = `
		SELECT s.code, s.name, count(c.id)
		FROM sectors s
		JOIN companies c ON c.sector_id = s.id AND c.origin = 'registry'
		GROUP BY s.code, s.name
		ORDER BY s.name`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Sector, 0, 13)
	for rows.Next() {
		var x Sector
		if err := rows.Scan(&x.Code, &x.Name, &x.Companies); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// MarketStatus is the exchange's own open/closed state at a point in time.
type MarketStatus struct {
	State            string    `json:"state"`
	ReportedStatus   string    `json:"reported_status"`
	AsOf             time.Time `json:"as_of"`
	ObservedAt       time.Time `json:"observed_at"`
	AgeSeconds       int64     `json:"age_seconds"`
	SessionDate      string    `json:"session_date,omitempty"`
	IsOpen           bool      `json:"is_open"`
	IsCurrentSession bool      `json:"is_current_session"`
}

// LatestMarketStatus returns the most recent state the collector observed.
//
// A missing row is not an error the caller must handle as failure: an empty
// database has no status, and the API then says "unknown" rather than guessing.
func (s *Store) LatestMarketStatus(ctx context.Context) (*MarketStatus, error) {
	const q = `
		SELECT status, reported_status, as_of, observed_at
		FROM market_status_snapshots
		ORDER BY as_of DESC, observed_at DESC
		LIMIT 1`
	var st MarketStatus
	if err := s.pool.QueryRow(ctx, q).Scan(&st.State, &st.ReportedStatus, &st.AsOf, &st.ObservedAt); err != nil {
		return nil, wrapNoRows(err)
	}
	st.AgeSeconds = int64(time.Since(st.ObservedAt).Seconds())

	// The session the status belongs to is the last date with a stored summary,
	// and is what a client compares against its own clock.
	_ = s.pool.QueryRow(ctx, `SELECT max(trading_date)::text FROM market_summaries`).Scan(&st.SessionDate)
	st.IsOpen = st.State == "open"
	// "Current" is judged in Nepal time: UTC would call the previous session
	// today's for the first 5h45m after midnight NPT.
	npt := time.FixedZone("NPT", 5*3600+45*60)
	st.IsCurrentSession = st.SessionDate != "" && st.SessionDate == time.Now().In(npt).Format("2006-01-02")
	return &st, nil
}

// Movers is the three rankings the overview renders.
type Movers struct {
	Gainers  []Mover `json:"gainers"`
	Losers   []Mover `json:"losers"`
	Turnover []Mover `json:"turnover"`
}

// Overview is the whole market front page in one consistent payload.
type Overview struct {
	Summary     *Summary      `json:"summary"`
	Status      *MarketStatus `json:"status"`
	Movers      Movers        `json:"movers"`
	Indices     []Index       `json:"indices"`
	GeneratedAt time.Time     `json:"generated_at"`
}

// MarketOverview composes the overview from the same stored session, so every
// figure on the page describes one moment rather than five different ones.
func (s *Store) MarketOverview(ctx context.Context) (*Overview, error) {
	sum, err := s.LatestSummary(ctx)
	if err != nil {
		return nil, err
	}

	out := &Overview{Summary: sum, GeneratedAt: time.Now().UTC()}
	if st, err := s.LatestMarketStatus(ctx); err == nil {
		out.Status = st
	} else if err != ErrNotFound {
		return nil, err
	}

	for kind, target := range map[string]*[]Mover{
		"gainers": &out.Movers.Gainers, "losers": &out.Movers.Losers, "turnover": &out.Movers.Turnover,
	} {
		rows, err := s.TopMovers(ctx, kind, 8)
		if err != nil {
			return nil, err
		}
		*target = rows
	}

	indices, err := s.ListIndices(ctx)
	if err != nil {
		return nil, err
	}
	out.Indices = indices
	return out, nil
}

// Bar is one daily OHLCV observation.
type Bar struct {
	TradingDate   string `json:"trading_date"`
	Open          string `json:"open"`
	High          string `json:"high"`
	Low           string `json:"low"`
	Close         string `json:"close"`
	PreviousClose string `json:"previous_close"`
	Volume        string `json:"volume"`
	Turnover      string `json:"turnover"`
	Transactions  string `json:"transactions"`
	MarketCap     string `json:"market_cap,omitempty"`
}

// PriceHistory returns the most recent daily bars for a symbol, oldest first.
func (s *Store) PriceHistory(ctx context.Context, symbol string, limit int) ([]Bar, error) {
	const q = `
		SELECT d.trading_date::text,
		       COALESCE(d.open, 0)::text, COALESCE(d.high, 0)::text,
		       COALESCE(d.low, 0)::text, COALESCE(d.close, 0)::text,
		       COALESCE(d.previous_close, 0)::text,
		       COALESCE(d.volume, 0)::text, COALESCE(d.turnover, 0)::text,
		       COALESCE(d.transactions, 0)::text,
		       COALESCE(d.market_cap::text, '')
		FROM daily_ohlcv d
		JOIN securities sec ON sec.id = d.security_id
		WHERE sec.symbol = $1
		ORDER BY d.trading_date DESC
		LIMIT $2`
	rows, err := s.pool.Query(ctx, q, symbol, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Bar
	for rows.Next() {
		var b Bar
		if err := rows.Scan(&b.TradingDate, &b.Open, &b.High, &b.Low, &b.Close,
			&b.PreviousClose, &b.Volume, &b.Turnover, &b.Transactions, &b.MarketCap); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse to chronological order for charting.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// Index is an index definition with its latest observation.
type Index struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Value         string `json:"value"`
	PointChange   string `json:"point_change"`
	PercentChange string `json:"percent_change"`
	TradingDate   string `json:"trading_date"`
}

// ListIndices returns every defined index with its latest daily value.
func (s *Store) ListIndices(ctx context.Context) ([]Index, error) {
	// A day can hold several observations of the same index (one per capture).
	// The list shows the most recent point, which is the observation time rather
	// than the trading date alone.
	const q = `
		SELECT d.code, d.name, d.kind,
		       COALESCE(obs.value, 0)::text,
		       COALESCE(obs.point_change, 0)::text,
		       COALESCE(obs.percent_change, 0)::text,
		       COALESCE(obs.trading_date::text, '')
		FROM index_definitions d
		LEFT JOIN LATERAL (
			SELECT o.value, o.point_change, o.percent_change, o.trading_date
			FROM index_observations o
			WHERE o.index_id = d.id AND o.granularity = 'daily'
			ORDER BY o.trading_date DESC, o.observed_at DESC
			LIMIT 1
		) obs ON true
		ORDER BY d.kind, d.code`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Index, 0, 16)
	for rows.Next() {
		var i Index
		if err := rows.Scan(&i.Code, &i.Name, &i.Kind, &i.Value,
			&i.PointChange, &i.PercentChange, &i.TradingDate); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Trade is one floorsheet row.
type Trade struct {
	ContractID   string     `json:"contract_id"`
	TradingDate  string     `json:"trading_date"`
	Symbol       string     `json:"symbol"`
	BuyerBroker  *string    `json:"buyer_broker"`
	SellerBroker *string    `json:"seller_broker"`
	Quantity     string     `json:"quantity"`
	Rate         string     `json:"rate"`
	Amount       string     `json:"amount"`
	ExecutedAt   *time.Time `json:"executed_at"`
}

// Health reports data-plane facts, distinct from process liveness.
type Health struct {
	Companies     int    `json:"companies"`
	Securities    int    `json:"securities"`
	Sessions      int    `json:"sessions"`
	Trades        int    `json:"trades"`
	Bars          int    `json:"bars"`
	LatestSession string `json:"latest_session"`
	LatestIngest  string `json:"latest_ingest"`
	SchemaVersion string `json:"schema_version"`
}

// DataHealth aggregates counts for /api/v1/health.
func (s *Store) DataHealth(ctx context.Context) (*Health, error) {
	const q = `
		SELECT
		  (SELECT count(*) FROM companies WHERE origin = 'registry'),
		  (SELECT count(*) FROM securities),
		  (SELECT count(*) FROM trading_sessions),
		  (SELECT count(*) FROM floorsheet_trades),
		  (SELECT count(*) FROM daily_ohlcv),
		  COALESCE((SELECT max(trading_date)::text FROM trading_sessions), ''),
		  COALESCE((SELECT max(ingested_at)::text FROM market_summaries), ''),
		  COALESCE((SELECT max(version) FROM schema_migrations), '')`
	var h Health
	if err := s.pool.QueryRow(ctx, q).Scan(&h.Companies, &h.Securities, &h.Sessions,
		&h.Trades, &h.Bars, &h.LatestSession, &h.LatestIngest, &h.SchemaVersion); err != nil {
		return nil, err
	}
	return &h, nil
}

func nullIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func wrapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
