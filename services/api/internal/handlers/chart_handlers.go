package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"nepse/terminal/api/internal/indicators"
	"nepse/terminal/api/internal/repositories"
)

// handleOHLCV returns verified historical OHLCV.
// Minute charts require genuine minute data; otherwise DATA_UNAVAILABLE is returned.
func (s *Server) handleOHLCV(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		Fail(w, r, http.StatusServiceUnavailable, CodeUnavailable, "store unavailable")
		return
	}

	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if !symbolRe.MatchString(symbol) {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "invalid or missing symbol")
		return
	}

	interval := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("interval")))
	if interval == "" {
		interval = "1d"
	}

	from := strings.TrimSpace(r.URL.Query().Get("from"))
	if from != "" && !dateRe.MatchString(from) {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "invalid from date (YYYY-MM-DD)")
		return
	}

	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if to != "" && !dateRe.MatchString(to) {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "invalid to date (YYYY-MM-DD)")
		return
	}

	bars, err := s.store.GetOHLCV(r.Context(), symbol, interval, from, to)
	if err != nil {
		if errors.Is(err, repositories.ErrDataUnavailable) {
			Fail(w, r, http.StatusNotFound, "DATA_UNAVAILABLE", err.Error())
			return
		}
		if errors.Is(err, repositories.ErrNotFound) {
			Fail(w, r, http.StatusNotFound, CodeNotFound, "symbol not found")
			return
		}
		s.internal(w, r, "fetch ohlcv", err)
		return
	}

	m := metaFrom(r)
	m.Source = "postgres:daily_ohlcv"
	m.LicenseClass = "public_unverified"
	m.Method = "interval:" + interval
	OK(w, r, bars, m)
}

// handleIndicators computes requested technical indicators (e.g. set=sma:20,ema:50,rsi:14).
func (s *Server) handleIndicators(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		Fail(w, r, http.StatusServiceUnavailable, CodeUnavailable, "store unavailable")
		return
	}

	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if !symbolRe.MatchString(symbol) {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "invalid or missing symbol")
		return
	}

	interval := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("interval")))
	if interval == "" {
		interval = "1d"
	}

	setParam := r.URL.Query().Get("set")
	if setParam == "" {
		setParam = "sma:20,ema:50,rsi:14"
	}

	bars, err := s.store.GetOHLCV(r.Context(), symbol, interval, "", "")
	if err != nil {
		if errors.Is(err, repositories.ErrDataUnavailable) {
			Fail(w, r, http.StatusNotFound, "DATA_UNAVAILABLE", err.Error())
			return
		}
		s.internal(w, r, "fetch ohlcv for indicators", err)
		return
	}

	barInputs := repositories.ConvertToIndicatorBars(bars)
	specs := strings.Split(setParam, ",")
	results := make([]any, 0, len(specs))

	for _, spec := range specs {
		name, params, err := indicators.ParseIndicatorSpec(spec)
		if err != nil {
			continue
		}
		switch name {
		case "sma":
			period := 20
			if len(params) > 0 && params[0] > 0 {
				period = int(params[0])
			}
			res, err := indicators.SMA(barInputs, period)
			if err == nil {
				results = append(results, res)
			}
		case "ema":
			period := 20
			if len(params) > 0 && params[0] > 0 {
				period = int(params[0])
			}
			res, err := indicators.EMA(barInputs, period)
			if err == nil {
				results = append(results, res)
			}
		case "rsi":
			period := 14
			if len(params) > 0 && params[0] > 0 {
				period = int(params[0])
			}
			res, err := indicators.RSI(barInputs, period)
			if err == nil {
				results = append(results, res)
			}
		case "macd":
			fast, slow, signal := 12, 26, 9
			if len(params) >= 3 && params[0] > 0 && params[1] > 0 && params[2] > 0 && params[0] < params[1] {
				fast, slow, signal = int(params[0]), int(params[1]), int(params[2])
			}
			res, err := indicators.MACD(barInputs, fast, slow, signal)
			if err == nil {
				results = append(results, res)
			}
		case "bb":
			period := 20
			stdDev := 2.0
			if len(params) >= 1 && params[0] > 0 {
				period = int(params[0])
			}
			if len(params) >= 2 && params[1] > 0 {
				stdDev = params[1]
			}
			res, err := indicators.BollingerBands(barInputs, period, stdDev)
			if err == nil {
				results = append(results, res)
			}
		case "atr":
			period := 14
			if len(params) > 0 && params[0] > 0 {
				period = int(params[0])
			}
			res, err := indicators.ATR(barInputs, period)
			if err == nil {
				results = append(results, res)
			}
		case "stoch":
			k, d := 14, 3
			if len(params) >= 2 && params[0] > 0 && params[1] > 0 {
				k, d = int(params[0]), int(params[1])
			}
			res, err := indicators.Stochastic(barInputs, k, d)
			if err == nil {
				results = append(results, res)
			}
		case "vwap":
			res, err := indicators.VWAP(barInputs)
			if err == nil {
				results = append(results, res)
			}
		}
	}

	m := metaFrom(r)
	m.Source = "indicator_engine"
	m.Method = setParam
	OK(w, r, results, m)
}

