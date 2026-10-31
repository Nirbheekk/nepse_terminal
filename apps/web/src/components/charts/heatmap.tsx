"use client";

import { useState, useMemo } from "react";
import Link from "next/link";
import type { Company } from "@/lib/schemas";
import { money, num, pct, signed } from "@/lib/utils/format";

type SizingMode = "market_cap" | "turnover" | "equal";

interface HeatmapProps {
  companies: Company[];
}

export function Heatmap({ companies }: HeatmapProps) {
  const [sizingMode, setSizingMode] = useState<SizingMode>("market_cap");
  const [selectedSector, setSelectedSector] = useState<string>("ALL");
  const [search, setSearch] = useState<string>("");
  const [hoveredCompany, setHoveredCompany] = useState<Company | null>(null);

  // Collect unique sectors
  const sectors = useMemo(() => {
    const set = new Set<string>();
    companies.forEach((c) => {
      if (c.sector) set.add(c.sector);
    });
    return ["ALL", ...Array.from(set).sort()];
  }, [companies]);

  // Filter companies
  const filtered = useMemo(() => {
    return companies.filter((c) => {
      if (selectedSector !== "ALL" && c.sector !== selectedSector) return false;
      if (search) {
        const q = search.toLowerCase();
        if (!c.symbol.toLowerCase().includes(q) && !c.name.toLowerCase().includes(q)) {
          return false;
        }
      }
      return true;
    });
  }, [companies, selectedSector, search]);

  // Helper for color scale based on % change (-10% to +10%)
  const getTileColor = (percentStr?: string) => {
    const p = Number(percentStr || 0);
    if (p >= 9.5) return "#10b981"; // Upper circuit (bright emerald)
    if (p >= 5) return "#16a34a";   // Strong green
    if (p >= 2) return "#22c55e";   // Green
    if (p > 0) return "#15803d";    // Soft green
    if (p === 0) return "#334155";  // Neutral slate
    if (p <= -9.5) return "#b91c1c"; // Lower circuit (dark crimson)
    if (p <= -5) return "#dc2626";  // Strong red
    if (p <= -2) return "#ef4444";  // Red
    return "#991b1b";              // Soft red
  };

  // Group by sector
  const groupedBySector = useMemo(() => {
    const map = new Map<string, Company[]>();
    filtered.forEach((c) => {
      const sec = c.sector || "Other";
      if (!map.has(sec)) map.set(sec, []);
      map.get(sec)!.push(c);
    });
    return map;
  }, [filtered]);

  return (
    <div className="space-y-4">
      {/* Controls: Sector Filter, Sizing Mode, Search */}
      <div className="flex flex-wrap items-center justify-between gap-3 text-xs">
        <div className="flex flex-wrap items-center gap-2">
          {/* Sector Selector */}
          <select
            value={selectedSector}
            onChange={(e) => setSelectedSector(e.target.value)}
            className="rounded border border-surface-line bg-surface-raised px-2.5 py-1 text-slate-100 focus:border-accent focus:outline-none"
          >
            {sectors.map((s) => (
              <option key={s} value={s}>
                Sector: {s}
              </option>
            ))}
          </select>

          {/* Sizing Mode */}
          <div className="flex rounded border border-surface-line bg-surface-raised p-0.5">
            {[
              { id: "market_cap", label: "Market Cap" },
              { id: "turnover", label: "Turnover" },
              { id: "equal", label: "Equal Size" },
            ].map((m) => (
              <button
                key={m.id}
                onClick={() => setSizingMode(m.id as SizingMode)}
                className={`rounded px-2.5 py-1 text-2xs font-medium transition-colors ${
                  sizingMode === m.id
                    ? "bg-accent/20 text-accent font-semibold"
                    : "text-surface-muted hover:text-slate-200"
                }`}
              >
                {m.label}
              </button>
            ))}
          </div>
        </div>

        {/* Search */}
        <div className="flex items-center gap-2">
          <input
            type="text"
            placeholder="Search symbol / company..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="rounded border border-surface-line bg-surface-raised px-2.5 py-1 text-xs text-slate-100 placeholder:text-surface-muted focus:border-accent focus:outline-none"
          />
          <span className="text-2xs text-surface-muted">
            {filtered.length} scrips
          </span>
        </div>
      </div>

      {/* Heatmap Legend */}
      <div className="flex flex-wrap items-center justify-between text-2xs text-surface-muted">
        <div className="flex items-center gap-1.5">
          <span>-10%</span>
          <span className="h-2.5 w-6 rounded-sm bg-[#b91c1c]" />
          <span className="h-2.5 w-6 rounded-sm bg-[#dc2626]" />
          <span className="h-2.5 w-6 rounded-sm bg-[#ef4444]" />
          <span className="h-2.5 w-6 rounded-sm bg-[#334155]" />
          <span className="h-2.5 w-6 rounded-sm bg-[#15803d]" />
          <span className="h-2.5 w-6 rounded-sm bg-[#22c55e]" />
          <span className="h-2.5 w-6 rounded-sm bg-[#10b981]" />
          <span>+10%</span>
        </div>
        <div className="text-2xs text-surface-muted">
          Sizing by: <span className="font-semibold text-slate-200">{sizingMode.replace("_", " ")}</span>
        </div>
      </div>

      {/* Heatmap Grid by Sector */}
      <div className="space-y-4">
        {Array.from(groupedBySector.entries()).map(([secName, secCompanies]) => {
          return (
            <div key={secName} className="rounded border border-surface-line/70 bg-surface-raised p-3">
              <div className="flex items-center justify-between pb-2 text-xs font-semibold text-slate-200 border-b border-surface-line/50">
                <span>{secName}</span>
                <span className="text-2xs font-normal text-surface-muted">{secCompanies.length} scrips</span>
              </div>

              <div className="mt-2.5 flex flex-wrap gap-1.5">
                {secCompanies.map((c) => {
                  const chg = Number(c.percent_change || 0);
                  const color = getTileColor(c.percent_change);

                  // Calculate proportional size
                  let sizeClass = "h-14 min-w-[70px] flex-1 max-w-[140px]";
                  if (sizingMode === "market_cap") {
                    const cap = Number(c.market_cap || 0);
                    if (cap > 100_000_000_000) sizeClass = "h-20 min-w-[130px] flex-[3] max-w-[220px]";
                    else if (cap > 40_000_000_000) sizeClass = "h-16 min-w-[100px] flex-[2] max-w-[170px]";
                  } else if (sizingMode === "turnover") {
                    const tn = Number(c.turnover || 0);
                    if (tn > 50_000_000) sizeClass = "h-20 min-w-[130px] flex-[3] max-w-[220px]";
                    else if (tn > 15_000_000) sizeClass = "h-16 min-w-[100px] flex-[2] max-w-[170px]";
                  }

                  return (
                    <Link
                      key={c.symbol}
                      href={`/companies/${encodeURIComponent(c.symbol)}`}
                      onMouseEnter={() => setHoveredCompany(c)}
                      onMouseLeave={() => setHoveredCompany(null)}
                      style={{ backgroundColor: color }}
                      className={`relative flex flex-col justify-center items-center rounded p-1.5 text-center text-white transition-transform hover:scale-[1.03] hover:z-20 hover:shadow-lg no-underline ${sizeClass}`}
                    >
                      <span className="font-bold text-xs leading-none drop-shadow-sm">{c.symbol}</span>
                      <span className="mt-1 text-2xs font-mono font-medium drop-shadow-sm">
                        {pct(c.percent_change)}
                      </span>
                      {Number(c.close) > 0 && (
                        <span className="text-3xs opacity-80 font-mono">{num(c.close)}</span>
                      )}
                    </Link>
                  );
                })}
              </div>
            </div>
          );
        })}
      </div>

      {/* Floating Hover Details Card */}
      {hoveredCompany && (
        <div className="fixed bottom-4 right-4 z-50 w-72 rounded-lg border border-surface-line bg-surface-raised p-3.5 shadow-2xl backdrop-blur-md">
          <div className="flex items-start justify-between">
            <div>
              <div className="font-bold text-sm text-slate-100">{hoveredCompany.symbol}</div>
              <div className="text-2xs text-surface-muted line-clamp-1">{hoveredCompany.name}</div>
            </div>
            <div className={`text-xs font-bold ${Number(hoveredCompany.percent_change || 0) >= 0 ? "text-up" : "text-down"}`}>
              {pct(hoveredCompany.percent_change)}
            </div>
          </div>
          <div className="mt-2.5 divide-y divide-surface-line/50 text-2xs">
            <div className="flex justify-between py-1">
              <span className="text-surface-muted">Last Price:</span>
              <span className="font-semibold text-slate-200">{num(hoveredCompany.close)}</span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-surface-muted">Change:</span>
              <span className={Number(hoveredCompany.change || 0) >= 0 ? "text-up font-semibold" : "text-down font-semibold"}>
                {signed(hoveredCompany.change)}
              </span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-surface-muted">Turnover:</span>
              <span className="text-slate-200">{money(hoveredCompany.turnover)}</span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-surface-muted">Volume:</span>
              <span className="text-slate-200">{num(hoveredCompany.volume, "0")}</span>
            </div>
            <div className="flex justify-between py-1">
              <span className="text-surface-muted">Market Cap:</span>
              <span className="text-slate-200">{money(hoveredCompany.market_cap)}</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
