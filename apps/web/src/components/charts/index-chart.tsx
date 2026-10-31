"use client";

import { useEffect, useRef, useState } from "react";
import type { IndexQuote, IndexTrendPoint } from "@/lib/schemas";
import { num, signed, pct } from "@/lib/utils/format";

interface IndexChartProps {
  indices: IndexQuote[];
  defaultCode?: string;
}

export function IndexChart({ indices, defaultCode = "nepse" }: IndexChartProps) {
  const [selectedCode, setSelectedCode] = useState<string>(defaultCode);
  const [mode, setMode] = useState<"intraday" | "daily">("intraday");
  const [points, setPoints] = useState<IndexTrendPoint[]>([]);
  const [loading, setLoading] = useState(false);
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);

  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  const currentIndex = indices.find((i) => i.code === selectedCode) || indices[0];

  useEffect(() => {
    let active = true;
    const fetchPoints = async () => {
      setLoading(true);
      try {
        const url =
          mode === "intraday"
            ? `/api/v1/indices/${encodeURIComponent(selectedCode)}/intraday`
            : `/api/v1/indices/${encodeURIComponent(selectedCode)}/history?granularity=daily`;

        const res = await fetch(url);
        if (res.ok) {
          const json = await res.json();
          if (active && json.data) {
            setPoints(json.data);
          }
        }
      } catch {
        // Handled
      } finally {
        if (active) setLoading(false);
      }
    };

    fetchPoints();
    return () => {
      active = false;
    };
  }, [selectedCode, mode]);

  // Canvas drawing
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();
    const width = rect.width;
    const height = rect.height;

    canvas.width = width * dpr;
    canvas.height = height * dpr;
    ctx.scale(dpr, dpr);

    // Background
    ctx.fillStyle = "#090d16";
    ctx.fillRect(0, 0, width, height);

    if (points.length === 0) {
      ctx.fillStyle = "#64748b";
      ctx.font = "12px sans-serif";
      ctx.textAlign = "center";
      ctx.fillText(
        loading ? "Loading index data..." : `No ${mode} observations recorded for this index`,
        width / 2,
        height / 2
      );
      return;
    }

    const rightMargin = 65;
    const chartWidth = width - rightMargin;
    const bottomMargin = 25;
    const chartHeight = height - bottomMargin;

    const vals = points.map((p) => Number(p.value));
    let minVal = Math.min(...vals);
    let maxVal = Math.max(...vals);
    if (minVal === maxVal) {
      minVal *= 0.99;
      maxVal *= 1.01;
    }
    const pad = (maxVal - minVal) * 0.1;
    minVal -= pad;
    maxVal += pad;

    const valToY = (v: number) => {
      return chartHeight - ((v - minVal) / (maxVal - minVal)) * (chartHeight - 20) - 10;
    };

    // Horizontal Grid & Labels
    ctx.strokeStyle = "rgba(255, 255, 255, 0.05)";
    ctx.lineWidth = 1;
    const steps = 4;
    for (let i = 0; i <= steps; i++) {
      const v = minVal + ((maxVal - minVal) / steps) * i;
      const y = valToY(v);
      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(chartWidth, y);
      ctx.stroke();

      ctx.fillStyle = "#64748b";
      ctx.font = "10px monospace";
      ctx.textAlign = "left";
      ctx.fillText(v.toFixed(1), chartWidth + 6, y + 3);
    }

    // Trend Line
    const firstPoint = points[0];
    const lastPoint = points[points.length - 1];
    if (!firstPoint || !lastPoint) return;

    const firstVal = Number(firstPoint.value);
    const lastVal = Number(lastPoint.value);
    const isUp = lastVal >= firstVal;
    const strokeColor = isUp ? "#22c55e" : "#ef4444";

    ctx.beginPath();
    points.forEach((p, i) => {
      const x = (i / Math.max(points.length - 1, 1)) * chartWidth;
      const y = valToY(Number(p.value));
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.strokeStyle = strokeColor;
    ctx.lineWidth = 2;
    ctx.stroke();

    // Area Gradient
    ctx.lineTo(chartWidth, chartHeight);
    ctx.lineTo(0, chartHeight);
    ctx.closePath();

    const grad = ctx.createLinearGradient(0, 0, 0, chartHeight);
    grad.addColorStop(0, isUp ? "rgba(34, 197, 94, 0.25)" : "rgba(239, 68, 68, 0.25)");
    grad.addColorStop(1, "rgba(0, 0, 0, 0)");
    ctx.fillStyle = grad;
    ctx.fill();

    // Time Axis
    const labelStep = Math.max(Math.floor(points.length / 5), 1);
    ctx.fillStyle = "#64748b";
    ctx.font = "10px monospace";
    ctx.textAlign = "center";
    for (let i = 0; i < points.length; i += labelStep) {
      const pt = points[i];
      if (!pt) continue;
      const x = (i / Math.max(points.length - 1, 1)) * chartWidth;
      const t = pt.time;
      const label = t.length > 10 ? t.slice(11, 16) : t;
      ctx.fillText(label, x, height - 6);
    }

    // Crosshair
    if (hoverIndex !== null && hoverIndex >= 0 && hoverIndex < points.length) {
      const hp = points[hoverIndex];
      if (!hp) return;
      const hx = (hoverIndex / Math.max(points.length - 1, 1)) * chartWidth;
      const hy = valToY(Number(hp.value));

      ctx.strokeStyle = "rgba(255, 255, 255, 0.35)";
      ctx.lineWidth = 1;
      ctx.setLineDash([3, 3]);

      ctx.beginPath();
      ctx.moveTo(hx, 0);
      ctx.lineTo(hx, chartHeight);
      ctx.stroke();

      ctx.beginPath();
      ctx.moveTo(0, hy);
      ctx.lineTo(chartWidth, hy);
      ctx.stroke();
      ctx.setLineDash([]);

      // Point circle
      ctx.fillStyle = strokeColor;
      ctx.beginPath();
      ctx.arc(hx, hy, 4, 0, Math.PI * 2);
      ctx.fill();

      // Price Tag on Right Axis
      ctx.fillStyle = "#1e293b";
      ctx.fillRect(chartWidth + 2, hy - 9, 56, 18);
      ctx.strokeStyle = "#475569";
      ctx.strokeRect(chartWidth + 2, hy - 9, 56, 18);
      ctx.fillStyle = "#f8fafc";
      ctx.font = "10px monospace";
      ctx.textAlign = "left";
      ctx.fillText(Number(hp.value).toFixed(1), chartWidth + 6, hy + 4);
    }
  }, [points, mode, loading, hoverIndex]);

  const handleMouseMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current;
    if (!canvas || points.length === 0) return;
    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const chartWidth = rect.width - 65;
    if (x < 0 || x > chartWidth) {
      setHoverIndex(null);
      return;
    }
    const idx = Math.round((x / chartWidth) * (points.length - 1));
    if (idx >= 0 && idx < points.length) {
      setHoverIndex(idx);
    }
  };

  const hoveredPoint = hoverIndex !== null && points[hoverIndex] ? points[hoverIndex] : points[points.length - 1];

  return (
    <div className="space-y-3">
      {/* Header controls: Index selector, Mode toggle, Granularity badge */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-surface-line/70 pb-2">
        <div className="flex flex-wrap items-center gap-2">
          {/* Index Selector */}
          <select
            value={selectedCode}
            onChange={(e) => setSelectedCode(e.target.value)}
            className="rounded border border-surface-line bg-surface-raised px-2.5 py-1 text-xs font-semibold text-slate-100 focus:border-accent focus:outline-none"
          >
            {indices.map((idx) => (
              <option key={idx.code} value={idx.code}>
                {idx.name} ({idx.code})
              </option>
            ))}
          </select>

          {/* Intraday vs Daily Closes Toggle */}
          <div className="flex rounded border border-surface-line bg-surface-raised p-0.5 text-xs">
            <button
              onClick={() => setMode("intraday")}
              className={`rounded px-2.5 py-1 text-2xs font-medium transition-colors ${
                mode === "intraday" ? "bg-accent/20 text-accent font-semibold" : "text-surface-muted hover:text-slate-200"
              }`}
            >
              Session Intraday
            </button>
            <button
              onClick={() => setMode("daily")}
              className={`rounded px-2.5 py-1 text-2xs font-medium transition-colors ${
                mode === "daily" ? "bg-accent/20 text-accent font-semibold" : "text-surface-muted hover:text-slate-200"
              }`}
            >
              Daily Historical Closes
            </button>
          </div>
        </div>

        {/* Clear separation banner/badge */}
        <div className="flex items-center gap-2">
          <span
            className={`rounded border px-2 py-0.5 text-2xs font-mono uppercase tracking-wider ${
              mode === "intraday" ? "border-sky-500/40 text-sky-400" : "border-emerald-500/40 text-emerald-400"
            }`}
          >
            {mode === "intraday" ? "Actual Intraday Observations (Session)" : "Daily Historical Closes"}
          </span>
        </div>
      </div>

      {/* Point Readout */}
      {hoveredPoint && (
        <div className="flex flex-wrap items-center gap-4 text-xs font-mono">
          <span className="text-surface-muted">
            Time: <span className="text-slate-200">{hoveredPoint.time}</span>
          </span>
          <span className="text-surface-muted">
            Index Level: <span className="font-semibold text-slate-100">{num(hoveredPoint.value)}</span>
          </span>
          {currentIndex && (
            <span
              className={
                Number(currentIndex.point_change) >= 0 ? "text-up font-semibold" : "text-down font-semibold"
              }
            >
              {signed(currentIndex.point_change)} ({pct(currentIndex.percent_change)})
            </span>
          )}
        </div>
      )}

      {/* Canvas */}
      <div className="relative w-full h-[320px] rounded border border-surface-line/70 bg-[#090d16] overflow-hidden">
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
