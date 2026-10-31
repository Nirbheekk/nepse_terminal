package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"nepse/terminal/api/internal/indicators"
)

// ErrDataUnavailable indicates that the requested granularity or range is not available.
var ErrDataUnavailable = errors.New("data unavailable")

// OHLCVBar represents a single candlestick or bar in an OHLCV series.
type OHLCVBar struct {
	Time          string `json:"time"`
	Open          string `json:"open"`
	High          string `json:"high"`
	Low           string `json:"low"`
	Close         string `json:"close"`
	Volume        string `json:"volume"`
	Turnover      string `json:"turnover,omitempty"`
	Transactions  string `json:"transactions,omitempty"`
	PreviousClose string `json:"previous_close,omitempty"`
}

// ChartLayout represents a user's saved chart layout.
type ChartLayout struct {
	ID         string          `json:"id"`
	UserID     *string         `json:"user_id,omitempty"`
	Name       string          `json:"name"`
	Symbol     string          `json:"symbol"`
	Interval   string          `json:"interval"`
	ChartType  string          `json:"chart_type"`
	Indicators json.RawMessage `json:"indicators"`
	Settings   json.RawMessage `json:"settings"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// IndexTrendPoint represents a point in an index history or intraday trend.
type IndexTrendPoint struct {
	Time          string `json:"time"`
	Value         string `json:"value"`
	PointChange   string `json:"point_change"`
	PercentChange string `json:"percent_change"`
	Granularity   string `json:"granularity"`
}

// SectorComparisonItem represents one sector's performance in a comparative analysis.
type SectorComparisonItem struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	CurrentValue  string  `json:"current_value"`
	StartValue    string  `json:"start_value"`
	PointChange   string  `json:"point_change"`
	PercentChange float64 `json:"percent_change"`
	Turnover      string  `json:"turnover"`
	MarketCap     string  `json:"market_cap"`
	ScripsCount   int     `json:"scrips_count"`
	WeightPct     float64 `json:"weight_pct"`
}

// MarketBreadthSummary represents the overall market breadth and return distribution.
type MarketBreadthSummary struct {
	Advanced      int             `json:"advanced"`
	Declined      int             `json:"declined"`
	Unchanged     int             `json:"unchanged"`
	Total         int             `json:"total"`
	AdvanceRatio  float64         `json:"advance_ratio"`
	Distribution  map[string]int  `json:"distribution"` // e.g. "lt_minus_5", "minus_5_to_2", etc.
	SectorBreadth []SectorBreadth `json:"by_sector"`
}

// SectorBreadth holds advance/decline stats per sector.
type SectorBreadth struct {
	Sector    string `json:"sector"`
	Advanced  int    `json:"advanced"`
	Declined  int    `json:"declined"`
	Unchanged int    `json:"unchanged"`
}

// GetOHLCV returns verified historical OHLCV bars.
// Minute intervals (1m-1h) require genuine intraday data; otherwise ErrDataUnavailable is returned.
func (s *Store) GetOHLCV(ctx context.Context, symbol, interval, from, to string) ([]OHLCVBar, error) {
	interval = strings.ToLower(strings.TrimSpace(interval))
	if interval == "" {
		interval = "1d"
	}

	// Minute intervals rule: "Enable minute charts only when valid minute data exists.
	// Separate actual intraday observations from daily historical closes."
	isMinute := interval == "1m" || interval == "5m" || interval == "15m" || interval == "30m" || interval == "1h"
	if isMinute {
		return s.getMinuteOHLCV(ctx, symbol, interval, from, to)
	}

	// Daily query
	var whereClauses []string
	args := []any{symbol}

	whereClauses = append(whereClauses, "sec.symbol = $1")
	if from != "" {
		args = append(args, from)
		whereClauses = append(whereClauses, fmt.Sprintf("d.trading_date >= $%d::date", len(args)))
	}
	if to != "" {
		args = append(args, to)
		whereClauses = append(whereClauses, fmt.Sprintf("d.trading_date <= $%d::date", len(args)))
	}

	q := fmt.Sprintf(`
		SELECT d.trading_date::text,
		       COALESCE(d.open, 0)::text,
		       COALESCE(d.high, 0)::text,
		       COALESCE(d.low, 0)::text,
		       COALESCE(d.close, 0)::text,
		       COALESCE(d.volume, 0)::text,
		       COALESCE(d.turnover, 0)::text,
		       COALESCE(d.transactions, 0)::text,
		       COALESCE(d.previous_close, 0)::text
		FROM daily_ohlcv d
		JOIN securities sec ON sec.id = d.security_id
		WHERE %s
		ORDER BY d.trading_date ASC`, strings.Join(whereClauses, " AND "))

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dailyBars []OHLCVBar
	for rows.Next() {
		var b OHLCVBar
		if err := rows.Scan(&b.Time, &b.Open, &b.High, &b.Low, &b.Close,
			&b.Volume, &b.Turnover, &b.Transactions, &b.PreviousClose); err != nil {
			return nil, err
		}
		dailyBars = append(dailyBars, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if interval == "1w" {
		return resampleWeekly(dailyBars), nil
	}
	if interval == "1m" || interval == "1month" || interval == "monthly" {
		return resampleMonthly(dailyBars), nil
	}

	return dailyBars, nil
}

// getMinuteOHLCV aggregates trade executions into minute bars when valid trade data exists.
func (s *Store) getMinuteOHLCV(ctx context.Context, symbol, interval, from, to string) ([]OHLCVBar, error) {
	// Check if there are valid timestamped trades in floorsheet_trades
	var tradeCount int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM floorsheet_trades
		WHERE symbol = $1 AND executed_at IS NOT NULL`, symbol).Scan(&tradeCount)
	if err != nil {
		return nil, err
	}

	if tradeCount == 0 {
		return nil, fmt.Errorf("%w: no genuine minute trade data exists for symbol %s in requested interval %s", ErrDataUnavailable, symbol, interval)
	}

	var bucketSeconds int
	switch interval {
	case "1m":
		bucketSeconds = 60
	case "5m":
		bucketSeconds = 300
	case "15m":
		bucketSeconds = 900
	case "30m":
		bucketSeconds = 1800
	case "1h":
		bucketSeconds = 3600
	default:
		bucketSeconds = 60
	}

	q := `
		SELECT to_char(to_timestamp(floor(extract(epoch from executed_at) / $2) * $2) at time zone 'Asia/Kathmandu', 'YYYY-MM-DD"T"HH24:MI:SS') as bucket,
		       (array_agg(rate ORDER BY executed_at ASC))[1]::text as open_price,
		       max(rate)::text as high_price,
		       min(rate)::text as low_price,
		       (array_agg(rate ORDER BY executed_at DESC))[1]::text as close_price,
		       sum(quantity)::text as volume,
		       sum(amount)::text as turnover,
		       count(*)::text as txns
		FROM floorsheet_trades
		WHERE symbol = $1 AND executed_at IS NOT NULL
		GROUP BY floor(extract(epoch from executed_at) / $2)
		ORDER BY floor(extract(epoch from executed_at) / $2) ASC`

	rows, err := s.pool.Query(ctx, q, symbol, bucketSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bars []OHLCVBar
	for rows.Next() {
		var b OHLCVBar
		if err := rows.Scan(&b.Time, &b.Open, &b.High, &b.Low, &b.Close,
			&b.Volume, &b.Turnover, &b.Transactions); err != nil {
			return nil, err
		}
		bars = append(bars, b)
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("%w: no minute bars generated", ErrDataUnavailable)
	}
	return bars, nil
}

