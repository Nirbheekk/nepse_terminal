package handlers

import (
	"encoding/csv"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nepse/terminal/api/internal/repositories"
)

var (
	symbolRe   = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,19}$`)
	dateRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	sectorRe   = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
	contractRe = regexp.MustCompile(`^[0-9]{1,30}$`)
)

// marketMeta attaches provenance and an explicit freshness verdict to a market
// payload.
//
// "Stale" is not the same as "yesterday's close". Once the exchange is closed,
// the last session's figures are final and will not change again; calling that
// stale trains people to ignore the warning. Stale means a capture is older than
// the session it should describe.
func (s *Server) marketMeta(r *http.Request, ingestedAt time.Time, sessionDate string) Meta {
	m := metaFrom(r)
	m.Source = "postgres:market_data"
	m.LicenseClass = "public_unverified"
	m.Method = "derived@v1"
	m.SessionDate = sessionDate

	var status *repositories.MarketStatus
	if s.store != nil {
		if st, err := s.store.LatestMarketStatus(r.Context()); err == nil {
			status = st
		}
	}

	sessionAge := sessionAgeDays(sessionDate)

	// "As of" is when the exchange said it, not when this process wrote the row.
	// A re-ingest touches ingested_at and would otherwise make week-old data look
	// seconds old.
	observed := ingestedAt
	if status != nil && status.SessionDate == sessionDate && !status.ObservedAt.IsZero() {
		observed = status.ObservedAt
	}
	if !observed.IsZero() {
		m.AsOf = observed.UTC().Format(time.RFC3339)
		age := int64(time.Since(observed).Seconds())
		m.AgeSeconds = &age
	}

	state := "unknown"
	if status != nil {
		state = status.State
	}
	m.MarketState = state

	switch {
	case sessionDate != "" && sessionAge > 4:
		m.Freshness = "stale"
		m.FreshnessReason = "no session has been captured in " + strconv.Itoa(sessionAge) + " days"
	case state == "closed" || state == "preopen":
		m.Freshness = "final"
		if sessionDate != "" {
			m.FreshnessReason = "exchange " + state + "; session of " + sessionDate + " is final"
		} else {
			m.FreshnessReason = "exchange " + state + "; no session stored"
		}
	case observed.IsZero():
		m.Freshness = "stale"
		m.FreshnessReason = "no capture timestamp is stored for this payload"
	default:
		age := time.Since(observed)
		switch {
		case age < 2*time.Minute:
			m.Freshness = "live"
			m.FreshnessReason = "captured less than 2 minutes ago"
		case age < 15*time.Minute:
			m.Freshness = "delayed"
			m.FreshnessReason = "captured " + strconv.Itoa(int(age.Minutes())) + " minutes ago"
		default:
			m.Freshness = "stale"
			m.FreshnessReason = "no capture for " + strconv.Itoa(int(age.Hours())) + " hours"
		}
	}
	return m
}

// sessionAgeDays compares a trading date with today's date in Nepal. Using UTC
// would report the previous session as today's for the first 5h45m after
// midnight NPT.
func sessionAgeDays(sessionDate string) int {
	if sessionDate == "" {
		return 0
	}
	day, err := time.Parse("2006-01-02", sessionDate)
	if err != nil {
		return 0
	}
	npt := time.FixedZone("NPT", 5*3600+45*60)
	now := time.Now().In(npt)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, npt)
	days := int(today.Sub(day).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// ---------------------------------------------------------------- overview

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := s.store.MarketOverview(r.Context())
	if err != nil {
		if err == repositories.ErrNotFound {
			Fail(w, r, http.StatusNotFound, CodeNotFound, "no market summary stored yet")
			return
		}
		s.internal(w, r, "overview", err)
		return
	}
	sum := ov.Summary
	meta := s.marketMeta(r, sum.IngestedAt, sum.TradingDate)
	meta.Coverage = &Coverage{Covered: sum.CapCovered, Eligible: sum.CapEligible, Unit: "companies"}
	OK(w, r, ov, meta)
}

// marketStatusView adds the two questions a dashboard actually asks: is it open,
// and does this status describe the session I am showing?
type marketStatusView struct {
	State            string     `json:"state"`
	ReportedStatus   string     `json:"reported_status"`
	AsOf             *time.Time `json:"as_of"`
	ObservedAt       *time.Time `json:"observed_at"`
	AgeSeconds       *int64     `json:"age_seconds"`
	SessionDate      string     `json:"session_date"`
	IsOpen           bool       `json:"is_open"`
	IsCurrentSession bool       `json:"is_current_session"`
}

func (s *Server) handleMarketStatus(w http.ResponseWriter, r *http.Request) {
	meta := s.marketMeta(r, time.Time{}, "")
	view := marketStatusView{State: "unknown"}

	st, err := s.store.LatestMarketStatus(r.Context())
	switch {
	case err == repositories.ErrNotFound:
		// Not an error: a fresh database has no status yet, and inventing
		// "closed" would be a claim nobody made.
	case err != nil:
		s.internal(w, r, "market status", err)
		return
	default:
		view = marketStatusView{
			State:          st.State,
			ReportedStatus: st.ReportedStatus,
			AsOf:           &st.AsOf,
			ObservedAt:     &st.ObservedAt,
			AgeSeconds:     &st.AgeSeconds,
			SessionDate:    st.SessionDate,
			IsOpen:         st.State == "open",
		}
		view.IsCurrentSession = st.SessionDate != "" && sessionAgeDays(st.SessionDate) == 0
		meta.SessionDate = st.SessionDate
		meta.MarketState = st.State
		if st.State == "closed" || st.State == "preopen" {
			meta.Freshness = "final"
			meta.FreshnessReason = "exchange " + st.State + "; last status at " + st.AsOf.Format(time.RFC3339)
		} else {
			meta.Freshness = "live"
			meta.FreshnessReason = "exchange " + st.State + "; last status at " + st.AsOf.Format(time.RFC3339)
		}
		asOf := st.ObservedAt.Format(time.RFC3339)
		meta.AsOf = asOf
	}
	OK(w, r, view, meta)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	sum, err := s.store.LatestSummary(r.Context())
	if err != nil {
		if err == repositories.ErrNotFound {
			Fail(w, r, http.StatusNotFound, CodeNotFound, "no market summary stored yet")
			return
		}
		s.internal(w, r, "summary", err)
		return
	}
	meta := s.marketMeta(r, sum.IngestedAt, sum.TradingDate)
	meta.Coverage = &Coverage{Covered: sum.CapCovered, Eligible: sum.CapEligible, Unit: "companies"}
	OK(w, r, sum, meta)
}

func (s *Server) handleTopMovers(w http.ResponseWriter, r *http.Request) {
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	if kind == "" {
		kind = "gainers"
	}
	switch kind {
	case "gainers", "losers", "turnover", "volume":
	default:
		badRequest(w, r, "type must be one of gainers, losers, turnover, volume")
		return
	}
	limit, err := queryInt(r, "limit", 10, 1, 50)
	if err != nil {
		badRequest(w, r, err.Error())
		return
	}

	rows, err := s.store.TopMovers(r.Context(), kind, limit)
	if err != nil {
		s.internal(w, r, "top movers", err)
		return
	}
	meta := metaFrom(r)
	meta.Source = "postgres:daily_ohlcv"
	meta.LicenseClass = "public_unverified"
	if len(rows) > 0 {
		// The ranking is computed from one session; say which one.
		if sum, err := s.store.LatestSummary(r.Context()); err == nil {
			meta = s.marketMeta(r, sum.IngestedAt, sum.TradingDate)
		}
	}
	OK(w, r, rows, meta)
}

func (s *Server) handleIndices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListIndices(r.Context())
	if err != nil {
		s.internal(w, r, "indices", err)
		return
	}
	meta := metaFrom(r)
	meta.Source = "postgres:index_observations"
	meta.LicenseClass = "public_unverified"
	if sum, err := s.store.LatestSummary(r.Context()); err == nil {
		meta = s.marketMeta(r, sum.IngestedAt, sum.TradingDate)
	}
	OK(w, r, rows, meta)
}

func (s *Server) handleSectors(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListSectors(r.Context())
	if err != nil {
		s.internal(w, r, "sectors", err)
		return
	}
	meta := metaFrom(r)
	meta.Source = "postgres:companies"
	meta.LicenseClass = "public_unverified"
	OK(w, r, rows, meta)
}

// ---------------------------------------------------------------- companies

// parseCompanyFilter validates every filter and sort before SQL sees it. An
// unknown sort is refused rather than silently replaced: a client that asks for
// "market cap" and receives alphabetical order has been lied to.
func parseCompanyFilter(w http.ResponseWriter, r *http.Request, defaultLimit, maxLimit int) (repositories.CompanyFilter, bool) {
	var f repositories.CompanyFilter

	limit, err := queryInt(r, "limit", defaultLimit, 1, maxLimit)
	if err != nil {
		badRequest(w, r, err.Error())
		return f, false
	}
	offset, err := queryInt(r, "offset", 0, 0, 100000)
	if err != nil {
		badRequest(w, r, err.Error())
		return f, false
	}

	f.Query = strings.TrimSpace(r.URL.Query().Get("q"))
	if len(f.Query) > 64 {
		badRequest(w, r, "q is too long")
		return f, false
	}

	f.Sector = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sector")))
	if f.Sector != "" && !sectorRe.MatchString(f.Sector) {
		badRequest(w, r, "sector is not a known code")
		return f, false
	}

	f.Instrument = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("instrument")))
	if len(f.Instrument) > 32 {
		badRequest(w, r, "instrument is too long")
		return f, false
	}
	f.Status = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if f.Status != "" && f.Status != "active" && f.Status != "inactive" {
		badRequest(w, r, "status must be active or inactive")
		return f, false
	}

	f.Sort = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	if f.Sort == "" {
		f.Sort = "symbol"
	}
	valid := false
	for _, name := range repositories.CompanySorts() {
		if name == f.Sort {
			valid = true
			break
		}
	}
	if !valid {
		badRequest(w, r, "sort must be one of "+strings.Join(repositories.CompanySorts(), ", "))
		return f, false
	}

	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("order"))) {
	case "", "asc":
		f.Descending = false
	case "desc":
		f.Descending = true
	default:
		badRequest(w, r, "order must be asc or desc")
		return f, false
	}

	f.Limit = limit
	f.Offset = offset
	return f, true
}

func (s *Server) handleCompanies(w http.ResponseWriter, r *http.Request) {
	f, ok := parseCompanyFilter(w, r, 50, 200)
	if !ok {
		return
	}
	rows, total, err := s.store.ListCompanies(r.Context(), f)
	if err != nil {
		s.internal(w, r, "companies", err)
		return
	}
	session := ""
	if sum, err := s.store.LatestSummary(r.Context()); err == nil {
		session = sum.TradingDate
	}
	meta := s.marketMeta(r, time.Time{}, session)
	meta.Source = "postgres:companies"
	meta.Total = &total
	if covered, eligible, err := s.store.CapitalisationCoverage(r.Context()); err == nil {
		meta.Coverage = &Coverage{Covered: covered, Eligible: eligible, Unit: "companies"}
	}
	OK(w, r, rows, meta)
}

func (s *Server) handleCompany(w http.ResponseWriter, r *http.Request) {
	symbol, ok := cleanSymbol(w, r)
	if !ok {
		return
	}
	c, err := s.store.GetCompany(r.Context(), symbol)
	if err != nil {
		if err == repositories.ErrNotFound {
			Fail(w, r, http.StatusNotFound, CodeNotFound, "unknown symbol")
			return
		}
		s.internal(w, r, "company", err)
		return
	}
	ingested := time.Time{}
	if c.IngestedAt != nil {
		ingested = *c.IngestedAt
	}
	meta := s.marketMeta(r, ingested, c.TradingDate)
	meta.Source = "postgres:companies"
	meta.Method = c.MarketCapMethod
	OK(w, r, c, meta)
}

func (s *Server) handlePrices(w http.ResponseWriter, r *http.Request) {
	symbol, ok := cleanSymbol(w, r)
	if !ok {
		return
	}
	limit, err := queryInt(r, "limit", 120, 1, 1000)
	if err != nil {
		badRequest(w, r, err.Error())
		return
	}
	bars, err := s.store.PriceHistory(r.Context(), symbol, limit)
	if err != nil {
		s.internal(w, r, "prices", err)
		return
	}
	session := ""
	if n := len(bars); n > 0 {
		session = bars[n-1].TradingDate
	}
	meta := metaFrom(r)
	meta.Source = "postgres:daily_ohlcv"
	meta.LicenseClass = "public_unverified"
	meta = s.marketMeta(r, time.Time{}, session)
	OK(w, r, bars, meta)
}

// ---------------------------------------------------------------- exports

// companyCSVHeader is the export's column order. It is stable so a downstream
// spreadsheet can be re-imported without guessing.
var companyCSVHeader = []string{
	"symbol", "name", "sector", "instrument", "status", "trading_date",
	"close", "previous_close", "change", "percent_change",
	"volume", "turnover", "transactions",
	"listed_shares", "public_shares",
	"market_cap", "market_cap_method",
	"market_cap_reported", "market_cap_reported_basis",
	"float_market_cap", "float_market_cap_method",
}

func companyCSVRow(c repositories.Company) []string {
	return []string{
		c.Symbol, c.Name, c.Sector, c.Instrument, c.Status, c.TradingDate,
		c.Close, c.PreviousClose, c.Change, c.PercentChange,
		c.Volume, c.Turnover, c.Transactions,
		c.ListedShares, c.PublicShares,
		c.MarketCap, c.MarketCapMethod,
		c.MarketCapReported, c.MarketCapBasis,
		c.FloatMarketCap, c.FloatCapMethod,
	}
}

// handleCompaniesExport streams the filtered company screen as CSV, stamped with
// when it was produced and what it may be used for.
func (s *Server) handleCompaniesExport(w http.ResponseWriter, r *http.Request) {
	f, ok := parseCompanyFilter(w, r, 1000, 5000)
	if !ok {
		return
	}
	f.Offset = 0
	meta := s.marketMeta(r, time.Time{}, "")

	var rows []repositories.Company
	// One page is enough at the current scale, but the loop keeps a growing
	// directory from being silently truncated at the page limit.
	for {
		page, total, err := s.store.ListCompanies(r.Context(), f)
		if err != nil {
			s.internal(w, r, "companies export", err)
			return
		}
		rows = append(rows, page...)
		if len(page) == 0 || len(rows) >= total || len(rows) >= 5000 {
			break
		}
		f.Offset += len(page)
	}

	generated := time.Now().UTC().Format(time.RFC3339)
	session := ""
	if len(rows) > 0 {
		session = rows[0].TradingDate
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="nepse-companies-`+time.Now().Format("20060102")+`.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Data-Session", session)

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"# nepse platform — listed companies export"})
	_ = cw.Write([]string{"# generated_at", generated})
	_ = cw.Write([]string{"# session_date", session})
	_ = cw.Write([]string{"# freshness", meta.Freshness})
	_ = cw.Write([]string{"# source", "postgres:companies"})
	_ = cw.Write([]string{"# licence", "public_unverified — not for redistribution"})
	if err := cw.Write(companyCSVHeader); err != nil {
		return
	}
	for _, c := range rows {
		if err := cw.Write(companyCSVRow(c)); err != nil {
			return
		}
	}
	cw.Flush()
}

// handlePricesExport streams one security's stored daily bars as CSV.
func (s *Server) handlePricesExport(w http.ResponseWriter, r *http.Request) {
	symbol, ok := cleanSymbol(w, r)
	if !ok {
		return
	}
	bars, err := s.store.PriceHistory(r.Context(), symbol, 1000)
	if err != nil {
		s.internal(w, r, "prices export", err)
		return
	}
	session := ""
	if n := len(bars); n > 0 {
		session = bars[n-1].TradingDate
	}
	meta := s.marketMeta(r, time.Time{}, session)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="nepse-`+strings.ToLower(symbol)+`-prices.csv"`)
	w.Header().Set("Cache-Control", "no-store")

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"# nepse platform — daily price history"})
	_ = cw.Write([]string{"# symbol", symbol})
	_ = cw.Write([]string{"# generated_at", time.Now().UTC().Format(time.RFC3339)})
	_ = cw.Write([]string{"# freshness", meta.Freshness})
	_ = cw.Write([]string{"# licence", "public_unverified — not for redistribution"})
	_ = cw.Write([]string{"trading_date", "open", "high", "low", "close", "previous_close", "volume", "turnover", "transactions", "market_cap"})
	for _, b := range bars {
		_ = cw.Write([]string{b.TradingDate, b.Open, b.High, b.Low, b.Close,
			b.PreviousClose, b.Volume, b.Turnover, b.Transactions, b.MarketCap})
	}
	cw.Flush()
}

