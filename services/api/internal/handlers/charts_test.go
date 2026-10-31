//go:build integration

package handlers

import (
	"net/http"
	"testing"
)

func TestChartsOHLCVAndMinuteDataSeparation(t *testing.T) {
	c, _ := newStack(t)

	// 1. Daily historical OHLCV for an existing symbol (ACLBSL)
	status, env := c.do(http.MethodGet, "/api/v1/charts/ohlcv?symbol=ACLBSL&interval=1d", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for daily OHLCV, got %d", status)
	}
	bars, ok := env.Data.([]any)
	if !ok || len(bars) == 0 {
		t.Fatalf("expected bars for ACLBSL, got %v", env.Data)
	}

	// 2. Weekly and monthly resampling
	statusW, _ := c.do(http.MethodGet, "/api/v1/charts/ohlcv?symbol=ACLBSL&interval=1w", nil, nil)
	if statusW != http.StatusOK {
		t.Fatalf("expected 200 for weekly OHLCV, got %d", statusW)
	}

	statusM, _ := c.do(http.MethodGet, "/api/v1/charts/ohlcv?symbol=ACLBSL&interval=1m", nil, nil)
	if statusM != http.StatusOK {
		t.Fatalf("expected 200 for monthly OHLCV, got %d", statusM)
	}

	// 3. Minute charts rule test:
	// "Enable minute charts only when valid minute data exists. Separate actual intraday observations from daily historical closes."
	// When requesting minute intervals without genuine minute trade data (or non-existent symbol), must return DATA_UNAVAILABLE.
	statusMin, minEnv := c.do(http.MethodGet, "/api/v1/charts/ohlcv?symbol=NONEXISTENT_TICKER&interval=5m", nil, nil)
	if statusMin != http.StatusNotFound {
		t.Fatalf("expected 404 DATA_UNAVAILABLE for minute interval with no trade data, got %d", statusMin)
	}
	if minEnv.Error == nil || (minEnv.Error.Code != "DATA_UNAVAILABLE" && minEnv.Error.Code != "NOT_FOUND") {
		t.Errorf("expected DATA_UNAVAILABLE or NOT_FOUND error, got %v", minEnv.Error)
	}
}

func TestIndicatorsEndpointAndWarmup(t *testing.T) {
	c, _ := newStack(t)

	status, env := c.do(http.MethodGet, "/api/v1/charts/indicators?symbol=ACLBSL&interval=1d&set=sma:2,ema:2,rsi:2,macd:2:4:2,bb:2:2,atr:2,vwap", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for indicators, got %d", status)
	}

	dataList, ok := env.Data.([]any)
	if !ok || len(dataList) == 0 {
		t.Fatalf("expected indicator results array, got %v", env.Data)
	}

	foundRSI := false
	for _, raw := range dataList {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if m["indicator"] == "rsi" {
			foundRSI = true
			if warmup, ok := m["warmup_bars"].(float64); !ok || int(warmup) != 2 {
				t.Errorf("expected RSI warmup 2, got %v", m["warmup_bars"])
			}
		}
	}
	if !foundRSI {
		t.Errorf("RSI indicator not found in response")
	}
}

func TestChartLayoutsLifecycle(t *testing.T) {
	c, _ := newStack(t)

	// List initial layouts
	status, _ := c.do(http.MethodGet, "/api/v1/charts/layouts", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 listing layouts, got %d", status)
	}

	// Create a new layout
	layoutPayload := map[string]any{
		"name":       "Test Layout",
		"symbol":     "ACLBSL",
		"interval":   "1d",
		"chart_type": "candlestick",
		"indicators": []map[string]any{
			{"indicator": "sma", "period": 20},
			{"indicator": "rsi", "period": 14},
		},
		"settings": map[string]any{"theme": "dark"},
	}
	saveStatus, saveEnv := c.do(http.MethodPost, "/api/v1/charts/layouts", layoutPayload, nil)
	if saveStatus != http.StatusOK {
		t.Fatalf("expected 200 saving layout, got %d (err: %v)", saveStatus, saveEnv.Error)
	}

	savedData, ok := saveEnv.Data.(map[string]any)
	if !ok || savedData["id"] == nil {
		t.Fatalf("expected layout ID in response, got %v", saveEnv.Data)
	}
	layoutID := savedData["id"].(string)

	// Delete layout
	delStatus, _ := c.do(http.MethodDelete, "/api/v1/charts/layouts/"+layoutID, nil, nil)
	if delStatus != http.StatusOK {
		t.Fatalf("expected 200 deleting layout, got %d", delStatus)
	}
}

func TestIndexHistoryAndIntradaySeparation(t *testing.T) {
	c, _ := newStack(t)

	// Historical daily closes: granularity = 'daily'
	statusHist, histEnv := c.do(http.MethodGet, "/api/v1/indices/nepse/history?granularity=daily", nil, nil)
	if statusHist != http.StatusOK {
		t.Fatalf("expected 200 for index history, got %d", statusHist)
	}
	if points, ok := histEnv.Data.([]any); ok {
		for _, raw := range points {
			if m, ok := raw.(map[string]any); ok {
				if m["granularity"] != "daily" {
					t.Errorf("expected daily granularity in history endpoint, got %v", m["granularity"])
				}
			}
		}
	}

	// Actual session intraday points: granularity = 'intraday'
	statusIntra, intraEnv := c.do(http.MethodGet, "/api/v1/indices/nepse/intraday", nil, nil)
	if statusIntra != http.StatusOK {
		t.Fatalf("expected 200 for index intraday, got %d", statusIntra)
	}
	if points, ok := intraEnv.Data.([]any); ok {
		for _, raw := range points {
			if m, ok := raw.(map[string]any); ok {
				if m["granularity"] != "intraday" {
					t.Errorf("expected intraday granularity in intraday endpoint, got %v", m["granularity"])
				}
			}
		}
	}
}

func TestSectorComparisonAndMarketBreadth(t *testing.T) {
	c, _ := newStack(t)

	statusSector, sectorEnv := c.do(http.MethodGet, "/api/v1/sectors/comparison?period=1d", nil, nil)
	if statusSector != http.StatusOK {
		t.Fatalf("expected 200 for sector comparison, got %d", statusSector)
	}
	if list, ok := sectorEnv.Data.([]any); !ok || len(list) == 0 {
		t.Fatalf("expected sector comparison items, got %v", sectorEnv.Data)
	}

	statusBreadth, breadthEnv := c.do(http.MethodGet, "/api/v1/market/breadth", nil, nil)
	if statusBreadth != http.StatusOK {
		t.Fatalf("expected 200 for market breadth, got %d", statusBreadth)
	}
	breadthMap, ok := breadthEnv.Data.(map[string]any)
	if !ok || breadthMap["distribution"] == nil {
		t.Fatalf("expected distribution map in market breadth, got %v", breadthEnv.Data)
	}
}