func resampleWeekly(daily []OHLCVBar) []OHLCVBar {
	if len(daily) == 0 {
		return nil
	}
	type weekBucket struct {
		key   string
		start string
		bars  []OHLCVBar
	}
	var buckets []weekBucket

	for _, d := range daily {
		t, err := time.Parse("2006-01-02", d.Time)
		if err != nil {
			continue
		}
		year, week := t.ISOWeek()
		key := fmt.Sprintf("%04d-W%02d", year, week)
		if len(buckets) == 0 || buckets[len(buckets)-1].key != key {
			buckets = append(buckets, weekBucket{key: key, start: d.Time, bars: []OHLCVBar{d}})
		} else {
			buckets[len(buckets)-1].bars = append(buckets[len(buckets)-1].bars, d)
		}
	}

	out := make([]OHLCVBar, 0, len(buckets))
	for _, wb := range buckets {
		b := aggregateBars(wb.bars, wb.start)
		out = append(out, b)
	}
	return out
}

func resampleMonthly(daily []OHLCVBar) []OHLCVBar {
	if len(daily) == 0 {
		return nil
	}
	type monthBucket struct {
		key   string
		start string
		bars  []OHLCVBar
	}
	var buckets []monthBucket

	for _, d := range daily {
		if len(d.Time) < 7 {
			continue
		}
		key := d.Time[:7] // "2026-10"
		if len(buckets) == 0 || buckets[len(buckets)-1].key != key {
			buckets = append(buckets, monthBucket{key: key, start: d.Time, bars: []OHLCVBar{d}})
		} else {
			buckets[len(buckets)-1].bars = append(buckets[len(buckets)-1].bars, d)
		}
	}

	out := make([]OHLCVBar, 0, len(buckets))
	for _, mb := range buckets {
		b := aggregateBars(mb.bars, mb.start)
		out = append(out, b)
	}
	return out
}