// ---------------------------------------------------------------- floorsheet

var (
	brokerRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
	amountRe = regexp.MustCompile(`^[0-9]{1,18}(\.[0-9]{1,6})?$`)
)

var tradeSorts = map[string]bool{
	"contract_desc": true,
	"amount_desc":   true,
	"amount_asc":    true,
	"time_desc":     true,
	"time_asc":      true,
}

// parseTradeFilter validates every floorsheet query parameter once, so the table,
// the count and the CSV export cannot disagree about what a filter means.
func parseTradeFilter(w http.ResponseWriter, r *http.Request, defLimit, maxLimit int) (repositories.TradeFilter, bool) {
	q := r.URL.Query()
	f := repositories.TradeFilter{
		Date:   strings.TrimSpace(q.Get("date")),
		Symbol: strings.ToUpper(strings.TrimSpace(q.Get("symbol"))),
		Broker: strings.TrimSpace(q.Get("broker")),
		Side:   strings.ToLower(strings.TrimSpace(q.Get("side"))),
		Sort:   strings.TrimSpace(q.Get("sort")),
		Cursor: strings.TrimSpace(q.Get("cursor")),
	}

	if f.Date != "" && !dateRe.MatchString(f.Date) {
		badRequest(w, r, "date must be YYYY-MM-DD")
		return f, false
	}
	if f.Symbol != "" && !symbolRe.MatchString(f.Symbol) {
		badRequest(w, r, "symbol is not valid")
		return f, false
	}
	if f.Broker != "" && !brokerRe.MatchString(f.Broker) {
		badRequest(w, r, "broker is not a valid member id")
		return f, false
	}
	if f.Side != "" && f.Side != "buy" && f.Side != "sell" {
		badRequest(w, r, "side must be buy or sell")
		return f, false
	}
	if f.Side != "" && f.Broker == "" {
		badRequest(w, r, "side requires a broker")
		return f, false
	}
	if f.Sort != "" && !tradeSorts[f.Sort] {
		badRequest(w, r, "sort must be one of contract_desc, amount_desc, amount_asc, time_desc, time_asc")
		return f, false
	}
	if f.Cursor != "" && !contractRe.MatchString(f.Cursor) {
		badRequest(w, r, "cursor is not valid")
		return f, false
	}
	for _, key := range []string{"min_amount", "max_amount"} {
		v := strings.TrimSpace(q.Get(key))
		if v != "" && !amountRe.MatchString(v) {
			badRequest(w, r, key+" must be a plain number")
			return f, false
		}
	}
	f.MinAmount = strings.TrimSpace(q.Get("min_amount"))
	f.MaxAmount = strings.TrimSpace(q.Get("max_amount"))
	if f.MinAmount != "" && f.MaxAmount != "" && compareDecimal(f.MinAmount, f.MaxAmount) > 0 {
		badRequest(w, r, "min_amount is greater than max_amount")
		return f, false
	}

	limit, err := queryInt(r, "limit", defLimit, 1, maxLimit)
	if err != nil {
		badRequest(w, r, err.Error())
		return f, false
	}
	f.Limit = limit

	// A cursor and a page are two ways of saying "where to start"; accepting both
	// would let a caller silently get a page they did not ask for.
	page, err := queryInt(r, "page", 1, 1, 100000)
	if err != nil {
		badRequest(w, r, err.Error())
		return f, false
	}
	if f.Cursor != "" && page > 1 {
		badRequest(w, r, "use either cursor or page, not both")
		return f, false
	}
	f.Offset = (page - 1) * limit
	return f, true
}

