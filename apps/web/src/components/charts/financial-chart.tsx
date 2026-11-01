"use client";

import { useEffect, useRef, useState, useMemo } from "react";
import type { OHLCVBar, IndicatorResult, ChartLayout } from "@/lib/schemas";
import { num, signed, pct } from "@/lib/utils/format";

export type ChartType = "candlestick" | "bar" | "line" | "area";

interface FinancialChartProps {
  symbol: string;
  initialBars: OHLCVBar[];
  onIntervalChange?: (interval: string) => void;
  onLayoutChange?: (layout: ChartLayout) => void;
}

export function FinancialChart({ symbol, initialBars }: FinancialChartProps) {
  const [bars, setBars] = useState<OHLCVBar[]>(initialBars);
  const [interval, setInterval] = useState<string>("1d");
  const [chartType, setChartType] = useState<ChartType>("candlestick");
  const [activeIndicators, setActiveIndicators] = useState<{
    sma: boolean;
    ema: boolean;
    bb: boolean;
    vwap: boolean;
    rsi: boolean;
    macd: boolean;
  }>({
    sma: true,
    ema: false,
    bb: false,
    vwap: false,
    rsi: true,
    macd: false,
  });

  const [indicatorData, setIndicatorData] = useState<IndicatorResult[]>([]);
  const [isMinuteDataUnavailable, setIsMinuteDataUnavailable] = useState(false);
  const [loading, setLoading] = useState(false);
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);

  // Layouts state
  const [layouts, setLayouts] = useState<ChartLayout[]>([]);
  const [selectedLayoutName, setSelectedLayoutName] = useState<string>("Default");
  const [saveName, setSaveName] = useState("");
  const [showSaveModal, setShowSaveModal] = useState(false);

  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);

  // Fetch OHLCV when interval changes
  useEffect(() => {
    let active = true;
    const fetchBars = async () => {
      setLoading(true);
      setIsMinuteDataUnavailable(false);
      try {
        const res = await fetch(`/api/v1/charts/ohlcv?symbol=${encodeURIComponent(symbol)}&interval=${interval}`);
        if (!res.ok) {
          const body = await res.json().catch(() => null);
          if (body?.error?.code === "DATA_UNAVAILABLE" || res.status === 404) {
            if (active) {
              setIsMinuteDataUnavailable(true);
              setBars([]);
            }
            return;
          }
          throw new Error("Failed to load bars");
        }
        const json = await res.json();
        if (active && json.data) {
          setBars(json.data);
          setIsMinuteDataUnavailable(false);
        }
      } catch {
        if (active) {
          // If network fetch fails on relative path, fallback to initialBars for 1d
          if (interval === "1d") {
            setBars(initialBars);
          } else if (["1m", "5m", "15m", "30m", "1h"].includes(interval)) {
            setIsMinuteDataUnavailable(true);
          }
        }
      } finally {
        if (active) setLoading(false);
      }
    };

    fetchBars();
    return () => {
      active = false;
    };
  }, [symbol, interval, initialBars]);

  // Fetch Indicators when bars or active indicators change
  useEffect(() => {
    if (bars.length === 0 || isMinuteDataUnavailable) {
      setIndicatorData([]);
      return;
    }

    const requestedSet: string[] = [];
    if (activeIndicators.sma) requestedSet.push("sma:20");
    if (activeIndicators.ema) requestedSet.push("ema:20");
    if (activeIndicators.bb) requestedSet.push("bb:20:2");
    if (activeIndicators.vwap) requestedSet.push("vwap");
    if (activeIndicators.rsi) requestedSet.push("rsi:14");
    if (activeIndicators.macd) requestedSet.push("macd:12:26:9");

    if (requestedSet.length === 0) {
      setIndicatorData([]);
      return;
    }

    let active = true;
    const fetchIndicators = async () => {
      try {
        const setQuery = requestedSet.join(",");
        const res = await fetch(
          `/api/v1/charts/indicators?symbol=${encodeURIComponent(symbol)}&interval=${interval}&set=${encodeURIComponent(setQuery)}`
        );
        if (res.ok) {
          const json = await res.json();
          if (active && json.data) {
            setIndicatorData(json.data);
          }
        }
      } catch {
        // Handled silently
      }
    };

    fetchIndicators();
    return () => {
      active = false;
    };
  }, [symbol, interval, bars, activeIndicators, isMinuteDataUnavailable]);

  // Load saved layouts
  useEffect(() => {
    const fetchLayouts = async () => {
      try {
        const res = await fetch(`${API_BASE}/api/v1/charts/layouts`);
        if (res.ok) {
          const json = await res.json();
          if (json.data) setLayouts(json.data);
        }
      } catch {
        // Handled
      }
    };
    fetchLayouts();
  }, []);

  // Save layout
  const handleSaveLayout = async () => {
    if (!saveName.trim()) return;
    try {
      const payload = {
        name: saveName.trim(),
        symbol,
        interval,
        chart_type: chartType,
        indicators: activeIndicators,
        settings: {},
      };
      const res = await fetch(`${API_BASE}/api/v1/charts/layouts`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (res.ok) {
        const json = await res.json();
        if (json.data) {
          setLayouts((prev) => [json.data, ...prev]);
          setSelectedLayoutName(json.data.name);
          setShowSaveModal(false);
          setSaveName("");
        }
      }
    } catch {
      // Handled
    }
  };

  // Apply layout
  const handleApplyLayout = (layout: ChartLayout) => {
    setSelectedLayoutName(layout.name);
    if (layout.interval) setInterval(layout.interval);
    if (layout.chart_type) setChartType(layout.chart_type as ChartType);
    if (layout.indicators && typeof layout.indicators === "object") {
      setActiveIndicators((prev) => ({ ...prev, ...layout.indicators }));
    }
  };

  // Canvas drawing logic
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    // Handle high DPI display
    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();
    const width = rect.width;
    const height = rect.height;

    canvas.width = width * dpr;
    canvas.height = height * dpr;
    ctx.scale(dpr, dpr);

    // Clear background
    ctx.fillStyle = "#090d16";
    ctx.fillRect(0, 0, width, height);

    if (bars.length === 0) {
      ctx.fillStyle = "#64748b";
      ctx.font = "12px sans-serif";
      ctx.textAlign = "center";
      ctx.fillText(
        isMinuteDataUnavailable
          ? "Minute data unavailable: The exchange only supplies daily historical closes."
          : "No price bars stored for this interval",
        width / 2,
        height / 2
      );
      return;
    }

    // Layout dimensions
    // Main price pane: 65% height
    // Volume pane: 15% height
    // RSI or MACD pane: 20% height (if active)
    const hasSubPane = activeIndicators.rsi || activeIndicators.macd;
    const pricePaneHeight = hasSubPane ? height * 0.58 : height * 0.75;
    const volPaneY = pricePaneHeight;
    const volPaneHeight = height * 0.16;
    const subPaneY = volPaneY + volPaneHeight;
    const subPaneHeight = hasSubPane ? height - subPaneY - 20 : 0;
    const timeAxisY = height - 20;

    const rightMargin = 65; // Price scale
    const chartWidth = width - rightMargin;

    // Calculate Price Bounds
    const highVals = bars.map((b) => Number(b.high));
    const lowVals = bars.map((b) => Number(b.low));
    let minPrice = Math.min(...lowVals);
    let maxPrice = Math.max(...highVals);
    if (minPrice === maxPrice) {
      minPrice *= 0.95;
      maxPrice *= 1.05;
    }
    const pricePadding = (maxPrice - minPrice) * 0.08;
    minPrice -= pricePadding;
    maxPrice += pricePadding;

    const maxVolume = Math.max(...bars.map((b) => Number(b.volume)), 1);

    const n = bars.length;
    const barWidth = Math.max(chartWidth / n, 2);
    const candleWidth = Math.max(barWidth * 0.65, 1);

    const priceToY = (p: number) => {
      return pricePaneHeight - ((p - minPrice) / (maxPrice - minPrice)) * (pricePaneHeight - 20) - 10;
    };

    const volToY = (v: number) => {
      return volPaneY + volPaneHeight - (v / maxVolume) * (volPaneHeight - 10) - 5;
    };

    // Draw Grid Lines
    ctx.strokeStyle = "rgba(255, 255, 255, 0.05)";
    ctx.lineWidth = 1;
    const priceSteps = 5;
    for (let i = 0; i <= priceSteps; i++) {
      const p = minPrice + ((maxPrice - minPrice) / priceSteps) * i;
      const y = priceToY(p);
      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(chartWidth, y);
      ctx.stroke();

      // Right axis price labels
      ctx.fillStyle = "#64748b";
      ctx.font = "10px monospace";
      ctx.textAlign = "left";
      ctx.fillText(p.toFixed(1), chartWidth + 6, y + 3);
    }

    // Draw Volume pane border
    ctx.beginPath();
    ctx.moveTo(0, volPaneY);
    ctx.lineTo(chartWidth, volPaneY);
    ctx.stroke();

    // Draw Volume Bars
    bars.forEach((b, i) => {
      const x = i * barWidth + barWidth / 2;
      const v = Number(b.volume);
      const y = volToY(v);
      const isUp = Number(b.close) >= Number(b.open);

      ctx.fillStyle = isUp ? "rgba(34, 197, 94, 0.3)" : "rgba(239, 68, 68, 0.3)";
      ctx.fillRect(x - candleWidth / 2, y, candleWidth, volPaneY + volPaneHeight - y);
    });

    // Draw Main Price Chart based on ChartType
    bars.forEach((b, i) => {
      const x = i * barWidth + barWidth / 2;
      const o = Number(b.open);
      const h = Number(b.high);
      const l = Number(b.low);
      const c = Number(b.close);
      const isUp = c >= o;
      const upColor = "#22c55e";
      const downColor = "#ef4444";
      const color = isUp ? upColor : downColor;

      if (chartType === "candlestick") {
        const yOpen = priceToY(o);
        const yClose = priceToY(c);
        const yHigh = priceToY(h);
        const yLow = priceToY(l);

        // Wick
        ctx.strokeStyle = color;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(x, yHigh);
        ctx.lineTo(x, yLow);
        ctx.stroke();

        // Candle Body
        const top = Math.min(yOpen, yClose);
        const bodyHeight = Math.max(Math.abs(yClose - yOpen), 1);
        ctx.fillStyle = color;
        ctx.fillRect(x - candleWidth / 2, top, candleWidth, bodyHeight);
      } else if (chartType === "bar") {
        const yOpen = priceToY(o);
        const yClose = priceToY(c);
        const yHigh = priceToY(h);
        const yLow = priceToY(l);

        ctx.strokeStyle = color;
        ctx.lineWidth = 1.5;
        // Central high-low bar
        ctx.beginPath();
        ctx.moveTo(x, yHigh);
        ctx.lineTo(x, yLow);
        ctx.stroke();

        // Left open peg
        ctx.beginPath();
        ctx.moveTo(x - candleWidth / 2, yOpen);
        ctx.lineTo(x, yOpen);
        ctx.stroke();

        // Right close peg
        ctx.beginPath();
        ctx.moveTo(x, yClose);
        ctx.lineTo(x + candleWidth / 2, yClose);
        ctx.stroke();
      }
    });

    // Line / Area chart rendering
    if (chartType === "line" || chartType === "area") {
      ctx.beginPath();
      bars.forEach((b, i) => {
        const x = i * barWidth + barWidth / 2;
        const y = priceToY(Number(b.close));
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      });
      ctx.strokeStyle = "#38bdf8";
      ctx.lineWidth = 2;
      ctx.stroke();

      if (chartType === "area") {
        const lastX = (bars.length - 1) * barWidth + barWidth / 2;
        const firstX = barWidth / 2;
        ctx.lineTo(lastX, pricePaneHeight);
        ctx.lineTo(firstX, pricePaneHeight);
        ctx.closePath();

        const grad = ctx.createLinearGradient(0, 0, 0, pricePaneHeight);
        grad.addColorStop(0, "rgba(56, 189, 248, 0.35)");
        grad.addColorStop(1, "rgba(56, 189, 248, 0.0)");
        ctx.fillStyle = grad;
        ctx.fill();
      }
    }

    // Draw Overlaid Indicators (SMA, EMA, Bollinger Bands, VWAP)
    indicatorData.forEach((ind) => {
      if (ind.indicator === "sma" && activeIndicators.sma) {
        ctx.strokeStyle = "#f59e0b"; // Amber
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        let started = false;
        ind.series.forEach((pt, i) => {
          if (pt.v === null || pt.v === undefined) return;
          const x = i * barWidth + barWidth / 2;
          const y = priceToY(pt.v);
          if (!started) {
            ctx.moveTo(x, y);
            started = true;
          } else {
            ctx.lineTo(x, y);
          }
        });
        ctx.stroke();
      }

      if (ind.indicator === "ema" && activeIndicators.ema) {
        ctx.strokeStyle = "#a855f7"; // Purple
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        let started = false;
        ind.series.forEach((pt, i) => {
          if (pt.v === null || pt.v === undefined) return;
          const x = i * barWidth + barWidth / 2;
          const y = priceToY(pt.v);
          if (!started) {
            ctx.moveTo(x, y);
            started = true;
          } else {
            ctx.lineTo(x, y);
          }
        });
        ctx.stroke();
      }

      if (ind.indicator === "vwap" && activeIndicators.vwap) {
        ctx.strokeStyle = "#06b6d4"; // Cyan
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        let started = false;
        ind.series.forEach((pt, i) => {
          if (pt.v === null || pt.v === undefined) return;
          const x = i * barWidth + barWidth / 2;
          const y = priceToY(pt.v);
          if (!started) {
            ctx.moveTo(x, y);
            started = true;
          } else {
            ctx.lineTo(x, y);
          }
        });
        ctx.stroke();
      }

      if (ind.indicator === "bb" && activeIndicators.bb) {
        // Upper
        ctx.strokeStyle = "rgba(147, 197, 253, 0.7)";
        ctx.lineWidth = 1;
        ctx.setLineDash([3, 3]);
        ctx.beginPath();
        let started = false;
        ind.series.forEach((pt, i) => {
          if (pt.upper === null || pt.upper === undefined) return;
          const x = i * barWidth + barWidth / 2;
          const y = priceToY(pt.upper);
          if (!started) {
            ctx.moveTo(x, y);
            started = true;
          } else {
            ctx.lineTo(x, y);
          }
        });
        ctx.stroke();

        // Lower
        ctx.beginPath();
        started = false;
        ind.series.forEach((pt, i) => {
          if (pt.lower === null || pt.lower === undefined) return;
          const x = i * barWidth + barWidth / 2;
          const y = priceToY(pt.lower);
          if (!started) {
            ctx.moveTo(x, y);
            started = true;
          } else {
            ctx.lineTo(x, y);
          }
        });
        ctx.stroke();
        ctx.setLineDash([]);
      }
    });

    // Draw Sub-Pane (RSI or MACD)
    if (hasSubPane) {
      ctx.beginPath();
      ctx.moveTo(0, subPaneY);
      ctx.lineTo(chartWidth, subPaneY);
      ctx.strokeStyle = "rgba(255, 255, 255, 0.08)";
      ctx.stroke();

      if (activeIndicators.rsi) {
        const rsiInd = indicatorData.find((d) => d.indicator === "rsi");
        const rsiToY = (val: number) => {
          return subPaneY + subPaneHeight - (val / 100) * (subPaneHeight - 10) - 5;
        };

        // Draw 70/30 dashed levels
        ctx.strokeStyle = "rgba(239, 68, 68, 0.4)";
        ctx.setLineDash([3, 3]);
        ctx.beginPath();
        ctx.moveTo(0, rsiToY(70));
        ctx.lineTo(chartWidth, rsiToY(70));
        ctx.stroke();

        ctx.strokeStyle = "rgba(34, 197, 94, 0.4)";
        ctx.beginPath();
        ctx.moveTo(0, rsiToY(30));
        ctx.lineTo(chartWidth, rsiToY(30));
        ctx.stroke();
        ctx.setLineDash([]);

        // Label
        ctx.fillStyle = "#64748b";
        ctx.font = "9px monospace";
        ctx.fillText("RSI(14)", 6, subPaneY + 12);
        ctx.fillText("70", chartWidth + 6, rsiToY(70) + 3);
        ctx.fillText("30", chartWidth + 6, rsiToY(30) + 3);

        if (rsiInd) {
          ctx.strokeStyle = "#818cf8";
          ctx.lineWidth = 1.5;
          ctx.beginPath();
          let started = false;
          rsiInd.series.forEach((pt, i) => {
            if (pt.v === null || pt.v === undefined) return;
            const x = i * barWidth + barWidth / 2;
            const y = rsiToY(pt.v);
            if (!started) {
              ctx.moveTo(x, y);
              started = true;
            } else {
              ctx.lineTo(x, y);
            }
          });
          ctx.stroke();
        }
      } else if (activeIndicators.macd) {
        const macdInd = indicatorData.find((d) => d.indicator === "macd");
        ctx.fillStyle = "#64748b";
        ctx.font = "9px monospace";
        ctx.fillText("MACD(12,26,9)", 6, subPaneY + 12);

        if (macdInd) {
          // Find max abs value for scaling
          let maxMACD = 1;
          macdInd.series.forEach((pt) => {
            if (pt.macd) maxMACD = Math.max(maxMACD, Math.abs(pt.macd));
            if (pt.signal) maxMACD = Math.max(maxMACD, Math.abs(pt.signal));
            if (pt.hist) maxMACD = Math.max(maxMACD, Math.abs(pt.hist));
          });
          const macdZeroY = subPaneY + subPaneHeight / 2;
          const macdScaleY = (v: number) => {
            return macdZeroY - (v / maxMACD) * (subPaneHeight / 2 - 8);
          };

          // Zero line
          ctx.strokeStyle = "rgba(255, 255, 255, 0.1)";
          ctx.beginPath();
          ctx.moveTo(0, macdZeroY);
          ctx.lineTo(chartWidth, macdZeroY);
          ctx.stroke();

          // Histogram bars
          macdInd.series.forEach((pt, i) => {
            if (pt.hist === null || pt.hist === undefined) return;
            const x = i * barWidth + barWidth / 2;
            const y = macdScaleY(pt.hist);
            const isPos = pt.hist >= 0;
            ctx.fillStyle = isPos ? "rgba(34, 197, 94, 0.6)" : "rgba(239, 68, 68, 0.6)";
            ctx.fillRect(x - candleWidth / 2, Math.min(y, macdZeroY), candleWidth, Math.abs(y - macdZeroY));
          });

          // MACD Line
          ctx.strokeStyle = "#38bdf8";
          ctx.lineWidth = 1.5;
          ctx.beginPath();
          let started = false;
          macdInd.series.forEach((pt, i) => {
            if (pt.macd === null || pt.macd === undefined) return;
            const x = i * barWidth + barWidth / 2;
            const y = macdScaleY(pt.macd);
            if (!started) {
              ctx.moveTo(x, y);
              started = true;
            } else {
              ctx.lineTo(x, y);
            }
          });
          ctx.stroke();

          // Signal Line
          ctx.strokeStyle = "#f97316";
          ctx.lineWidth = 1.2;
          ctx.beginPath();
          started = false;
          macdInd.series.forEach((pt, i) => {
            if (pt.signal === null || pt.signal === undefined) return;
            const x = i * barWidth + barWidth / 2;
            const y = macdScaleY(pt.signal);
            if (!started) {
              ctx.moveTo(x, y);
              started = true;
            } else {
              ctx.lineTo(x, y);
            }
          });
          ctx.stroke();
        }
      }
    }

    // Time Axis labels
    const labelStep = Math.max(Math.floor(n / 6), 1);
    ctx.fillStyle = "#64748b";
    ctx.font = "10px monospace";
    ctx.textAlign = "center";
    for (let i = 0; i < n; i += labelStep) {
      const b = bars[i];
      if (!b) continue;
      const x = i * barWidth + barWidth / 2;
      const tLabel = b.time.length > 10 ? b.time.slice(11, 16) : b.time;
      ctx.fillText(tLabel, x, timeAxisY + 12);
    }

    // Draw Crosshair on mouse hover
    if (hoverIndex !== null && hoverIndex >= 0 && hoverIndex < bars.length) {
      const hb = bars[hoverIndex];
      if (!hb) return;
      const hx = hoverIndex * barWidth + barWidth / 2;
      const hy = priceToY(Number(hb.close));

      ctx.strokeStyle = "rgba(255, 255, 255, 0.35)";
      ctx.lineWidth = 1;
      ctx.setLineDash([4, 4]);

      // Vertical line
      ctx.beginPath();
      ctx.moveTo(hx, 0);
      ctx.lineTo(hx, timeAxisY);
      ctx.stroke();

      // Horizontal price line
      ctx.beginPath();
      ctx.moveTo(0, hy);
      ctx.lineTo(chartWidth, hy);
      ctx.stroke();
      ctx.setLineDash([]);

      // Price tag on right axis
      ctx.fillStyle = "#1e293b";
      ctx.fillRect(chartWidth + 2, hy - 9, 56, 18);
      ctx.strokeStyle = "#475569";
      ctx.strokeRect(chartWidth + 2, hy - 9, 56, 18);
      ctx.fillStyle = "#f8fafc";
      ctx.font = "10px monospace";
      ctx.textAlign = "left";
      ctx.fillText(Number(hb.close).toFixed(1), chartWidth + 6, hy + 4);
    }
  }, [bars, chartType, activeIndicators, indicatorData, isMinuteDataUnavailable, hoverIndex]);

  // Hover detection
  const handleMouseMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current;
    if (!canvas || bars.length === 0) return;
    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const rightMargin = 65;
    const chartWidth = rect.width - rightMargin;
    if (x < 0 || x > chartWidth) {
      setHoverIndex(null);
      return;
    }
    const idx = Math.floor((x / chartWidth) * bars.length);
    if (idx >= 0 && idx < bars.length) {
      setHoverIndex(idx);
    } else {
      setHoverIndex(null);
    }
  };

  const currentBar = hoverIndex !== null && bars[hoverIndex] ? bars[hoverIndex] : bars[bars.length - 1];

  const priceChange = useMemo(() => {
    if (!currentBar) return { change: 0, pct: 0 };
    const prev = Number(currentBar.previous_close || currentBar.open);
    const curr = Number(currentBar.close);
    const chg = curr - prev;
    const p = prev > 0 ? (chg / prev) * 100 : 0;
    return { change: chg, pct: p };
  }, [currentBar]);

  return (
    <div className="space-y-3" ref={containerRef}>
      {/* Top Toolbar: Interval, Chart Type, Indicator Toggles, Layout Manager */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-surface-line/70 pb-2 text-xs">
        <div className="flex flex-wrap items-center gap-1.5">
          {/* Timeframe / Interval selector */}
          <div className="flex rounded border border-surface-line bg-surface-raised p-0.5">
            {[
              { id: "1d", label: "Daily" },
              { id: "1w", label: "Weekly" },
              { id: "1m", label: "Monthly" },
              { id: "1min", label: "1m" },
              { id: "5m", label: "5m" },
              { id: "15m", label: "15m" },
              { id: "1h", label: "1h" },
            ].map((it) => (
              <button
                key={it.id}
                onClick={() => setInterval(it.id === "1min" ? "1m" : it.id)}
                className={`rounded px-2 py-1 text-2xs font-medium transition-colors ${
                  interval === (it.id === "1min" ? "1m" : it.id)
                    ? "bg-accent/20 text-accent font-semibold"
                    : "text-surface-muted hover:text-slate-200"
                }`}
              >
                {it.label}
              </button>
            ))}
          </div>

          {/* Chart Type selector */}
          <div className="flex rounded border border-surface-line bg-surface-raised p-0.5">
            {(["candlestick", "bar", "line", "area"] as ChartType[]).map((t) => (
              <button
                key={t}
                onClick={() => setChartType(t)}
                className={`rounded px-2 py-1 text-2xs uppercase tracking-wider transition-colors ${
                  chartType === t ? "bg-white/[0.1] text-slate-100 font-semibold" : "text-surface-muted hover:text-slate-200"
                }`}
              >
                {t}
              </button>
            ))}
          </div>

          {/* Verification Badge */}
          <span
            className={`inline-flex items-center gap-1 rounded border px-2 py-0.5 text-2xs font-mono uppercase tracking-wider ${
              ["1m", "5m", "15m", "30m", "1h"].includes(interval)
                ? isMinuteDataUnavailable
                  ? "border-amber-500/40 text-amber-400"
                  : "border-sky-500/40 text-sky-400"
                : "border-emerald-500/40 text-emerald-400"
            }`}
          >
            {["1m", "5m", "15m", "30m", "1h"].includes(interval)
              ? isMinuteDataUnavailable
                ? "⚠ Minute History Unavailable"
                : "Intraday Session Ticks"
              : "Verified Historical OHLCV"}
          </span>
        </div>

        {/* Indicators & Layouts selector */}
        <div className="flex flex-wrap items-center gap-2">
          {/* Indicator toggles */}
          <div className="flex items-center gap-1">
            <span className="text-2xs uppercase text-surface-muted">Indicators:</span>
            {(["sma", "ema", "bb", "vwap", "rsi", "macd"] as const).map((ind) => (
              <button
                key={ind}
                onClick={() => setActiveIndicators((prev) => ({ ...prev, [ind]: !prev[ind] }))}
                className={`rounded border px-1.5 py-0.5 text-2xs font-mono uppercase transition-colors ${
                  activeIndicators[ind]
                    ? "border-accent/60 bg-accent/10 text-accent font-semibold"
                    : "border-surface-line/80 text-surface-muted hover:text-slate-300"
                }`}
              >
                {ind}
              </button>
            ))}
          </div>

          {/* Layouts Dropdown */}
          <div className="relative flex items-center gap-1">
            <select
              value={selectedLayoutName}
              onChange={(e) => {
                const found = layouts.find((l) => l.name === e.target.value);
                if (found) handleApplyLayout(found);
                else setSelectedLayoutName(e.target.value);
              }}
              className="rounded border border-surface-line bg-surface-raised px-2 py-1 text-2xs text-slate-200 focus:border-accent focus:outline-none"
            >
              <option value="Default">Layout: Default</option>
              {layouts.map((l) => (
                <option key={l.id} value={l.name}>
                  Layout: {l.name}
                </option>
              ))}
            </select>
            <button
              onClick={() => setShowSaveModal(true)}
              className="rounded border border-surface-line px-2 py-1 text-2xs text-surface-muted hover:border-accent hover:text-accent"
              title="Save current indicators and layout settings"
            >
              Save
            </button>
          </div>
        </div>
      </div>

      {/* Save Layout Modal */}
      {showSaveModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="w-full max-w-sm rounded-lg border border-surface-line bg-surface-raised p-4 shadow-xl">
            <h3 className="text-sm font-semibold text-slate-100">Save Chart Layout</h3>
            <p className="mt-1 text-2xs text-surface-muted">
              Save your timeframes, active indicators ({Object.keys(activeIndicators).filter((k) => (activeIndicators as any)[k]).join(", ")}) and chart settings.
            </p>
            <input
              type="text"
              placeholder="Layout Name (e.g. Trend Momentum)"
              value={saveName}
              onChange={(e) => setSaveName(e.target.value)}
              className="mt-3 w-full rounded border border-surface-line bg-surface-base px-3 py-1.5 text-xs text-slate-100 focus:border-accent focus:outline-none"
            />
            <div className="mt-4 flex justify-end gap-2">
              <button
                onClick={() => setShowSaveModal(false)}
                className="rounded px-3 py-1.5 text-xs text-surface-muted hover:text-slate-100"
              >
                Cancel
              </button>
              <button
                onClick={handleSaveLayout}
                className="rounded bg-accent px-3 py-1.5 text-xs font-medium text-slate-950 hover:bg-accent/90"
              >
                Save Layout
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Live Crosshair / Bar Readout Header */}
      {currentBar && (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-2xs font-mono text-surface-muted">
          <span>
            Date: <span className="text-slate-200">{currentBar.time}</span>
          </span>
          <span>
            Open: <span className="text-slate-200">{num(currentBar.open)}</span>
          </span>
          <span>
            High: <span className="text-slate-200">{num(currentBar.high)}</span>
          </span>
          <span>
            Low: <span className="text-slate-200">{num(currentBar.low)}</span>
          </span>
          <span>
            Close:{" "}
            <span className={priceChange.change >= 0 ? "text-up font-semibold" : "text-down font-semibold"}>
              {num(currentBar.close)}
            </span>
          </span>
          <span>
            Change:{" "}
            <span className={priceChange.change >= 0 ? "text-up" : "text-down"}>
              {signed(priceChange.change)} ({pct(priceChange.pct)})
            </span>
          </span>
          <span>
            Volume: <span className="text-slate-200">{num(currentBar.volume, "0")}</span>
          </span>
        </div>
      )}

      {/* Honest Warning when Minute Data is Unavailable */}
      {isMinuteDataUnavailable && (
        <div className="rounded border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-300">
          <div className="font-semibold">Intraday minute data is unavailable for historical ranges</div>
          <div className="mt-1 text-2xs text-amber-200/80 leading-relaxed">
            The NEPSE exchange only publishes daily historical closes and current session intraday snapshots.
            Minute bars (1m–1h) require live trade execution feeds and are not backfilled synthetically.
            Switch back to <span className="font-semibold text-accent underline cursor-pointer" onClick={() => setInterval("1d")}>Daily (1d)</span> or <span className="font-semibold text-accent underline cursor-pointer" onClick={() => setInterval("1w")}>Weekly (1w)</span> for verified historical data.
          </div>
        </div>
      )}

      {/* Canvas Chart Area */}
      <div className="relative w-full h-[460px] rounded border border-surface-line/70 bg-[#090d16] overflow-hidden">
        {loading && (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-black/40 text-xs text-accent">
            Loading chart data...
          </div>
        )}
        <canvas
          ref={canvasRef}
          onMouseMove={handleMouseMove}
          onMouseLeave={() => setHoverIndex(null)}
          className="w-full h-full cursor-crosshair block"
        />
      </div>
    </div>
  );
}
