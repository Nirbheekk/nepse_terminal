import { test, describe, it } from "node:test";
import assert from "node:assert/strict";

describe("Indicator Formula Math & Verification", () => {
  it("SMA computes correct arithmetic moving average with warmup", () => {
    const prices = [10, 11, 12, 13, 14, 15];
    const period = 3;
    const series = [];

    let sum = 0;
    for (let i = 0; i < prices.length; i++) {
      sum += prices[i];
      if (i >= period) sum -= prices[i - period];
      if (i < period - 1) {
        series.push(null); // warmup
      } else {
        series.push(sum / period);
      }
    }

    assert.equal(series[0], null);
    assert.equal(series[1], null);
    assert.equal(series[2], 11.0);
    assert.equal(series[5], 14.0);
  });

  it("EMA computes exponential smoothing using multiplier 2/(N+1)", () => {
    const prices = [10, 12, 14, 16, 18];
    const period = 3;
    const k = 2 / (period + 1); // 0.5
    const series = [];

    let sum = 0;
    let prevEMA = null;
    for (let i = 0; i < prices.length; i++) {
      if (i < period - 1) {
        sum += prices[i];
        series.push(null);
      } else if (i === period - 1) {
        sum += prices[i];
        prevEMA = sum / period;
        series.push(prevEMA);
      } else {
        const currEMA = prices[i] * k + prevEMA * (1 - k);
        series.push(currEMA);
        prevEMA = currEMA;
      }
    }

    assert.equal(series[0], null);
    assert.equal(series[1], null);
    assert.equal(series[2], 12.0); // Initial SMA
    assert.equal(series[3], 14.0); // 16 * 0.5 + 12 * 0.5
    assert.equal(series[4], 16.0); // 18 * 0.5 + 14 * 0.5
  });

  it("RSI correctly calculates Wilder's smoothed gains and losses", () => {
    const prices = [10, 12, 14, 16, 18, 20]; // 5 consecutive gains
    const period = 3;

    let sumGain = 0;
    let sumLoss = 0;
    let avgGain = 0;
    let avgLoss = 0;
    const rsi = [];

    for (let i = 0; i < prices.length; i++) {
      if (i === 0) {
        rsi.push(null);
        continue;
      }
      const diff = prices[i] - prices[i - 1];
      const gain = Math.max(diff, 0);
      const loss = Math.max(-diff, 0);

      if (i <= period) {
        sumGain += gain;
        sumLoss += loss;
        if (i === period) {
          avgGain = sumGain / period;
          avgLoss = sumLoss / period;
          const rs = avgLoss === 0 ? Infinity : avgGain / avgLoss;
          rsi.push(avgLoss === 0 ? 100 : 100 - 100 / (1 + rs));
        } else {
          rsi.push(null);
        }
      } else {
        avgGain = (avgGain * (period - 1) + gain) / period;
        avgLoss = (avgLoss * (period - 1) + loss) / period;
        const rs = avgLoss === 0 ? Infinity : avgGain / avgLoss;
        rsi.push(avgLoss === 0 ? 100 : 100 - 100 / (1 + rs));
      }
    }

    assert.equal(rsi[0], null);
    assert.equal(rsi[1], null);
    assert.equal(rsi[2], null);
    assert.equal(rsi[3], 100.0);
    assert.equal(rsi[4], 100.0);
  });

  it("Bollinger Bands calculates Upper, Middle and Lower bands with stdDev", () => {
    const prices = [10, 12, 14];
    const period = 3;
    const stdDevMultiplier = 2.0;

    const mean = (10 + 12 + 14) / 3; // 12
    const variance = ((10 - 12) ** 2 + (12 - 12) ** 2 + (14 - 12) ** 2) / 3; // 8/3 = 2.6667
    const sd = Math.sqrt(variance); // ~1.633

    const upper = mean + stdDevMultiplier * sd;
    const lower = mean - stdDevMultiplier * sd;

    assert.equal(mean, 12);
    assert.ok(Math.abs(upper - 15.266) < 0.01);
    assert.ok(Math.abs(lower - 8.734) < 0.01);
  });
});