// compareDecimal orders two plain decimal strings without converting to float64.
func compareDecimal(a, b string) int {
	ai, af := splitDecimal(a)
	bi, bf := splitDecimal(b)
	if c := compareDigitString(ai, bi); c != 0 {
		return c
	}
	width := len(af)
	if len(bf) > width {
		width = len(bf)
	}
	return compareDigitString(padRight(af, width), padRight(bf, width))
}

func splitDecimal(s string) (string, string) {
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat("0", n-len(s))
}

func compareDigitString(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func (s *Server) handleFloorsheet(w http.ResponseWriter, r *http.Request) {
	f, ok := parseTradeFilter(w, r, 50, 500)
	if !ok {
		return
	}

	rows, total, err := s.store.ListTradesPage(r.Context(), f)
	if err != nil {
		s.internal(w, r, "floorsheet", err)
		return
	}
	session := f.Date
	if session == "" && len(rows) > 0 {
		session = rows[0].TradingDate
	}
	meta := s.marketMeta(r, time.Time{}, session)
	meta.Source = "postgres:floorsheet_trades"
	meta.Total = &total
	if f.Cursor == "" && f.Sort == "" && len(rows) == f.Limit && f.Offset+len(rows) < total {
		meta.NextCursor = rows[len(rows)-1].ContractID
	}
	OK(w, r, rows, meta)
}

// handleFloorsheetBrokers returns member totals for one session, together with
// the coverage those totals actually describe.
func (s *Server) handleFloorsheetBrokers(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date != "" && !dateRe.MatchString(date) {
		badRequest(w, r, "date must be YYYY-MM-DD")
		return
	}
	limit, err := queryInt(r, "limit", 50, 1, 500)
	if err != nil {
		badRequest(w, r, err.Error())
		return
	}

	if date == "" {
		cov, err := s.store.FloorsheetCoverageReport(r.Context(), 30)
		if err != nil {
			s.internal(w, r, "floorsheet brokers", err)
			return
		}
		date = cov.LatestPresent
	}
	if date == "" {
		meta := s.marketMeta(r, time.Time{}, "")
		meta.Source = "postgres:floorsheet_broker_summaries"
		OK(w, r, map[string]any{"brokers": []repositories.BrokerSummary{}, "coverage": nil}, meta)
		return
	}

	brokers, err := s.store.BrokerSummaries(r.Context(), date, limit)
	if err != nil {
		s.internal(w, r, "floorsheet brokers", err)
		return
	}
	coverage, err := s.store.BrokerCoverageFor(r.Context(), date)
	if err != nil {
		s.internal(w, r, "floorsheet brokers", err)
		return
	}
	meta := s.marketMeta(r, time.Time{}, date)
	meta.Source = "postgres:floorsheet_trades"
	meta.Method = "sum of contract amounts per member id"
	OK(w, r, map[string]any{"brokers": brokers, "coverage": coverage}, meta)
}

// handleFloorsheetCoverage reports which sessions the archive covers and how it
// was proven, so the UI can distinguish "no trades" from "not captured".
func (s *Server) handleFloorsheetCoverage(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 120, 1, 500)
	if err != nil {
		badRequest(w, r, err.Error())
		return
	}
	report, err := s.store.FloorsheetCoverageReport(r.Context(), limit)
	if err != nil {
		s.internal(w, r, "floorsheet coverage", err)
		return
	}
	runs, err := s.store.LatestFloorsheetRuns(r.Context(), 30)
	if err != nil {
		s.internal(w, r, "floorsheet coverage", err)
		return
	}
	meta := s.marketMeta(r, time.Time{}, report.LatestPresent)
	meta.Source = "postgres:floorsheet_runs"
	OK(w, r, map[string]any{"coverage": report, "runs": runs}, meta)
}

