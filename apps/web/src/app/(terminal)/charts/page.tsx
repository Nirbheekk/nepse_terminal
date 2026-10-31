import Link from "next/link";
import { api, ApiError } from "@/lib/api";
import { dateTime, num } from "@/lib/utils/format";
import { ErrorState, Notice, Panel, Provenance, UnavailableState } from "@/components/ui";
import { FinancialChart } from "@/components/charts/financial-chart";
import type { OHLCVBar } from "@/lib/schemas";

export const dynamic = "force-dynamic";

type SearchParams = Promise<{ symbol?: string; interval?: string }>;

function one(v: string | string[] | undefined): string {
  return Array.isArray(v) ? (v[0] ?? "") : (v ?? "");
}

/**
 * Chart workspace.
 *
 * The symbol is a plain GET form rather than client state so a chart is a real
 * URL that can be linked, bookmarked and opened straight from a company page.
 */
export default async function ChartsPage({ searchParams }: { searchParams: SearchParams }) {
  const sp = await searchParams;
  const requested = one(sp.symbol).toUpperCase();
  const interval = one(sp.interval) || "1d";

  let companies: Awaited<ReturnType<typeof api.companies>>["data"] = [];
  let listError: string | null = null;
  try {
    // The list endpoint caps a page at 200, so the picker offers suggestions
    // rather than pretending to enumerate all 650 instruments; the field is
    // free text, so any symbol can still be charted by typing it.
    companies = (await api.companies({ limit: 200 })).data;
  } catch (err) {
    listError = err instanceof ApiError ? err.message : "Instrument list unavailable";
  }

  const symbol = requested || companies[0]?.symbol || "";

  let bars: OHLCVBar[] = [];
  let barsError: string | null = null;
  if (symbol) {
    try {
      bars = (await api.ohlcv(symbol, interval)).data;
    } catch (err) {
      barsError = err instanceof ApiError ? err.message : "Price history unavailable";
    }
  }

  let indicators: Awaited<ReturnType<typeof api.indicators>>["data"] = [];
  if (symbol) {
    try {
      indicators = (await api.indicators(symbol, interval)).data;
    } catch {
      // Indicators need history the database may not hold yet; the chart still
      // renders, and the panel below says which series is missing.
    }
  }

  if (listError && !symbol) {
    return <ErrorState title="The instrument list is unavailable" detail={listError} />;
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-slate-100">Charts</h1>
          <p className="mt-1 text-xs text-surface-muted">
            Daily bars from stored sessions. Intraday intervals report their own absence rather than
            interpolating a line between captures.
          </p>
        </div>

        <form method="get" className="flex flex-wrap items-end gap-2">
          <label className="text-2xs uppercase tracking-widest text-surface-muted">
            Symbol
            <input
              name="symbol"
              list="chart-symbols"
              defaultValue={symbol}
              placeholder="NABIL"
              autoComplete="off"
              className="mt-1 block w-40 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs uppercase text-slate-200"
            />
            <datalist id="chart-symbols">
              {companies.map((c) => (
                <option key={c.symbol} value={c.symbol}>
                  {c.name}
                </option>
              ))}
            </datalist>
          </label>
          <label className="text-2xs uppercase tracking-widest text-surface-muted">
            Interval
            <select
              name="interval"
              defaultValue={interval}
              className="mt-1 block w-28 rounded border border-surface-line bg-surface-raised px-2 py-1.5 text-xs text-slate-200"
            >
              {["1d", "5m", "15m", "1h"].map((i) => (
                <option key={i} value={i}>
                  {i}
                </option>
              ))}
            </select>
          </label>
          <button
            type="submit"
            className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 hover:border-accent/60 hover:text-accent"
          >
            Load
          </button>
        </form>
      </div>

      {barsError ? (
        <Notice tone="bad" title="Price history could not be loaded" detail={barsError} />
      ) : null}

      <Panel
        title={symbol ? `${symbol} · ${interval}` : "No instrument selected"}
        action={
          symbol ? (
            <Link href={`/stocks/${encodeURIComponent(symbol)}`} className="text-2xs text-accent no-underline">
              Open company page
            </Link>
          ) : null
        }
      >
        {symbol && bars.length > 0 ? (
          <div className="p-3">
            <FinancialChart symbol={symbol} initialBars={bars} />
          </div>
        ) : (
          <UnavailableState
            title="No bars for this instrument and interval"
            detail={
              symbol
                ? `The database holds no ${interval} observation for ${symbol}. Daily history is captured per session; minute bars only exist for the intervals the collector actually archived.`
                : "Choose an instrument to draw a chart."
            }
            action={symbol ? { href: `/stocks/${encodeURIComponent(symbol)}`, label: "See what is stored" } : undefined}
          />
        )}
      </Panel>

      <Panel title="Indicator readings">
        {indicators.length === 0 ? (
          <UnavailableState
            title="No indicator series"
            detail="Indicators are computed from stored bars. With two sessions on record, a 20-period average has nothing to average yet — the API says so instead of returning a number that looks like a signal."
          />
        ) : (
          <div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3">
            {indicators.map((ind, i) => {
              // Only points with a value are readings: the warm-up window is
              // emitted as nulls, and "no value yet" is not a zero.
              const points = ind.series.filter((p) => typeof p.v === "number");
              const latest = points.length ? points[points.length - 1] : null;
              return (
                <div key={`${ind.indicator}-${i}`} className="rounded border border-surface-line px-3 py-2">
                  <div className="flex items-baseline justify-between">
                    <span className="text-2xs uppercase tracking-widest text-surface-muted">
                      {ind.indicator}
                    </span>
                    <span className="text-2xs text-surface-muted">{points.length} points</span>
                  </div>
                  <div className="mt-1 text-sm tnum text-slate-200">
                    {latest && typeof latest.v === "number" ? num(latest.v, "—") : "no value"}
                  </div>
                  <div className="mt-0.5 text-2xs text-surface-muted">
                    {ind.warmup_bars > 0 ? `${ind.warmup_bars} warm-up bars` : "no warm-up"}
                    {latest?.t ? ` · ${dateTime(latest.t)}` : ""}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Panel>

      <Provenance source="postgres:daily_ohlcv · computed indicators" license="public_unverified" />
    </div>
  );
}
