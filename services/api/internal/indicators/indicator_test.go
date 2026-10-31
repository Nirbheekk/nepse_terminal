package indicators

import (
	"math"
	"testing"
)

func makeTestBars(closes []float64) []BarInput {
	bars := make([]BarInput, len(closes))
	for i, c := range closes {
		bars[i] = BarInput{
			Time:   "2026-10-01",
			Open:   c - 1.0,
			High:   c + 2.0,
			Low:    c - 2.0,
			Close:  c,
			Volume: 100.0,
		}
	}
	return bars
}

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestSMA(t *testing.T) {
	prices := []float64{10, 11, 12, 13, 14, 15}
	bars := makeTestBars(prices)
	res, err := SMA(bars, 3)
	if err != nil {
		t.Fatalf("SMA failed: %v", err)
	}
	if res.WarmupBars != 2 {
		t.Errorf("expected warmup 2, got %d", res.WarmupBars)
	}
	if res.Series[0].V != nil || res.Series[1].V != nil {
		t.Errorf("expected nil for warmup bars")
	}
	// bar 2: (10 + 11 + 12)/3 = 11.0
	if res.Series[2].V == nil || !almostEqual(*res.Series[2].V, 11.0, 1e-4) {
		t.Errorf("bar 2 SMA: want 11.0, got %v", res.Series[2].V)
	}
	// bar 5: (13 + 14 + 15)/3 = 14.0
	if res.Series[5].V == nil || !almostEqual(*res.Series[5].V, 14.0, 1e-4) {
		t.Errorf("bar 5 SMA: want 14.0, got %v", res.Series[5].V)
	}
}

func TestEMA(t *testing.T) {
	prices := []float64{10, 12, 14, 16, 18}
	bars := makeTestBars(prices)
	period := 3
	res, err := EMA(bars, period)
	if err != nil {
		t.Fatalf("EMA failed: %v", err)
	}
	if res.WarmupBars != 2 {
		t.Errorf("expected warmup 2, got %d", res.WarmupBars)
	}
	// bar 2 (initial EMA is SMA): (10 + 12 + 14)/3 = 12.0
	if res.Series[2].V == nil || !almostEqual(*res.Series[2].V, 12.0, 1e-4) {
		t.Errorf("bar 2 initial EMA: want 12.0, got %v", res.Series[2].V)
	}
	// k = 2 / (3 + 1) = 0.5
	// bar 3: 16 * 0.5 + 12.0 * 0.5 = 14.0
	if res.Series[3].V == nil || !almostEqual(*res.Series[3].V, 14.0, 1e-4) {
		t.Errorf("bar 3 EMA: want 14.0, got %v", res.Series[3].V)
	}
	// bar 4: 18 * 0.5 + 14.0 * 0.5 = 16.0
	if res.Series[4].V == nil || !almostEqual(*res.Series[4].V, 16.0, 1e-4) {
		t.Errorf("bar 4 EMA: want 16.0, got %v", res.Series[4].V)
	}
}

func TestRSI(t *testing.T) {
	// Monotonically increasing prices: RSI should be 100 after warmup
	prices := []float64{10, 12, 14, 16, 18, 20}
	bars := makeTestBars(prices)
	res, err := RSI(bars, 3)
	if err != nil {
		t.Fatalf("RSI failed: %v", err)
	}
	if res.WarmupBars != 3 {
		t.Errorf("expected warmup 3, got %d", res.WarmupBars)
	}
	if res.Series[3].V == nil || !almostEqual(*res.Series[3].V, 100.0, 1e-4) {
		t.Errorf("RSI of pure gains: want 100.0, got %v", res.Series[3].V)
	}

	// Flat prices: RSI should be 50.0
	flatPrices := []float64{20, 20, 20, 20, 20, 20}
	flatBars := makeTestBars(flatPrices)
	resFlat, err := RSI(flatBars, 3)
	if err != nil {
		t.Fatalf("RSI failed: %v", err)
	}
	if resFlat.Series[3].V == nil || !almostEqual(*resFlat.Series[3].V, 50.0, 1e-4) {
		t.Errorf("RSI of flat prices: want 50.0, got %v", resFlat.Series[3].V)
	}
}

func TestBollingerBands(t *testing.T) {
	// Standard deviation of [10, 10, 10] is 0
	prices := []float64{10, 10, 10, 10, 10}
	bars := makeTestBars(prices)
	res, err := BollingerBands(bars, 3, 2.0)
	if err != nil {
		t.Fatalf("BollingerBands failed: %v", err)
	}
	if res.Series[2].Middle == nil || !almostEqual(*res.Series[2].Middle, 10.0, 1e-4) {
		t.Errorf("BB Middle: want 10.0, got %v", res.Series[2].Middle)
	}
	if res.Series[2].Upper == nil || !almostEqual(*res.Series[2].Upper, 10.0, 1e-4) {
		t.Errorf("BB Upper zero variance: want 10.0, got %v", res.Series[2].Upper)
	}

	// Dynamic series [10, 12, 14]
	// Mean = 12. Variance = ((10-12)^2 + (12-12)^2 + (14-12)^2)/3 = 8/3 = 2.666667
	// StdDev = sqrt(8/3) ~= 1.632993
	// Upper = 12 + 2 * 1.632993 = 15.265986
	// Lower = 12 - 2 * 1.632993 = 8.734014
	prices2 := []float64{10, 12, 14}
	bars2 := makeTestBars(prices2)
	res2, err := BollingerBands(bars2, 3, 2.0)
	if err != nil {
		t.Fatalf("BollingerBands failed: %v", err)
	}
	if res2.Series[2].Upper == nil || !almostEqual(*res2.Series[2].Upper, 15.265986, 1e-3) {
		t.Errorf("BB Upper: want ~15.266, got %v", res2.Series[2].Upper)
	}
	if res2.Series[2].Lower == nil || !almostEqual(*res2.Series[2].Lower, 8.734014, 1e-3) {
		t.Errorf("BB Lower: want ~8.734, got %v", res2.Series[2].Lower)
	}
}