// handleGetChartLayouts returns saved chart layouts.
func (s *Server) handleGetChartLayouts(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		Fail(w, r, http.StatusServiceUnavailable, CodeUnavailable, "store unavailable")
		return
	}

	var uid *string
	if claims, ok := ClaimsFrom(r.Context()); ok {
		uid = &claims.Subject
	}

	layouts, err := s.store.ListChartLayouts(r.Context(), uid)
	if err != nil {
		s.internal(w, r, "list chart layouts", err)
		return
	}
	OK(w, r, layouts, metaFrom(r))
}

// handleSaveChartLayout creates or updates a chart layout.
func (s *Server) handleSaveChartLayout(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		Fail(w, r, http.StatusServiceUnavailable, CodeUnavailable, "store unavailable")
		return
	}

	var req struct {
		Name       string          `json:"name"`
		Symbol     string          `json:"symbol"`
		Interval   string          `json:"interval"`
		ChartType  string          `json:"chart_type"`
		Indicators json.RawMessage `json:"indicators"`
		Settings   json.RawMessage `json:"settings"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "invalid payload: "+err.Error())
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "name is required")
		return
	}
	req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	if req.Symbol == "" {
		req.Symbol = "NEPSE"
	}
	if req.Interval == "" {
		req.Interval = "1d"
	}
	if req.ChartType == "" {
		req.ChartType = "candlestick"
	}

	layout := repositories.ChartLayout{
		Name:       req.Name,
		Symbol:     req.Symbol,
		Interval:   req.Interval,
		ChartType:  req.ChartType,
		Indicators: req.Indicators,
		Settings:   req.Settings,
	}
	if claims, ok := ClaimsFrom(r.Context()); ok {
		layout.UserID = &claims.Subject
	}

	saved, err := s.store.SaveChartLayout(r.Context(), layout)
	if err != nil {
		s.internal(w, r, "save chart layout", err)
		return
	}
	OK(w, r, saved, metaFrom(r))
}

// handleDeleteChartLayout deletes a layout by ID.
func (s *Server) handleDeleteChartLayout(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		Fail(w, r, http.StatusServiceUnavailable, CodeUnavailable, "store unavailable")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "layout id is required")
		return
	}

	var uid *string
	if claims, ok := ClaimsFrom(r.Context()); ok {
		uid = &claims.Subject
	}

	if err := s.store.DeleteChartLayout(r.Context(), id, uid); err != nil {
		s.internal(w, r, "delete chart layout", err)
		return
	}
	OK(w, r, map[string]string{"status": "deleted"}, metaFrom(r))
}

// handleIndexHistory returns historical daily closes for an index.
func (s *Server) handleIndexHistory(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		Fail(w, r, http.StatusServiceUnavailable, CodeUnavailable, "store unavailable")
		return
	}

	code := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "code")))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("code")))
	}
	if code == "" {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "index code required")
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	points, err := s.store.GetIndexHistory(r.Context(), code, from, to)
	if err != nil {
		s.internal(w, r, "fetch index history", err)
		return
	}

	m := metaFrom(r)
	m.Source = "postgres:index_observations"
	m.Method = "granularity:daily"
	OK(w, r, points, m)
}

// handleIndexIntraday returns session intraday points for an index.
func (s *Server) handleIndexIntraday(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		Fail(w, r, http.StatusServiceUnavailable, CodeUnavailable, "store unavailable")
		return
	}

	code := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "code")))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("code")))
	}
	if code == "" {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "index code required")
		return
	}

	date := r.URL.Query().Get("date")

	points, err := s.store.GetIndexIntraday(r.Context(), code, date)
	if err != nil {
		s.internal(w, r, "fetch index intraday", err)
		return
	}

	m := metaFrom(r)
	m.Source = "postgres:index_observations"
	m.Method = "granularity:intraday"
	OK(w, r, points, m)
}

// handleSectorComparison computes sector performance comparison.
func (s *Server) handleSectorComparison(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		Fail(w, r, http.StatusServiceUnavailable, CodeUnavailable, "store unavailable")
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "1d"
	}

	items, err := s.store.GetSectorComparison(r.Context(), period)
	if err != nil {
		s.internal(w, r, "fetch sector comparison", err)
		return
	}

	m := metaFrom(r)
	m.Source = "postgres:index_observations + postgres:sectors"
	m.Method = "period:" + period
	OK(w, r, items, m)
}

// handleMarketBreadth returns market breadth statistics and return distributions.
func (s *Server) handleMarketBreadth(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		Fail(w, r, http.StatusServiceUnavailable, CodeUnavailable, "store unavailable")
		return
	}

	breadth, err := s.store.GetMarketBreadth(r.Context())
	if err != nil {
		s.internal(w, r, "fetch market breadth", err)
		return
	}

	m := metaFrom(r)
	m.Source = "postgres:daily_ohlcv"
	OK(w, r, breadth, m)
}
