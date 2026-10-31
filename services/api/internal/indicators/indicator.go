// Package indicator provides deterministic, audited technical indicator calculations
// for financial time series with explicit warmup periods and boundary guarantees.
package indicators

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// BarInput represents the minimal OHLCV observation required for indicator calculation.
type BarInput struct {
	Time   string  `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// Point represents a single point in a single-value indicator series.
// V is nil during the warmup phase.
type Point struct {
	T string   `json:"t"`
	V *float64 `json:"v"`
}

// Result represents a single-value indicator response payload.
type Result struct {
	Indicator  string         `json:"indicator"`
	Params     map[string]any `json:"params"`
	WarmupBars int            `json:"warmup_bars"`
	Series     []Point        `json:"series"`
}

// MACDPoint represents a single observation for the MACD indicator.
type MACDPoint struct {
	T      string   `json:"t"`
	MACD   *float64 `json:"macd"`
	Signal *float64 `json:"signal"`
	Hist   *float64 `json:"hist"`
}

// MACDResult represents the MACD indicator response payload.
type MACDResult struct {
	Indicator  string         `json:"indicator"`
	Params     map[string]any `json:"params"`
	WarmupBars int            `json:"warmup_bars"`
	Series     []MACDPoint    `json:"series"`
}

// BBPoint represents a single observation of Bollinger Bands.
type BBPoint struct {
	T      string   `json:"t"`
	Upper  *float64 `json:"upper"`
	Middle *float64 `json:"middle"`
	Lower  *float64 `json:"lower"`
}

// BBResult represents the Bollinger Bands indicator response payload.
type BBResult struct {
	Indicator  string         `json:"indicator"`
	Params     map[string]any `json:"params"`
	WarmupBars int            `json:"warmup_bars"`
	Series     []BBPoint      `json:"series"`
}

// StochPoint represents a single observation of Stochastic Oscillator.
type StochPoint struct {
	T string   `json:"t"`
	K *float64 `json:"k"`
	D *float64 `json:"d"`
}

// StochResult represents the Stochastic Oscillator response payload.
type StochResult struct {
	Indicator  string         `json:"indicator"`
	Params     map[string]any `json:"params"`
	WarmupBars int            `json:"warmup_bars"`
	Series     []StochPoint   `json:"series"`
}

func round6(val float64) float64 {
	return math.Round(val*1e6) / 1e6
}

func ptrFloat(f float64) *float64 {
	r := round6(f)
	return &r
}

// SMA computes the Simple Moving Average over close prices for the given period.
func SMA(bars []BarInput, period int) (Result, error) {
	if period <= 0 {
		return Result{}, fmt.Errorf("SMA period must be positive, got %d", period)
	}
	n := len(bars)
	res := Result{
		Indicator:  "sma",
		Params:     map[string]any{"period": period},
		WarmupBars: period - 1,
		Series:     make([]Point, n),
	}
	if n == 0 {
		return res, nil
	}

	var sum float64
	for i := 0; i < n; i++ {
		sum += bars[i].Close
		if i >= period {
			sum -= bars[i-period].Close
		}
		res.Series[i].T = bars[i].Time
		if i >= period-1 {
			res.Series[i].V = ptrFloat(sum / float64(period))
		}
	}
	return res, nil
}

// EMA computes the Exponential Moving Average over close prices.
// Initialized with the SMA of the first period bars.
func EMA(bars []BarInput, period int) (Result, error) {
	if period <= 0 {
		return Result{}, fmt.Errorf("EMA period must be positive, got %d", period)
	}
	n := len(bars)
	res := Result{
		Indicator:  "ema",
		Params:     map[string]any{"period": period},
		WarmupBars: period - 1,
		Series:     make([]Point, n),
	}
	if n == 0 {
		return res, nil
	}

	k := 2.0 / float64(period+1)
	var sum float64
	var prevEMA float64

	for i := 0; i < n; i++ {
		res.Series[i].T = bars[i].Time
		if i < period {
			sum += bars[i].Close
			if i == period-1 {
				prevEMA = sum / float64(period)
				res.Series[i].V = ptrFloat(prevEMA)
			}
		} else {
			currentEMA := (bars[i].Close * k) + (prevEMA * (1.0 - k))
			res.Series[i].V = ptrFloat(currentEMA)
			prevEMA = currentEMA
		}
	}
	return res, nil
}

// RSI computes Wilder's Relative Strength Index (0 to 100) over close prices.
func RSI(bars []BarInput, period int) (Result, error) {
	if period <= 0 {
		return Result{}, fmt.Errorf("RSI period must be positive, got %d", period)
	}
	n := len(bars)
	res := Result{
		Indicator:  "rsi",
		Params:     map[string]any{"period": period},
		WarmupBars: period,
		Series:     make([]Point, n),
	}
	if n == 0 {
		return res, nil
	}

	var sumGain, sumLoss float64
	var avgGain, avgLoss float64

	for i := 0; i < n; i++ {
		res.Series[i].T = bars[i].Time
		if i == 0 {
			continue
		}
		diff := bars[i].Close - bars[i-1].Close
		gain := math.Max(diff, 0)
		loss := math.Max(-diff, 0)

		if i <= period {
			sumGain += gain
			sumLoss += loss
			if i == period {
				avgGain = sumGain / float64(period)
				avgLoss = sumLoss / float64(period)
				var rsiVal float64
				if avgLoss == 0 {
					if avgGain == 0 {
						rsiVal = 50.0
					} else {
						rsiVal = 100.0
					}
				} else {
					rs := avgGain / avgLoss
					rsiVal = 100.0 - (100.0 / (1.0 + rs))
				}
				res.Series[i].V = ptrFloat(rsiVal)
			}
		} else {
			avgGain = (avgGain*float64(period-1) + gain) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
			var rsiVal float64
			if avgLoss == 0 {
				if avgGain == 0 {
					rsiVal = 50.0
				} else {
					rsiVal = 100.0
				}
			} else {
				rs := avgGain / avgLoss
				rsiVal = 100.0 - (100.0 / (1.0 + rs))
			}
			res.Series[i].V = ptrFloat(rsiVal)
		}
	}
	return res, nil
}

// MACD computes Moving Average Convergence Divergence.
// Standard params: fast = 12, slow = 26, signal = 9.
func MACD(bars []BarInput, fast, slow, signal int) (MACDResult, error) {
	if fast <= 0 || slow <= 0 || signal <= 0 {
		return MACDResult{}, fmt.Errorf("MACD parameters must be positive, got (%d, %d, %d)", fast, slow, signal)
	}
	if fast >= slow {
		return MACDResult{}, fmt.Errorf("MACD fast period (%d) must be less than slow period (%d)", fast, slow)
	}
	n := len(bars)
	warmup := slow + signal - 2
	res := MACDResult{
		Indicator:  "macd",
		Params:     map[string]any{"fast": fast, "slow": slow, "signal": signal},
		WarmupBars: warmup,
		Series:     make([]MACDPoint, n),
	}
	if n == 0 {
		return res, nil
	}

	fastRes, err := EMA(bars, fast)
	if err != nil {
		return res, err
	}
	slowRes, err := EMA(bars, slow)
	if err != nil {
		return res, err
	}

	macdValues := make([]*float64, n)
	for i := 0; i < n; i++ {
		res.Series[i].T = bars[i].Time
		if fastRes.Series[i].V != nil && slowRes.Series[i].V != nil {
			v := *fastRes.Series[i].V - *slowRes.Series[i].V
			macdValues[i] = &v
			res.Series[i].MACD = ptrFloat(v)
		}
	}

	// Calculate Signal line: EMA of MACD values over valid MACD entries
	kSignal := 2.0 / float64(signal+1)
	var signalSum float64
	validCount := 0
	var prevSignal float64

	for i := 0; i < n; i++ {
		if macdValues[i] == nil {
			continue
		}
		validCount++
		val := *macdValues[i]
		if validCount <= signal {
			signalSum += val
			if validCount == signal {
				prevSignal = signalSum / float64(signal)
				res.Series[i].Signal = ptrFloat(prevSignal)
				diff := val - prevSignal
				res.Series[i].Hist = ptrFloat(diff)
			}
		} else {
			currentSignal := (val * kSignal) + (prevSignal * (1.0 - kSignal))
			res.Series[i].Signal = ptrFloat(currentSignal)
			diff := val - currentSignal
			res.Series[i].Hist = ptrFloat(diff)
			prevSignal = currentSignal
		}
	}
	return res, nil
}

// BollingerBands computes Bollinger Bands (Upper, Middle, Lower).
func BollingerBands(bars []BarInput, period int, stdDev float64) (BBResult, error) {
	if period <= 0 {
		return BBResult{}, fmt.Errorf("Bollinger Bands period must be positive, got %d", period)
	}
	if stdDev <= 0 {
		return BBResult{}, fmt.Errorf("Bollinger Bands stdDev must be positive, got %f", stdDev)
	}
	n := len(bars)
	res := BBResult{
		Indicator:  "bb",
		Params:     map[string]any{"period": period, "std_dev": stdDev},
		WarmupBars: period - 1,
		Series:     make([]BBPoint, n),
	}
	if n == 0 {
		return res, nil
	}

	smaRes, err := SMA(bars, period)
	if err != nil {
		return res, err
	}

	for i := 0; i < n; i++ {
		res.Series[i].T = bars[i].Time
		if smaRes.Series[i].V == nil {
			continue
		}
		mean := *smaRes.Series[i].V
		var varSum float64
		for j := i - period + 1; j <= i; j++ {
			diff := bars[j].Close - mean
			varSum += diff * diff
		}
		sd := math.Sqrt(varSum / float64(period))
		upper := mean + (stdDev * sd)
		lower := mean - (stdDev * sd)

		res.Series[i].Middle = ptrFloat(mean)
		res.Series[i].Upper = ptrFloat(upper)
		res.Series[i].Lower = ptrFloat(lower)
	}
	return res, nil
}

// ATR computes Average True Range over high, low, close prices.
func ATR(bars []BarInput, period int) (Result, error) {
	if period <= 0 {
		return Result{}, fmt.Errorf("ATR period must be positive, got %d", period)
	}
	n := len(bars)
	res := Result{
		Indicator:  "atr",
		Params:     map[string]any{"period": period},
		WarmupBars: period - 1,
		Series:     make([]Point, n),
	}
	if n == 0 {
		return res, nil
	}

	var trSum float64
	var prevATR float64

	for i := 0; i < n; i++ {
		res.Series[i].T = bars[i].Time
		var tr float64
		if i == 0 {
			tr = bars[i].High - bars[i].Low
		} else {
			hl := bars[i].High - bars[i].Low
			hpc := math.Abs(bars[i].High - bars[i-1].Close)
			lpc := math.Abs(bars[i].Low - bars[i-1].Close)
			tr = math.Max(hl, math.Max(hpc, lpc))
		}

		if i < period {
			trSum += tr
			if i == period-1 {
				prevATR = trSum / float64(period)
				res.Series[i].V = ptrFloat(prevATR)
			}
		} else {
			currentATR := (prevATR*float64(period-1) + tr) / float64(period)
			res.Series[i].V = ptrFloat(currentATR)
			prevATR = currentATR
		}
	}
	return res, nil
}

// Stochastic computes the Stochastic Oscillator (%K and %D).
func Stochastic(bars []BarInput, kPeriod, dPeriod int) (StochResult, error) {
	if kPeriod <= 0 || dPeriod <= 0 {
		return StochResult{}, fmt.Errorf("Stochastic periods must be positive, got %d, %d", kPeriod, dPeriod)
	}
	n := len(bars)
	warmup := kPeriod + dPeriod - 2
	res := StochResult{
		Indicator:  "stoch",
		Params:     map[string]any{"k_period": kPeriod, "d_period": dPeriod},
		WarmupBars: warmup,
		Series:     make([]StochPoint, n),
	}
	if n == 0 {
		return res, nil
	}

	kValues := make([]*float64, n)
	for i := 0; i < n; i++ {
		res.Series[i].T = bars[i].Time
		if i >= kPeriod-1 {
			lowMin := bars[i].Low
			highMax := bars[i].High
			for j := i - kPeriod + 1; j <= i; j++ {
				if bars[j].Low < lowMin {
					lowMin = bars[j].Low
				}
				if bars[j].High > highMax {
					highMax = bars[j].High
				}
			}
			var kVal float64
			if highMax == lowMin {
				kVal = 50.0
			} else {
				kVal = ((bars[i].Close - lowMin) / (highMax - lowMin)) * 100.0
			}
			kValues[i] = &kVal
			res.Series[i].K = ptrFloat(kVal)
		}
	}

	// Calculate %D: SMA of %K
	for i := 0; i < n; i++ {
		if i >= warmup {
			var dSum float64
			valid := true
			for j := i - dPeriod + 1; j <= i; j++ {
				if kValues[j] == nil {
					valid = false
					break
				}
				dSum += *kValues[j]
			}
			if valid {
				dVal := dSum / float64(dPeriod)
				res.Series[i].D = ptrFloat(dVal)
			}
		}
	}
	return res, nil
}

// VWAP computes Volume Weighted Average Price cumulative over the session or window.
// Typical price = (High + Low + Close) / 3.
func VWAP(bars []BarInput) (Result, error) {
	n := len(bars)
	res := Result{
		Indicator:  "vwap",
		Params:     map[string]any{},
		WarmupBars: 0,
		Series:     make([]Point, n),
	}
	if n == 0 {
		return res, nil
	}

	var cumVol, cumPV float64
	for i := 0; i < n; i++ {
		res.Series[i].T = bars[i].Time
		tp := (bars[i].High + bars[i].Low + bars[i].Close) / 3.0
		vol := bars[i].Volume
		if vol <= 0 {
			vol = 1.0 // fallback when volume is 0 or unrecorded
		}
		cumPV += tp * vol
		cumVol += vol
		if cumVol > 0 {
			res.Series[i].V = ptrFloat(cumPV / cumVol)
		}
	}
	return res, nil
}

// ParseIndicatorSpec parses an indicator query string specification like "sma:20" or "macd:12:26:9".
func ParseIndicatorSpec(spec string) (string, []float64, error) {
	parts := strings.Split(strings.TrimSpace(spec), ":")
	if len(parts) == 0 || parts[0] == "" {
		return "", nil, fmt.Errorf("empty indicator spec")
	}
	name := strings.ToLower(parts[0])
	var params []float64
	for _, p := range parts[1:] {
		val, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return "", nil, fmt.Errorf("invalid parameter %q for indicator %s", p, name)
		}
		params = append(params, val)
	}
	return name, params, nil
}
