import Link from "next/link";
import { api, ApiError } from "@/lib/api";
import { dateTime, money, num, pct, signed, timeOnly, trendClass } from "@/lib/utils/format";
import type { Company, IndexQuote, MarketBreadth, MarketStatus, Mover } from "@/lib/schemas";
import {
  Badge,
  ErrorState,
  FreshnessBadge,
  Notice,
  Panel,
  Provenance,
  Stat,
  UnavailableState,
} from "@/components/ui";
import { Heatmap } from "@/components/charts/heatmap";
import { PerformanceAnalysis } from "@/components/charts/performance-analysis";

export const dynamic = "force-dynamic";

export default async function OverviewPage() {
  let overview;
  try {
    overview = await api.overview();
  } catch (err) {
    const e = err instanceof ApiError ? err : null;
    const unreachable = e?.code === "API_UNREACHABLE";
    return (
      <div className="space-y-4">
        <PageHeading />
        <ErrorState
          title={unreachable ? "The market API is not reachable" : "The market overview could not be loaded"}
          detail={
            unreachable
              ? `Nothing was fetched from http://localhost:8081. Start the Go API (make api) or the compose stack (docker compose up -d), then reload. No cached market figures are shown, because a stale quote that looks live is worse than no quote.`
              : e?.code === "NOT_FOUND"
                ? "The database is reachable but holds no session yet. Run `make seed` (or let the collector capture a session) and reload."
                : `${e?.message ?? "Unknown error"} (${e?.code ?? "UNKNOWN"})`
          }
        />
      </div>
    );
  }

  const { data: ov, meta } = overview;
  const s = ov.summary;
  const st: MarketStatus | null = ov.status ?? null;

  const headline = ov.indices.filter((i) => i.kind === "headline");
  const sectors = ov.indices
    .filter((i) => i.kind !== "headline")
    .sort((a, b) => Number(b.percent_change) - Number(a.percent_change));

  const cap = Number(s.market_cap);
  const floatCap = Number(s.float_market_cap);
  const floatPct = cap > 0 ? (floatCap / cap) * 100 : null;

  let companies: Company[] = [];
  try {
    const compRes = await api.companies({ limit: 120 });
    companies = compRes.data;
  } catch {
    // Handled
  }

  let breadth: MarketBreadth | null = null;
  try {
    const bRes = await api.marketBreadth();
    breadth = bRes.data;
  } catch {
    // Handled
  }

  return (
    <div className="space-y-4">
      <PageHeading session={s.trading_date} state={st} freshness={meta.freshness} reason={meta.freshness_reason} />

      {meta.freshness === "stale" ? (
        <Notice
          tone="bad"
          title="This data may be out of date"
          detail={`${meta.freshness_reason ?? "The newest capture is older than expected."} Last capture ${
            meta.as_of ? dateTime(meta.as_of) : "unknown"
          }. Figures below are the last stored session, not live quotes.`}
        />
      ) : null}

      <div className="panel grid grid-cols-2 divide-surface-line md:grid-cols-4 md:divide-x">
        <Stat
          label="NEPSE index"
          value={num(s.nepse_index)}
          hint={
            <span className={trendClass(s.point_change)}>
              {signed(s.point_change)} ({pct(s.percent_change)})
            </span>
          }
        />
        <Stat
          label="Turnover"
          value={money(s.turnover)}
          hint={`${num(s.traded_shares, "0")} shares · ${num(s.transactions, "0")} transactions`}
        />
        <Stat
          label="Market capitalisation"
          value={money(s.market_cap)}
          hint={
            <>
              <span className="text-slate-300">float {money(s.float_market_cap)}</span>
              {floatPct !== null ? <span> · {floatPct.toFixed(1)}% of total</span> : null}
            </>
          }
        />
        <Stat
          label="Advances / declines"
          value={
            <span>
              <span className="text-up">{s.advanced}</span>
              <span className="text-surface-muted"> / </span>
              <span className="text-down">{s.declined}</span>
            </span>
          }
          hint={`${s.unchanged} unchanged · ${s.traded_securities} scrips traded`}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Panel title="Session totals">
          <dl className="divide-y divide-surface-line/60 text-xs">
            <Row label="Trading date" value={s.trading_date} />
            <Row label="Turnover" value={money(s.turnover)} />
            <Row label="Quantity traded" value={num(s.traded_shares, "0")} />
            <Row label="Transactions" value={num(s.transactions, "0")} />
            <Row label="Scrips traded" value={num(s.traded_securities, "0")} />
            <Row label="Advanced" value={String(s.advanced)} />
            <Row label="Declined" value={String(s.declined)} />
            <Row label="Unchanged" value={String(s.unchanged)} />
          </dl>
        </Panel>

        <Panel title="Capitalisation">
          <dl className="divide-y divide-surface-line/60 text-xs">
            <Row label="Market capitalisation" value={money(s.market_cap)} />
            <Row label="Float capitalisation" value={money(s.float_market_cap)} />
            <Row label="Float share" value={floatPct === null ? "—" : `${floatPct.toFixed(2)}%`} />			<Row
              label="Exchange coverage"
              value={
                meta.coverage ? `${meta.coverage.covered} of ${meta.coverage.eligible} companies` : "not reported"
              }
            />
            <Row label="Method" value={s.cap_method || "not recorded"} />
          </dl>		  <p className="px-4 py-2 text-2xs leading-relaxed text-surface-muted">
            Total capitalisation is the exchange&apos;s own session figure for all {s.cap_eligible} listed
            companies. Company pages compute their own figure at the current close, for the issuers whose
            share counts the exchange publishes, which is why those components do not add up to this
            total.
          </p>
        </Panel>

        <Panel title="Market status">
          <dl className="divide-y divide-surface-line/60 text-xs">
            <Row label="State" value={st ? st.state.toUpperCase() : "unknown"} />
            <Row label="Reported by feed" value={st?.reported_status || "—"} />
            <Row label="Exchange as of" value={st?.as_of ? dateTime(st.as_of) : "—"} />
            <Row label="Last capture" value={st?.observed_at ? dateTime(st.observed_at) : "—"} />
            <Row label="Session" value={st?.session_date || s.trading_date} />
            <Row
              label="Status applies to"
              value={st ? (st.is_current_session ? "the stored session" : "an earlier session") : "—"}
            />
            <Row label="Stored at" value={dateTime(s.ingested_at)} />
          </dl>
          {!st ? (
            <p className="px-4 py-2 text-2xs leading-relaxed text-surface-muted">
              No open/closed state has been captured yet. The terminal shows “unknown” rather than
              assuming the market is closed.
            </p>
          ) : null}
        </Panel>
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <MoverPanel title="Top gainers" rows={ov.movers.gainers} tone="up" />
        <MoverPanel title="Top losers" rows={ov.movers.losers} tone="down" />
        <MoverPanel title="Turnover leaders" rows={ov.movers.turnover} tone="neutral" />
      </div>

      {/* Market Breadth & Performance Analysis */}
      <Panel title="Market Breadth & Performance Analysis">
        <div className="p-4">
          <PerformanceAnalysis initialBreadth={breadth} />
        </div>
      </Panel>

      {/* Sector & Market Heatmap */}
      <Panel
        title="Market Heatmap"
        action={
          <Link href="/heatmap" className="text-2xs text-surface-muted no-underline hover:text-accent">
            full heatmap workspace →
          </Link>
        }
      >
        <div className="p-4">
          <Heatmap companies={companies} />
        </div>
      </Panel>

      <div className="grid gap-4 lg:grid-cols-2">
        <Panel
          title="Headline indices"
          action={
            <Link href="/indices" className="text-2xs text-surface-muted no-underline hover:text-accent">
              all indices →
            </Link>
          }
        >
          <IndexTable rows={headline} session={s.trading_date} />
        </Panel>
        <Panel title="Sector performance">
          <IndexTable rows={sectors} session={s.trading_date} />
        </Panel>
      </div>

      <Provenance
        source={meta.source}
        asOf={meta.as_of ? `${dateTime(meta.as_of)} (${timeOnly(meta.as_of)})` : undefined}
        license={meta.license_class}
      />
      <p className="px-1 text-2xs leading-relaxed text-surface-muted">
        Freshness: <FreshnessBadge freshness={meta.freshness} reason={meta.freshness_reason} />{" "}
        {meta.freshness_reason ?? "no verdict recorded"}		{meta.calculation ? ` · calculation: ${meta.calculation}` : ""}
      </p>
    </div>
  );
}