// handleFloorsheetExport streams every contract matching the filter as CSV.
func (s *Server) handleFloorsheetExport(w http.ResponseWriter, r *http.Request) {
	f, ok := parseTradeFilter(w, r, 1000, 5000)
	if !ok {
		return
	}
	f.Cursor = ""
	f.Offset = 0
	session := f.Date
	meta := s.marketMeta(r, time.Time{}, session)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="nepse-floorsheet-`+time.Now().Format("20060102")+`.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	if session != "" {
		w.Header().Set("X-Data-Session", session)
	}

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"# nepse platform — trade tape export"})
	_ = cw.Write([]string{"# generated_at", time.Now().UTC().Format(time.RFC3339)})
	_ = cw.Write([]string{"# session_date", session})
	_ = cw.Write([]string{"# freshness", meta.Freshness})
	_ = cw.Write([]string{"# filters", describeTradeFilter(f)})
	_ = cw.Write([]string{"# source", "postgres:floorsheet_trades"})
	_ = cw.Write([]string{"# licence", "public_unverified — not for redistribution"})
	_ = cw.Write([]string{"contract_id", "trading_date", "symbol", "buyer_broker",
		"seller_broker", "quantity", "rate", "amount", "executed_at"})
	if err := cw.Error(); err != nil {
		return
	}

	n, err := s.store.EachTrade(r.Context(), f, func(t repositories.Trade) error {
		executed := ""
		if t.ExecutedAt != nil {
			executed = t.ExecutedAt.UTC().Format(time.RFC3339Nano)
		}
		return cw.Write([]string{t.ContractID, t.TradingDate, t.Symbol,
			deref(t.BuyerBroker), deref(t.SellerBroker), t.Quantity, t.Rate, t.Amount, executed})
	})
	if err != nil {
		s.logger.Warn("floorsheet export interrupted", "rows", n, "error", err)
	}
	cw.Flush()
}

func describeTradeFilter(f repositories.TradeFilter) string {
	parts := []string{"date=" + orAll(f.Date), "symbol=" + orAll(f.Symbol),
		"broker=" + orAll(f.Broker), "side=" + orAll(f.Side),
		"min_amount=" + orAll(f.MinAmount), "max_amount=" + orAll(f.MaxAmount),
		"sort=" + orAll(f.Sort)}
	return strings.Join(parts, " ")
}

func orAll(v string) string {
	if v == "" {
		return "any"
	}
	return v
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func cleanSymbol(w http.ResponseWriter, r *http.Request) (string, bool) {
	symbol := strings.ToUpper(pathParam(r, "symbol"))
	if !symbolRe.MatchString(symbol) {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "symbol is not valid")
		return "", false
	}
	return symbol, true
}