func aggregateBars(bars []OHLCVBar, dateLabel string) OHLCVBar {
	if len(bars) == 0 {
		return OHLCVBar{}
	}
	first := bars[0]
	last := bars[len(bars)-1]

	var maxHigh, minLow, totalVol, totalTurnover float64
	var totalTxns int64
	minLow = math.MaxFloat64

	for _, b := range bars {
		h, _ := strconv.ParseFloat(b.High, 64)
		l, _ := strconv.ParseFloat(b.Low, 64)
		v, _ := strconv.ParseFloat(b.Volume, 64)
		tn, _ := strconv.ParseFloat(b.Turnover, 64)
		tx, _ := strconv.ParseInt(b.Transactions, 10, 64)

		if h > maxHigh {
			maxHigh = h
		}
		if l > 0 && l < minLow {
			minLow = l
		}
		totalVol += v
		totalTurnover += tn
		totalTxns += tx
	}
	if minLow == math.MaxFloat64 {
		minLow = 0
	}

	return OHLCVBar{
		Time:          dateLabel,
		Open:          first.Open,
		High:          strconv.FormatFloat(maxHigh, 'f', -1, 64),
		Low:           strconv.FormatFloat(minLow, 'f', -1, 64),
		Close:         last.Close,
		Volume:        strconv.FormatFloat(totalVol, 'f', -1, 64),
		Turnover:      strconv.FormatFloat(totalTurnover, 'f', -1, 64),
		Transactions:  strconv.FormatInt(totalTxns, 10),
		PreviousClose: first.PreviousClose,
	}
}

// ConvertToIndicatorBars converts OHLCVBars to indicator BarInput.
func ConvertToIndicatorBars(bars []OHLCVBar) []indicators.BarInput {
	out := make([]indicators.BarInput, len(bars))
	for i, b := range bars {
		op, _ := strconv.ParseFloat(b.Open, 64)
		hi, _ := strconv.ParseFloat(b.High, 64)
		lo, _ := strconv.ParseFloat(b.Low, 64)
		cl, _ := strconv.ParseFloat(b.Close, 64)
		vo, _ := strconv.ParseFloat(b.Volume, 64)
		out[i] = indicators.BarInput{
			Time:   b.Time,
			Open:   op,
			High:   hi,
			Low:    lo,
			Close:  cl,
			Volume: vo,
		}
	}
	return out
}