func TestMACD(t *testing.T) {
	prices := []float64{
		10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
		20, 21, 22, 23, 24, 25, 26, 27, 28, 29,
		30, 31, 32, 33, 34, 35, 36, 37, 38, 39,
	}
	bars := makeTestBars(prices)
	res, err := MACD(bars, 3, 6, 3)
	if err != nil {
		t.Fatalf("MACD failed: %v", err)
	}
	// slow = 6, signal = 3 => warmup = slow + signal - 2 = 7 (indices 0..6 are nil)
	if res.WarmupBars != 7 {
		t.Errorf("expected warmup 7, got %d", res.WarmupBars)
	}
	if res.Series[6].Signal != nil {
		t.Errorf("expected Signal nil at index 6")
	}
	if res.Series[7].Signal == nil {
		t.Errorf("expected Signal not nil at index 7")
	}
	if res.Series[7].Hist == nil {
		t.Errorf("expected Hist not nil at index 7")
	}
	// Verify Hist = MACD - Signal
	histDiff := *res.Series[7].MACD - *res.Series[7].Signal
	if !almostEqual(*res.Series[7].Hist, histDiff, 1e-4) {
		t.Errorf("Hist mismatch: %v vs %v", *res.Series[7].Hist, histDiff)
	}
}

func TestStochastic(t *testing.T) {
	bars := []BarInput{
		{Time: "1", Low: 10, High: 20, Close: 15},
		{Time: "2", Low: 12, High: 22, Close: 22},
		{Time: "3", Low: 11, High: 25, Close: 18},
	}
	res, err := Stochastic(bars, 2, 2)
	if err != nil {
		t.Fatalf("Stochastic failed: %v", err)
	}
	// Bar 1: lowMin = 10, highMax = 22, close = 22 => %K = (22-10)/(22-10)*100 = 100
	if res.Series[1].K == nil || !almostEqual(*res.Series[1].K, 100.0, 1e-4) {
		t.Errorf("Stochastic bar 1 %%K: want 100.0, got %v", res.Series[1].K)
	}
	// Bar 2: lowMin = 11, highMax = 25, close = 18 => %K = (18-11)/(25-11)*100 = 7/14*100 = 50.0
	if res.Series[2].K == nil || !almostEqual(*res.Series[2].K, 50.0, 1e-4) {
		t.Errorf("Stochastic bar 2 %%K: want 50.0, got %v", res.Series[2].K)
	}
	// %D at bar 2 = (100.0 + 50.0) / 2 = 75.0
	if res.Series[2].D == nil || !almostEqual(*res.Series[2].D, 75.0, 1e-4) {
		t.Errorf("Stochastic bar 2 %%D: want 75.0, got %v", res.Series[2].D)
	}
}

func TestATR(t *testing.T) {
	bars := []BarInput{
		{Time: "1", Low: 90, High: 100, Close: 95},
		{Time: "2", Low: 92, High: 104, Close: 102},
		{Time: "3", Low: 98, High: 108, Close: 105},
	}
	res, err := ATR(bars, 2)
	if err != nil {
		t.Fatalf("ATR failed: %v", err)
	}
	// TR0 = 100 - 90 = 10
	// TR1 = max(104-92=12, |104-95|=9, |92-95|=3) = 12
	// ATR at index 1 = (10 + 12)/2 = 11.0
	if res.Series[1].V == nil || !almostEqual(*res.Series[1].V, 11.0, 1e-4) {
		t.Errorf("ATR bar 1: want 11.0, got %v", res.Series[1].V)
	}
}

func TestVWAP(t *testing.T) {
	bars := []BarInput{
		{Time: "1", High: 10, Low: 10, Close: 10, Volume: 100}, // TP=10, PV=1000, cumV=100, VWAP=10
		{Time: "2", High: 20, Low: 20, Close: 20, Volume: 100}, // TP=20, PV=2000, cumPV=3000, cumV=200, VWAP=15
	}
	res, err := VWAP(bars)
	if err != nil {
		t.Fatalf("VWAP failed: %v", err)
	}
	if res.Series[0].V == nil || !almostEqual(*res.Series[0].V, 10.0, 1e-4) {
		t.Errorf("VWAP bar 0: want 10.0, got %v", res.Series[0].V)
	}
	if res.Series[1].V == nil || !almostEqual(*res.Series[1].V, 15.0, 1e-4) {
		t.Errorf("VWAP bar 1: want 15.0, got %v", res.Series[1].V)
	}
}

func TestParseIndicatorSpec(t *testing.T) {
	name, params, err := ParseIndicatorSpec("sma:20")
	if err != nil || name != "sma" || len(params) != 1 || params[0] != 20 {
		t.Errorf("failed parsing sma:20: name=%s, params=%v, err=%v", name, params, err)
	}

	name, params, err = ParseIndicatorSpec("macd:12:26:9")
	if err != nil || name != "macd" || len(params) != 3 || params[0] != 12 || params[1] != 26 || params[2] != 9 {
		t.Errorf("failed parsing macd:12:26:9: name=%s, params=%v, err=%v", name, params, err)
	}
}
