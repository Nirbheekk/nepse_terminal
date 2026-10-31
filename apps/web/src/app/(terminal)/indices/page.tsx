import { api, ApiError } from "@/lib/api";
import { num, pct, signed, trendClass } from "@/lib/utils/format";
import type { IndexQuote, MarketBreadth, SectorComparisonItem } from "@/lib/schemas";
import { dateTime } from "@/lib/utils/format";
import { ErrorState, FreshnessBadge, Panel, Provenance, UnavailableState } from "@/components/ui";
import { IndexChart } from "@/components/charts/index-chart";
import { PerformanceAnalysis } from "@/components/charts/performance-analysis";

export const dynamic = "force-dynamic";

export default async function IndicesPage() {	let quotes: IndexQuote[];
	let source: string | undefined;
	let license: string | undefined;
	let asOf: string | undefined;
	let freshness: string | undefined;
	let freshnessReason: string | undefined;
  let breadth: MarketBreadth | null = null;
  let sectorComparison: SectorComparisonItem[] = [];

	try {
		const res = await api.indices();
		quotes = res.data;
		source = res.meta.source;
		license = res.meta.license_class;
		asOf = res.meta.as_of;
		freshness = res.meta.freshness;
		freshnessReason = res.meta.freshness_reason;

    try {
      const bRes = await api.marketBreadth();
      breadth = bRes.data;
    } catch {
      // Handled
    }

    try {
      const scRes = await api.sectorComparison("1d");
      sectorComparison = scRes.data;
    } catch {
      // Handled
    }
  } catch (err) {
    const e = err instanceof ApiError ? err : null;
    return (
      <ErrorState
        title="Indices could not be loaded"
        detail={e ? `${e.message} (${e.code})` : "Unknown error."}
      />
    );
  }

  const headline = quotes.filter((q) => q.kind === "headline");
  const sector = quotes
    .filter((q) => q.kind !== "headline")
    .sort((a, b) => Number(b.percent_change) - Number(a.percent_change));

  const advancing = sector.filter((q) => Number(q.percent_change) > 0).length;
  const declining = sector.filter((q) => Number(q.percent_change) < 0).length;

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-lg font-semibold tracking-tight text-slate-100">Indices &amp; sectors</h1>		<p className="text-xs text-surface-muted">
          {quotes.length} indices tracked · {advancing} sector{advancing === 1 ? "" : "s"} up, {declining} down
        </p>
      </div>
      <FreshnessBadge freshness={freshness} reason={freshnessReason} />

      {/* Historical & Intraday Index Chart */}
      <Panel title="Historical Index Trends & Session Intraday">
        <div className="p-4">
          <IndexChart indices={quotes} defaultCode="nepse" />
        </div>
      </Panel>

      {/* Sector Performance Analysis & Comparison */}
      <Panel title="Sector Comparison & Market Performance">
        <div className="p-4">
          <PerformanceAnalysis initialBreadth={breadth} initialSectors={sectorComparison} />
        </div>
      </Panel>

      {quotes.length === 0 ? (
        <Panel title="Indices">
          <UnavailableState
            title="No index observations stored"
            detail="Index values are captured with each session. Until one is promoted into the database there is nothing to show."
          />
        </Panel>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          <Panel title="Headline indices">
            <IndexTable rows={headline} />
          </Panel>
          <Panel title="Sector performance (by % change)">
            <IndexTable rows={sector} />
          </Panel>
        </div>
      )}		<Provenance source={source} asOf={asOf ? dateTime(asOf) : undefined} license={license} />
    </div>
  );
}

function IndexTable({ rows }: { rows: IndexQuote[] }) {
  if (rows.length === 0) {
    return (
      <UnavailableState
        title="Nothing in this group"
        detail="This group has no observations for the latest stored session."
      />
    );
  }
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-xs">
        <thead className="text-surface-muted">
          <tr>
            <th className="cell-left font-normal">Index</th>
            <th className="cell font-normal">Value</th>
            <th className="cell font-normal">Change</th>
            <th className="cell font-normal">%</th>
            <th className="cell-left font-normal">Session</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((q) => (
            <tr key={q.code} className="row">
              <td className="cell-left text-slate-300">{q.name}</td>
              <td className="cell">{num(q.value)}</td>
              <td className={`cell ${trendClass(q.point_change)}`}>{signed(q.point_change)}</td>
              <td className={`cell ${trendClass(q.percent_change)}`}>{pct(q.percent_change)}</td>
              <td className="cell-left text-2xs text-surface-muted">{q.trading_date || "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