// GetIndexHistory returns historical daily closes for an index definition.
// Separated strictly from intraday session observations.
func (s *Store) GetIndexHistory(ctx context.Context, code, from, to string) ([]IndexTrendPoint, error) {
	var whereClauses []string
	args := []any{code}

	whereClauses = append(whereClauses, "d.code = $1 AND o.granularity = 'daily'")
	if from != "" {
		args = append(args, from)
		whereClauses = append(whereClauses, fmt.Sprintf("o.trading_date >= $%d::date", len(args)))
	}
	if to != "" {
		args = append(args, to)
		whereClauses = append(whereClauses, fmt.Sprintf("o.trading_date <= $%d::date", len(args)))
	}

	q := fmt.Sprintf(`
		SELECT o.trading_date::text,
		       COALESCE(o.value, 0)::text,
		       COALESCE(o.point_change, 0)::text,
		       COALESCE(o.percent_change, 0)::text,
		       o.granularity
		FROM index_observations o
		JOIN index_definitions d ON d.id = o.index_id
		WHERE %s
		ORDER BY o.trading_date ASC, o.observed_at ASC`, strings.Join(whereClauses, " AND "))

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []IndexTrendPoint
	for rows.Next() {
		var p IndexTrendPoint
		if err := rows.Scan(&p.Time, &p.Value, &p.PointChange, &p.PercentChange, &p.Granularity); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetIndexIntraday returns session intraday observations for an index.
func (s *Store) GetIndexIntraday(ctx context.Context, code, date string) ([]IndexTrendPoint, error) {
	args := []any{code}
	where := "d.code = $1 AND o.granularity = 'intraday'"
	if date != "" {
		args = append(args, date)
		where += " AND o.trading_date = $2::date"
	}

	q := fmt.Sprintf(`
		SELECT to_char(o.observed_at at time zone 'Asia/Kathmandu', 'YYYY-MM-DD"T"HH24:MI:SS') as obs_time,
		       COALESCE(o.value, 0)::text,
		       COALESCE(o.point_change, 0)::text,
		       COALESCE(o.percent_change, 0)::text,
		       o.granularity
		FROM index_observations o
		JOIN index_definitions d ON d.id = o.index_id
		WHERE %s
		ORDER BY o.observed_at ASC`, where)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []IndexTrendPoint
	for rows.Next() {
		var p IndexTrendPoint
		if err := rows.Scan(&p.Time, &p.Value, &p.PointChange, &p.PercentChange, &p.Granularity); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetSectorComparison computes multi-period sector performance comparisons.
func (s *Store) GetSectorComparison(ctx context.Context, period string) ([]SectorComparisonItem, error) {
	q := `
		WITH sector_info AS (
			SELECT s.code, s.name, count(c.id) as scrips_count
			FROM sectors s
			JOIN companies c ON c.sector_id = s.id AND c.origin = 'registry'
			GROUP BY s.code, s.name
		),
		latest_obs AS (
			SELECT DISTINCT ON (d.code)
			       d.code,
			       o.value as latest_val,
			       o.point_change,
			       o.percent_change,
			       o.trading_date
			FROM index_definitions d
			JOIN index_observations o ON o.index_id = d.id AND o.granularity = 'daily'
			ORDER BY d.code, o.trading_date DESC, o.observed_at DESC
		)
		SELECT si.code, si.name,
		       COALESCE(lo.latest_val, 0)::text,
		       COALESCE(lo.latest_val - lo.point_change, lo.latest_val, 0)::text as start_val,
		       COALESCE(lo.point_change, 0)::text,
		       COALESCE(lo.percent_change, 0),
		       si.scrips_count
		FROM sector_info si
		LEFT JOIN latest_obs lo ON lo.code = si.code
		ORDER BY lo.percent_change DESC NULLS LAST`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SectorComparisonItem
	for rows.Next() {
		var it SectorComparisonItem
		if err := rows.Scan(&it.Code, &it.Name, &it.CurrentValue, &it.StartValue,
			&it.PointChange, &it.PercentChange, &it.ScripsCount); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// GetMarketBreadth calculates advancing, declining, unchanged numbers and return distribution.
func (s *Store) GetMarketBreadth(ctx context.Context) (*MarketBreadthSummary, error) {
	q := `
		WITH latest_prices AS (
			SELECT DISTINCT ON (c.id)
			       c.id,
			       s.name as sector,
			       d.close,
			       d.previous_close,
			       CASE
			           WHEN d.previous_close > 0 THEN ((d.close - d.previous_close) / d.previous_close) * 100
			           ELSE 0
			       END as pct_change
			FROM companies c
			LEFT JOIN sectors s ON s.id = c.sector_id
			JOIN securities sec ON sec.company_id = c.id
			JOIN daily_ohlcv d ON d.security_id = sec.id
			WHERE c.origin = 'registry'
			ORDER BY c.id, (sec.symbol = c.symbol) DESC, d.trading_date DESC
		)
		SELECT
		    count(*) FILTER (WHERE pct_change > 0) as advanced,
		    count(*) FILTER (WHERE pct_change < 0) as declined,
		    count(*) FILTER (WHERE pct_change = 0) as unchanged,
		    count(*) as total,
		    count(*) FILTER (WHERE pct_change <= -5) as lt_minus_5,
		    count(*) FILTER (WHERE pct_change > -5 AND pct_change <= -2) as minus_5_to_2,
		    count(*) FILTER (WHERE pct_change > -2 AND pct_change < 0) as minus_2_to_0,
		    count(*) FILTER (WHERE pct_change = 0) as zero_bucket,
		    count(*) FILTER (WHERE pct_change > 0 AND pct_change <= 2) as plus_0_to_2,
		    count(*) FILTER (WHERE pct_change > 2 AND pct_change <= 5) as plus_2_to_5,
		    count(*) FILTER (WHERE pct_change > 5) as gt_plus_5
		FROM latest_prices`

	var b MarketBreadthSummary
	b.Distribution = make(map[string]int)

	var lt5, m52, m20, zero, p02, p25, gt5 int
	err := s.pool.QueryRow(ctx, q).Scan(
		&b.Advanced, &b.Declined, &b.Unchanged, &b.Total,
		&lt5, &m52, &m20, &zero, &p02, &p25, &gt5,
	)
	if err != nil {
		return nil, err
	}

	b.Distribution["lt_minus_5"] = lt5
	b.Distribution["minus_5_to_2"] = m52
	b.Distribution["minus_2_to_0"] = m20
	b.Distribution["zero"] = zero
	b.Distribution["plus_0_to_2"] = p02
	b.Distribution["plus_2_to_5"] = p25
	b.Distribution["gt_plus_5"] = gt5

	if b.Declined > 0 {
		b.AdvanceRatio = math.Round((float64(b.Advanced)/float64(b.Declined))*100) / 100
	} else if b.Advanced > 0 {
		b.AdvanceRatio = float64(b.Advanced)
	}

	return &b, nil
}

// ListChartLayouts lists saved chart layouts for a user or public/guest layouts.
func (s *Store) ListChartLayouts(ctx context.Context, userID *string) ([]ChartLayout, error) {
	var q string
	var args []any
	if userID != nil && *userID != "" {
		q = `
			SELECT id::text, user_id::text, name, symbol, interval, chart_type, indicators, settings, created_at, updated_at
			FROM chart_layouts
			WHERE user_id = $1::uuid OR user_id IS NULL
			ORDER BY updated_at DESC`
		args = append(args, *userID)
	} else {
		q = `
			SELECT id::text, user_id::text, name, symbol, interval, chart_type, indicators, settings, created_at, updated_at
			FROM chart_layouts
			ORDER BY updated_at DESC
			LIMIT 50`
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChartLayout
	for rows.Next() {
		var l ChartLayout
		var uID *string
		if err := rows.Scan(&l.ID, &uID, &l.Name, &l.Symbol, &l.Interval, &l.ChartType,
			&l.Indicators, &l.Settings, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		l.UserID = uID
		out = append(out, l)
	}
	return out, rows.Err()
}

// SaveChartLayout creates or updates a chart layout.
func (s *Store) SaveChartLayout(ctx context.Context, l ChartLayout) (*ChartLayout, error) {
	q := `
		INSERT INTO chart_layouts (user_id, name, symbol, interval, chart_type, indicators, settings, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		RETURNING id::text, created_at, updated_at`

	var uid any
	if l.UserID != nil && *l.UserID != "" {
		uid = *l.UserID
	}

	if len(l.Indicators) == 0 {
		l.Indicators = json.RawMessage("[]")
	}
	if len(l.Settings) == 0 {
		l.Settings = json.RawMessage("{}")
	}

	err := s.pool.QueryRow(ctx, q, uid, l.Name, l.Symbol, l.Interval, l.ChartType, l.Indicators, l.Settings).
		Scan(&l.ID, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// DeleteChartLayout deletes a layout by ID.
func (s *Store) DeleteChartLayout(ctx context.Context, id string, userID *string) error {
	var err error
	if userID != nil && *userID != "" {
		_, err = s.pool.Exec(ctx, `DELETE FROM chart_layouts WHERE id = $1::uuid AND (user_id = $2::uuid OR user_id IS NULL)`, id, *userID)
	} else {
		_, err = s.pool.Exec(ctx, `DELETE FROM chart_layouts WHERE id = $1::uuid`, id)
	}
	return err
}
