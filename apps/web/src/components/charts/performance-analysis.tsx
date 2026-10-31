"use client";

import { useEffect, useState } from "react";
import type { MarketBreadth, SectorComparisonItem } from "@/lib/schemas";
import { num, signed, pct } from "@/lib/utils/format";

interface PerformanceAnalysisProps {
  initialBreadth?: MarketBreadth | null;
  initialSectors?: SectorComparisonItem[];
}

export function PerformanceAnalysis({ initialBreadth, initialSectors = [] }: PerformanceAnalysisProps) {
  const [period, setPeriod] = useState<string>("1d");
  const [sectorComparison, setSectorComparison] = useState<SectorComparisonItem[]>(initialSectors);
  const [breadth, setBreadth] = useState<MarketBreadth | null>(initialBreadth || null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let active = true;
    const fetchComparison = async () => {
      setLoading(true);
      try {
        const res = await fetch(`/api/v1/sectors/comparison?period=${encodeURIComponent(period)}`);
        if (res.ok) {
          const json = await res.json();
          if (active && json.data) setSectorComparison(json.data);
        }
      } catch {
        // Handled
      } finally {
        if (active) setLoading(false);
      }
    };
    fetchComparison();
    return () => {
      active = false;
    };
  }, [period]);

  useEffect(() => {
    if (breadth) return;
    let active = true;
    const fetchBreadth = async () => {
      try {
        const res = await fetch("/api/v1/market/breadth");
        if (res.ok) {
          const json = await res.json();
          if (active && json.data) setBreadth(json.data);
        }
      } catch {
        // Handled
      }
    };
    fetchBreadth();
    return () => {
      active = false;
    };
  }, [breadth]);

  const distribution = breadth?.distribution || {
    lt_minus_5: 0,
    minus_5_to_2: 0,
    minus_2_to_0: 0,
    zero: 0,
    plus_0_to_2: 0,
    plus_2_to_5: 0,
    gt_plus_5: 0,
  };

  const distBuckets = [
    { label: "< -5%", count: distribution.lt_minus_5 || 0, color: "bg-red-700" },
    { label: "-5% to -2%", count: distribution.minus_5_to_2 || 0, color: "bg-red-500" },
    { label: "-2% to 0%", count: distribution.minus_2_to_0 || 0, color: "bg-red-400/80" },
    { label: "0% Flat", count: distribution.zero || 0, color: "bg-slate-500" },
    { label: "0% to +2%", count: distribution.plus_0_to_2 || 0, color: "bg-emerald-400/80" },
    { label: "+2% to +5%", count: distribution.plus_2_to_5 || 0, color: "bg-emerald-500" },
    { label: "> +5%", count: distribution.gt_plus_5 || 0, color: "bg-emerald-600" },
  ];

  const maxBucketCount = Math.max(...distBuckets.map((b) => b.count), 1);

  return (
    <div className="space-y-6">
      {/* Market Breadth & Return Distribution Cards */}
      <div className="grid gap-4 md:grid-cols-2">
        {/* Market Breadth Meter */}
        <div className="rounded border border-surface-line/70 bg-surface-raised p-4">
          <div className="flex items-center justify-between border-b border-surface-line/50 pb-2">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-200">
              Market Breadth
            </h3>
            {breadth && (
              <span className="text-2xs font-mono text-surface-muted">
                A/D Ratio: <strong className="text-slate-100">{breadth.advance_ratio.toFixed(2)}</strong>
              </span>
            )}
          </div>

          {breadth ? (
            <div className="mt-4 space-y-3">
              {/* Stacked Proportional Bar */}
              <div className="h-4 w-full flex rounded overflow-hidden bg-slate-800">
                <div
                  style={{ width: `${(breadth.advanced / Math.max(breadth.total, 1)) * 100}%` }}
                  className="bg-up transition-all"
                  title={`Advanced: ${breadth.advanced}`}
                />
                <div
                  style={{ width: `${(breadth.unchanged / Math.max(breadth.total, 1)) * 100}%` }}
                  className="bg-slate-600 transition-all"
                  title={`Unchanged: ${breadth.unchanged}`}
                />
                <div
                  style={{ width: `${(breadth.declined / Math.max(breadth.total, 1)) * 100}%` }}
                  className="bg-down transition-all"
                  title={`Declined: ${breadth.declined}`}
                />
              </div>

              <div className="grid grid-cols-3 text-center text-xs">
                <div>
                  <div className="text-2xs text-surface-muted">Advanced</div>
                  <div className="font-bold text-up">{breadth.advanced}</div>
                </div>
                <div>
                  <div className="text-2xs text-surface-muted">Unchanged</div>
                  <div className="font-bold text-slate-400">{breadth.unchanged}</div>
                </div>
                <div>
                  <div className="text-2xs text-surface-muted">Declined</div>
                  <div className="font-bold text-down">{breadth.declined}</div>
                </div>
              </div>
            </div>
          ) : (
            <div className="mt-6 text-center text-xs text-surface-muted">Loading breadth metrics...</div>
          )}
        </div>

        {/* Return Distribution Histogram */}
        <div className="rounded border border-surface-line/70 bg-surface-raised p-4">
          <div className="flex items-center justify-between border-b border-surface-line/50 pb-2">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-200">
              Return Distribution
            </h3>
            <span className="text-2xs text-surface-muted">Scrips by session gain/loss</span>
          </div>

          <div className="mt-3 flex items-end justify-between gap-1.5 h-28 pt-2">
            {distBuckets.map((b) => {
              const hPct = Math.max((b.count / maxBucketCount) * 100, 4);
              return (
                <div key={b.label} className="flex-1 flex flex-col items-center gap-1 group">
                  <span className="text-3xs font-mono text-surface-muted group-hover:text-slate-100">
                    {b.count}
                  </span>
                  <div className="w-full flex items-end justify-center h-20">
                    <div
                      style={{ height: `${hPct}%` }}
                      className={`w-full max-w-[24px] rounded-t ${b.color} transition-all group-hover:opacity-100 opacity-90`}
                    />
                  </div>
                  <span className="text-3xs text-surface-muted whitespace-nowrap overflow-hidden text-ellipsis max-w-[42px] text-center">
                    {b.label}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      </div>

      {/* Sector Comparison Table & Bars */}
      <div className="rounded border border-surface-line/70 bg-surface-raised p-4">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-surface-line/50 pb-3">
          <div>
            <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-200">
              Sector Performance Comparison
            </h3>
            <p className="text-2xs text-surface-muted mt-0.5">
              Ranked by percentage change using stored daily closes.
            </p>
          </div>

          {/* Period Tabs */}
          <div className="flex rounded border border-surface-line bg-surface-base p-0.5 text-xs">
            {["1d", "5d", "30d", "90d", "1y"].map((p) => (
              <button
                key={p}
                onClick={() => setPeriod(p)}
                className={`rounded px-2.5 py-1 text-2xs uppercase tracking-wider transition-colors ${
                  period === p ? "bg-accent/20 text-accent font-semibold" : "text-surface-muted hover:text-slate-200"
                }`}
              >
                {p}
              </button>
            ))}
          </div>
        </div>

        {/* Sector Bars */}
        <div className="mt-4 space-y-2">
          {sectorComparison.map((sec) => {
            const isPos = sec.percent_change >= 0;
            const barWidth = Math.min(Math.abs(sec.percent_change) * 12, 100);

            return (
              <div key={sec.code} className="flex items-center gap-3 text-xs">
                <span className="w-36 text-slate-200 truncate font-medium text-2xs">
                  {sec.name}
                </span>

                <div className="flex-1 flex items-center h-5 bg-surface-base rounded overflow-hidden relative">
                  {/* Center zero line */}
                  <div className="absolute left-1/2 top-0 bottom-0 w-[1px] bg-surface-line" />

                  {isPos ? (
                    <div
                      style={{ width: `${barWidth / 2}%`, left: "50%" }}
                      className="absolute h-full bg-up/70 rounded-r"
                    />
                  ) : (
                    <div
                      style={{ width: `${barWidth / 2}%`, right: "50%" }}
                      className="absolute h-full bg-down/70 rounded-l"
                    />
                  )}
                </div>

                <div className="w-20 text-right font-mono text-2xs font-semibold">
                  <span className={isPos ? "text-up" : "text-down"}>
                    {isPos ? "+" : ""}
                    {sec.percent_change.toFixed(2)}%
                  </span>
                </div>

                <div className="w-20 text-right font-mono text-2xs text-surface-muted">
                  {num(sec.current_value)}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