function PageHeading({
  session,
  state,
  freshness,
  reason,
}: {
  session?: string;
  state?: MarketStatus | null;
  freshness?: string;
  reason?: string;
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-2">
      <div>
        <h1 className="text-lg font-semibold tracking-tight text-slate-100">Market overview</h1>
        <p className="text-xs text-surface-muted">
          {session ? `Session of ${session}` : "Waiting for a captured session"}
          {state ? ` · exchange ${state.state} at ${state.as_of ? dateTime(state.as_of) : "unknown"}` : ""}
        </p>
      </div>
      <div className="flex items-center gap-2">
        <FreshnessBadge freshness={freshness} reason={reason} />
        <Badge tone="warn">unverified public data</Badge>
      </div>
    </div>
  );
}

function MoverPanel({
  title,
  rows,
  tone,
}: {
  title: string;
  rows: Mover[];
  tone: "up" | "down" | "neutral";
}) {
  if (rows.length === 0) {
    return (
      <Panel title={title}>
        <UnavailableState
          title="No priced securities"
          detail="Rankings need a stored session with both a close and a previous close, so a change can be computed."
        />
      </Panel>
    );
  }
  return (
    <Panel title={title}>
      <table className="w-full text-xs">
        <thead className="text-surface-muted">
          <tr>
            <th className="cell-left font-normal">Symbol</th>
            <th className="cell font-normal">LTP</th>
            <th className="cell font-normal">Chg</th>
            <th className="cell font-normal">%</th>
            <th className="cell font-normal">Turnover</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((m) => (
            <tr key={m.symbol} className="row">
              <td className="cell-left">
                <Link href={`/companies/${m.symbol}`} className="text-slate-200 no-underline hover:text-accent">
                  {m.symbol}
                </Link>
              </td>
              <td className="cell">{num(m.close)}</td>
              <td className={`cell ${trendClass(m.change)}`}>{signed(m.change)}</td>
              <td className={`cell ${tone === "neutral" ? trendClass(m.percent_change) : tone === "up" ? "text-up" : "text-down"}`}>
                {pct(m.percent_change)}
              </td>
              <td className="cell text-surface-muted">{money(m.turnover)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </Panel>
  );
}

function IndexTable({ rows, session }: { rows: IndexQuote[]; session: string }) {
  if (rows.length === 0) {
    return (
      <UnavailableState
        title="No index observations stored"
        detail="Index values appear once a session has been captured and promoted into the database."
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
          </tr>
        </thead>
        <tbody>
          {rows.map((i) => (
            <tr key={i.code} className="row">
              <td className="cell-left text-slate-300">{i.name}</td>
              <td className="cell">{num(i.value)}</td>
              <td className={`cell ${trendClass(i.point_change)}`}>{signed(i.point_change)}</td>
              <td className={`cell ${trendClass(i.percent_change)}`}>{pct(i.percent_change)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="px-3 py-2 text-2xs text-surface-muted">Session {session}</p>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 px-4 py-1.5">
      <dt className="text-surface-muted">{label}</dt>
      <dd className="tnum text-right text-slate-300">{value}</dd>
    </div>
  );
}
