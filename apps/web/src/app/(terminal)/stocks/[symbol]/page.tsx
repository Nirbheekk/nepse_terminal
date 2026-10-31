import Link from "next/link";
import { notFound } from "next/navigation";
import { api, ApiError, exportUrl } from "@/lib/api";
import { dateTime, money, num, pct, signed, trendClass } from "@/lib/utils/format";
import { ErrorState, FreshnessBadge, Panel, Provenance, Stat, UnavailableState } from "@/components/ui";
import { FinancialChart } from "@/components/charts/financial-chart";

export const dynamic = "force-dynamic";

type Params = Promise<{ symbol: string }>;

export default async function CompanyPage({ params }: { params: Params }) {
  const { symbol } = await params;
  const ticker = decodeURIComponent(symbol).toUpperCase();	let companyRes;
	try {
		companyRes = await api.company(ticker);
	} catch (err) {
    if (err instanceof ApiError && err.status === 404) notFound();
    if (err instanceof ApiError && err.code === "API_UNREACHABLE") {
      return <ErrorState title="The market API is not reachable" detail={`${err.message} Start the Go API, then reload.`} />;
    }		throw err;
	}
	const company = companyRes.data;
	const meta = companyRes.meta;

	// Price history is optional: a company can be listed but not yet traded in a
  // stored session, and that is shown as an absence rather than a flat line.
  let bars: Awaited<ReturnType<typeof api.prices>>["data"] = [];
  let historyError: string | null = null;
  try {
    bars = (await api.prices(ticker, 120)).data;
  } catch (err) {
    historyError = err instanceof ApiError ? err.message : "Price history unavailable";
  }

  const closes = bars.map((b) => Number(b.close)).filter((n) => Number.isFinite(n) && n > 0);
  const high = closes.length ? Math.max(...closes) : null;
  const low = closes.length ? Math.min(...closes) : null;

  const ohlcvBars = bars.map((b) => ({
    time: b.trading_date,
    open: b.open,
    high: b.high,
    low: b.low,
    close: b.close,
    volume: b.volume,
    turnover: b.turnover,
    transactions: b.transactions,
    previous_close: b.previous_close,
  }));

  const listed = Number(company.listed_shares);
  const publicShares = Number(company.public_shares);	const freeFloat =
    Number.isFinite(listed) && listed > 0 && Number.isFinite(publicShares) ? (publicShares / listed) * 100 : null;
	// A listed security with no stored bar has no quote. Its zeros are absence,
	// not prices, and are shown as dashes.
	const traded = Boolean(company.trading_date);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <div className="flex flex-wrap items-baseline gap-3">
            <h1 className="text-xl font-semibold tracking-tight text-slate-100">{company.symbol}</h1>
            <Link
              href={`/companies?sector=${encodeURIComponent(company.sector_code ?? "")}`}
              className="text-xs text-surface-muted no-underline hover:text-accent"
            >
              {company.sector}
            </Link>
            {company.status !== "active" ? (
              <span className="rounded border border-accent/40 px-1.5 py-0.5 text-2xs uppercase text-accent">
                {company.status}
              </span>
            ) : null}			<FreshnessBadge freshness={meta.freshness} reason={meta.freshness_reason} />
          </div>
          <p className="text-sm text-slate-300">{company.name}</p>
        </div>
        <div className="flex items-center gap-2">
          <a
            href={exportUrl.prices(ticker)}
            className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/60 hover:text-accent"
          >
            Export prices CSV
          </a>
          <Link href="/companies" className="text-2xs text-surface-muted no-underline hover:text-accent">
            ← all companies
          </Link>
        </div>
      </div>

      <div className="panel grid grid-cols-2 divide-surface-line md:grid-cols-3 md:divide-x lg:grid-cols-6">		<Stat
          label="Last traded"
          value={traded ? num(company.close) : "—"}
          hint={
            traded ? (
              <span className={trendClass(company.change)}>
                {signed(company.change)} ({pct(company.percent_change)})
              </span>
            ) : (
              "no session stored"
            )
          }
        />
        <Stat
          label="Previous close"
          value={traded ? num(company.previous_close) : "—"}
          hint={`session ${company.trading_date || "—"}`}
        />
        <Stat label="Quantity" value={traded ? num(company.volume, "0") : "—"} hint="shares traded" />
        <Stat
          label="Turnover"
          value={traded && Number(company.turnover) > 0 ? money(company.turnover) : "—"}
          hint={`${num(company.transactions, "0")} transactions`}
        />
        <Stat
          label="Market cap"
          value={company.market_cap ? money(company.market_cap) : "—"}
          hint={
            <span title={company.market_cap_method}>
              {company.market_cap_method === "close_x_listed_shares" ? "close × listed shares" : company.market_cap_method}
            </span>
          }
        />		<Stat
          label="Float cap"
          value={company.float_market_cap ? money(company.float_market_cap) : "—"}
          hint={<span title={company.float_market_cap_method}>close × public shares</span>}
        />
      </div>

      {/* Interactive Financial Chart */}
      <Panel title={`Interactive Chart · ${company.symbol}`}>
        <div className="p-3">
          <FinancialChart symbol={company.symbol} initialBars={ohlcvBars} />
        </div>
      </Panel>

      <div className="grid gap-4 lg:grid-cols-3">
        <Panel title="Daily price history" className="lg:col-span-2">
          {historyError ? (
            <UnavailableState title="Price history unavailable" detail={historyError} />
          ) : bars.length === 0 ? (
            <UnavailableState
              title="No stored sessions for this security"
              detail="The database holds no daily bars for this symbol yet. History accrues as sessions are captured; nothing is backfilled synthetically."
            />
          ) : (
            <div className="max-h-[28rem] overflow-auto">
              <table className="w-full text-xs">
                <thead className="sticky top-0 bg-surface-raised text-surface-muted">
                  <tr>
                    <th className="cell-left font-normal">Date</th>
                    <th className="cell font-normal">Open</th>
                    <th className="cell font-normal">High</th>
                    <th className="cell font-normal">Low</th>
                    <th className="cell font-normal">Close</th>
                    <th className="cell font-normal">Quantity</th>
                    <th className="cell font-normal">Txns</th>					<th className="cell font-normal" title="Exchange figure, computed at the previous close">
                      Reported cap
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {[...bars].reverse().map((b) => (
                    <tr key={b.trading_date} className="row">
                      <td className="cell-left text-surface-muted">{b.trading_date}</td>
                      <td className="cell">{num(b.open)}</td>
                      <td className="cell">{num(b.high)}</td>
                      <td className="cell">{num(b.low)}</td>
                      <td className="cell font-medium text-slate-100">{num(b.close)}</td>
                      <td className="cell">{num(b.volume, "0")}</td>
                      <td className="cell">{num(b.transactions, "0")}</td>
                      <td className="cell text-surface-muted">{b.market_cap ? money(b.market_cap) : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Panel>

        <div className="space-y-4">
          <Panel title="Share capital">
            <dl className="divide-y divide-surface-line/60 text-xs">
              <Row label="Listed shares" value={num(company.listed_shares, "—")} />
              <Row label="Public shares" value={num(company.public_shares, "—")} />
              <Row label="Free float" value={freeFloat === null ? "—" : `${freeFloat.toFixed(1)}%`} />			  <Row label="Market cap" value={company.market_cap ? money(company.market_cap) : "—"} />
              <Row label="Float cap" value={company.float_market_cap ? money(company.float_market_cap) : "—"} />
            </dl>
          </Panel>

          <Panel title="Exchange-reported figures">
            <dl className="divide-y divide-surface-line/60 text-xs">
              <Row
                label={`Capitalisation (${company.market_cap_reported_basis ?? "source"})`}
                value={company.market_cap_reported ? money(company.market_cap_reported) : "not published"}
              />
              <Row label="Instrument" value={company.instrument} />
              <Row label="Status" value={company.status} />
              <Row label="Session" value={company.trading_date || "—"} />
              <Row label="Stored at" value={dateTime(company.ingested_at)} />
            </dl>
            <p className="px-4 py-2 text-2xs leading-relaxed text-surface-muted">
              The exchange computes its capitalisation at the previous close, which is why it differs from
              the current figure above. Both are shown rather than one being silently preferred.
            </p>
          </Panel>

          <Panel title="Window summary">
            <dl className="divide-y divide-surface-line/60 text-xs">
              <Row label="Sessions stored" value={num(bars.length, "0")} />
              <Row label="High (window)" value={high === null ? "—" : num(high)} />
              <Row label="Low (window)" value={low === null ? "—" : num(low)} />
              <Row label="Latest session" value={bars.at(-1)?.trading_date ?? "—"} />
            </dl>
          </Panel>
        </div>
      </div>		<Provenance
			source="postgres:daily_ohlcv + postgres:companies"
			asOf={meta.as_of ? dateTime(meta.as_of) : dateTime(company.ingested_at)}
			license="public_unverified"
		/>
      <p className="px-1 text-2xs leading-relaxed text-surface-muted">
        Quantity and transactions are the session totals reported with the quote. Turnover is measured from
        captured contracts where the floorsheet covers the security, and is a lower bound for that reason.
        Capitalisation methods are stated on each figure.
      </p>
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