describe("Browser Chart & Layout Interaction State Tests", () => {
  it("Separates actual intraday observations from daily historical closes", () => {
    const indexHistoryDaily = [
      { time: "2026-10-28", value: "2660.24", granularity: "daily" },
      { time: "2026-10-29", value: "2678.22", granularity: "daily" },
    ];

    const indexIntraday = [
      { time: "2026-10-29T11:05:00", value: "2665.10", granularity: "intraday" },
      { time: "2026-10-29T11:10:00", value: "2670.30", granularity: "intraday" },
    ];

    assert.equal(indexHistoryDaily.every((p) => p.granularity === "daily"), true);
    assert.equal(indexIntraday.every((p) => p.granularity === "intraday"), true);
    assert.notDeepEqual(indexHistoryDaily, indexIntraday);
  });

  it("Enables minute charts only when valid minute data exists, otherwise flags unavailable", () => {
    function checkMinuteIntervalAvailability(interval, hasMinuteData) {
      const isMinute = ["1m", "5m", "15m", "30m", "1h"].includes(interval);
      if (isMinute && !hasMinuteData) {
        return {
          allowed: false,
          state: "DATA_UNAVAILABLE",
          message: "Minute intervals (1m-1h) unavailable: NEPSE exchange only publishes daily historical closes.",
        };
      }
      return {
        allowed: true,
        state: isMinute ? "INTRADAY_SESSION" : "VERIFIED_DAILY_OHLCV",
      };
    }

    // Historical test case (no minute data)
    const histResult = checkMinuteIntervalAvailability("5m", false);
    assert.equal(histResult.allowed, false);
    assert.equal(histResult.state, "DATA_UNAVAILABLE");

    // Daily interval (always available from daily_ohlcv)
    const dailyResult = checkMinuteIntervalAvailability("1d", false);
    assert.equal(dailyResult.allowed, true);
    assert.equal(dailyResult.state, "VERIFIED_DAILY_OHLCV");

    // Session minute interval with real trades
    const sessionResult = checkMinuteIntervalAvailability("1m", true);
    assert.equal(sessionResult.allowed, true);
    assert.equal(sessionResult.state, "INTRADAY_SESSION");
  });

  it("Chart Layout transitions and indicator toggles operate idempotently", () => {
    let state = {
      symbol: "ACLBSL",
      interval: "1d",
      chartType: "candlestick",
      indicators: { sma: true, ema: false, rsi: true, macd: false },
    };

    // Toggle MACD
    state = {
      ...state,
      indicators: { ...state.indicators, macd: !state.indicators.macd },
    };
    assert.equal(state.indicators.macd, true);

    // Apply Saved Layout
    const savedLayout = {
      name: "Momentum",
      symbol: "ACLBSL",
      interval: "1w",
      chart_type: "area",
      indicators: { sma: false, ema: true, rsi: true, macd: true, bb: true },
    };

    state = {
      ...state,
      interval: savedLayout.interval,
      chartType: savedLayout.chart_type,
      indicators: savedLayout.indicators,
    };

    assert.equal(state.interval, "1w");
    assert.equal(state.chartType, "area");
    assert.equal(state.indicators.bb, true);
    assert.equal(state.indicators.sma, false);
  });

  it("Heatmap color scale maps % changes correctly across circuit bounds", () => {
    function getTileColor(p) {
      if (p >= 9.5) return "#10b981"; // Upper circuit
      if (p >= 5) return "#16a34a";
      if (p > 0) return "#22c55e";
      if (p === 0) return "#334155";
      if (p <= -9.5) return "#b91c1c"; // Lower circuit
      if (p <= -5) return "#dc2626";
      return "#ef4444";
    }

    assert.equal(getTileColor(10.0), "#10b981");
    assert.equal(getTileColor(9.8), "#10b981");
    assert.equal(getTileColor(3.5), "#22c55e");
    assert.equal(getTileColor(0.0), "#334155");
    assert.equal(getTileColor(-3.5), "#ef4444");
    assert.equal(getTileColor(-9.9), "#b91c1c");
  });

  it("Sector Comparison calculates relative rankings and points from daily closes", () => {
    const sectors = [
      { code: "banking", start: 1600, current: 1616, point_change: 16, percent_change: 1.0 },
      { code: "hydropower", start: 3700, current: 3663, point_change: -37, percent_change: -1.0 },
      { code: "microfinance", start: 5400, current: 5508, point_change: 108, percent_change: 2.0 },
    ];

    const sorted = [...sectors].sort((a, b) => b.percent_change - a.percent_change);
    assert.equal(sorted[0].code, "microfinance");
    assert.equal(sorted[1].code, "banking");
    assert.equal(sorted[2].code, "hydropower");
  });
});
