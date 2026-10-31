import Link from "next/link";
import { api, ApiError, floorsheetExportUrl, type FloorsheetQuery } from "@/lib/api";
import { dateTime, money, num, timeOnly } from "@/lib/utils/format";
import { ErrorState, FreshnessBadge, Notice, Panel, Provenance, Stat, UnavailableState } from "@/components/ui";

export const dynamic = "force-dynamic";

type SearchParams = Promise<Record<string, string | string[] | undefined>>;

const SORTS = {
  contract_desc: "Newest contract first",
  amount_desc: "Amount: high → low",
  amount_asc: "Amount: low → high",
  time_desc: "Execution time: latest first",
  time_asc: "Execution time: earliest first",
} as const;

const PAGE_SIZES = [25, 50, 100, 200];

const one = (v: string | string[] | undefined): string =>
  (Array.isArray(v) ? (v[0] ?? "") : (v ?? "")).trim();

export default async function FloorsheetPage({ searchParams }: { searchParams: SearchParams }) {
  const sp = await searchParams;
  const date = one(sp.date);
  const symbol = one(sp.symbol).toUpperCase();
  const broker = one(sp.broker);
  const side = one(sp.side);
  const minAmount = one(sp.min_amount);
  const maxAmount = one(sp.max_amount);
  const sort = one(sp.sort) || "contract_desc";
  const page = Math.max(1, Number.parseInt(one(sp.page) || "1", 10) || 1);
  const limit = PAGE_SIZES.includes(Number(one(sp.limit))) ? Number(one(sp.limit)) : 50;

  // The API validates these too; this keeps a malformed link from producing an
  // ugly error page instead of an explanation.
  const problems: string[] = [];
  if (date && !/^\d{4}-\d{2}-\d{2}$/.test(date)) problems.push("date must be YYYY-MM-DD");
  if (symbol && !/^[A-Z0-9][A-Z0-9._-]{0,19}$/.test(symbol)) problems.push("symbol is not valid");
  if (broker && !/^[A-Za-z0-9_-]{1,32}$/.test(broker)) problems.push("broker must be a member id");
  if (side && side !== "buy" && side !== "sell") problems.push("side must be buy or sell");
  if (side && !broker) problems.push("side requires a broker");
  if (!(sort in SORTS)) problems.push("unknown sort");
  if (problems.length > 0) {
    return (
      <ErrorState
        title="Invalid filter"
        detail={`${problems.join("; ")}. Use the form below, which only builds valid links.`}
        action={{ href: "/floorsheet", label: "Reset filters" }}
      />
    );
  }

  const query: FloorsheetQuery = {
    date: date || undefined,
    symbol: symbol || undefined,
    broker: broker || undefined,
    side: (side as "buy" | "sell" | "") || undefined,
    min_amount: minAmount || undefined,
    max_amount: maxAmount || undefined,
    sort: sort as FloorsheetQuery["sort"],
    limit,
    page,
  };

  try {
    const [trades, brokers, coverage] = await Promise.all([
      api.floorsheet(query),
      api.floorsheetBrokers({ date: date || undefined, limit: 10 }),
      api.floorsheetCoverage(),
    ]);

    const rows = trades.data;
    const total = trades.meta.total ?? rows.length;
    const first = total === 0 ? 0 : (page - 1) * limit + 1;
    const last = (page - 1) * limit + rows.length;
    const pages = Math.max(1, Math.ceil(total / limit));

    const cov = coverage.data.coverage;
    const session = date || cov.latest_present;
    const sessionCoverage = cov.dates.find((d) => d.trading_date === session);
    const brokerCoverage = brokers.data.coverage;
    const run = coverage.data.runs.find((r) => r.trading_date === session);

    const pageAmount = rows.reduce((sum, t) => sum + Number(t.amount || 0), 0);

    const href = (over: Partial<FloorsheetQuery & { reset?: boolean }>): string => {
      const params = new URLSearchParams();
      const merged: FloorsheetQuery = { ...query, page: undefined, ...over };
      if (over.reset) {
        // Clearing means clearing: a filter left behind would silently disagree
        // with the form the reader is looking at.
        if (over.date) params.set("date", over.date);
        return `/floorsheet?${params.toString()}`;
      }
      if (merged.date) params.set("date", merged.date);
      if (merged.symbol) params.set("symbol", merged.symbol);
      if (merged.broker) params.set("broker", merged.broker);
      if (merged.broker && merged.side) params.set("side", merged.side);
      if (merged.min_amount) params.set("min_amount", merged.min_amount);
      if (merged.max_amount) params.set("max_amount", merged.max_amount);
      if (merged.sort && merged.sort !== "contract_desc") params.set("sort", merged.sort);
      if (merged.limit && merged.limit !== 50) params.set("limit", String(merged.limit));
      if (merged.page && merged.page > 1) params.set("page", String(merged.page));
      return `/floorsheet?${params.toString()}`;
    };

    const sortLink = (key: keyof typeof SORTS) => href({ sort: key, page: 1 });

    return (
      <div className="space-y-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h1 className="text-lg font-semibold tracking-tight text-slate-100">Floorsheet</h1>
            <p className="text-xs text-surface-muted">
              <FreshnessBadge freshness={trades.meta.freshness} reason={trades.meta.freshness_reason} />{" "}
              Trade-level contracts · session {session || "—"}
              {symbol ? ` · ${symbol}` : ""}
              {broker ? ` · broker ${broker}${side ? ` (${side})` : ""}` : ""}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <a
              href={floorsheetExportUrl(query)}
              className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/60 hover:text-accent"
            >
              Export CSV ({num(total)} rows)
            </a>
            {date || symbol || broker || minAmount || maxAmount ? (
              <Link
                href="/floorsheet"
                className="rounded border border-surface-line px-3 py-1.5 text-xs text-surface-muted no-underline hover:border-accent/60 hover:text-accent"
              >
                Clear filters
              </Link>
            ) : null}
          </div>
        </div>

        {/* Coverage first: a reader must know whether a quiet table means a quiet
            market or an uncaptured session, before reading any number. */}
        {sessionCoverage ? (
          <div className="panel grid grid-cols-2 divide-x divide-surface-line sm:grid-cols-3 lg:grid-cols-6">
            <Stat
              label="Session"
              value={sessionCoverage.trading_date}
              hint={sessionCoverage.status === "complete" ? "verified complete" : sessionCoverage.status}
              tone={sessionCoverage.status === "complete" ? "text-slate-100" : "text-accent"}
            />
            <Stat
              label="Contracts stored"
              value={num(sessionCoverage.contracts)}
              hint={
                sessionCoverage.contracts_reported !== undefined
                  ? `exchange reported ${num(sessionCoverage.contracts_reported)}`
                  : "reported total unavailable"
              }
            />
            <Stat
              label="Pages"
              value={`${sessionCoverage.pages_present}/${sessionCoverage.pages_expected}`}
              hint={sessionCoverage.converged ? "sweep converged" : "not proven converged"}
            />
            <Stat label="Scrips traded" value={num(sessionCoverage.symbols)} />
            <Stat label="Contract turnover" value={money(sessionCoverage.turnover)} />
            <Stat
              label="Attribution"
              value={
                sessionCoverage.contracts > 0
                  ? `${num(sessionCoverage.contracts - sessionCoverage.unattributed_contracts)} attributed`
                  : "—"
              }
              hint={`${num(sessionCoverage.unattributed_contracts)} without a member id`}
            />
          </div>
        ) : null}

        <Panel
          title="Session coverage"
          action={
            <span className="text-2xs text-surface-muted">
              {cov.earliest_verified
                ? `verified ${cov.earliest_verified} → ${cov.latest_verified}`
                : "no session proven complete"}
            </span>
          }
        >
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead className="text-surface-muted">
                <tr>
                  <th className="cell-left font-normal">Date</th>
                  <th className="cell-left font-normal">Verdict</th>
                  <th className="cell font-normal">Contracts</th>
                  <th className="cell font-normal">Reported</th>
                  <th className="cell font-normal">Missing</th>
                  <th className="cell font-normal">Pages</th>
                  <th className="cell font-normal">Turnover</th>
                  <th className="cell-left font-normal">Precision</th>
                  <th className="cell-left font-normal" />
                </tr>
              </thead>
              <tbody>
                {cov.dates.slice(0, 12).map((d) => (
                  <tr key={d.trading_date} className={`row ${d.trading_date === session ? "bg-white/[0.03]" : ""}`}>
                    <td className="cell-left">{d.trading_date}</td>
                    <td className="cell-left">
                      <span
                        className={
                          d.status === "complete"
                            ? "text-up"
                            : d.status === "missing"
                              ? "text-surface-muted"
                              : "text-accent"
                        }
                      >
                        {d.status}
                      </span>
                    </td>
                    <td className="cell">{num(d.contracts)}</td>
                    <td className="cell">{d.contracts_reported !== undefined ? num(d.contracts_reported) : "—"}</td>
                    <td className="cell">{d.missing_contracts !== undefined ? num(d.missing_contracts) : "—"}</td>
                    <td className="cell">
                      {d.pages_expected > 0 ? `${d.pages_present}/${d.pages_expected}` : "—"}
                    </td>
                    <td className="cell">{money(d.turnover)}</td>
                    <td className="cell-left text-2xs text-surface-muted">{d.timestamp_precision ?? "—"}</td>
                    <td className="cell-left">
                      {d.contracts > 0 ? (
                        <Link
                          href={href({ date: d.trading_date, page: 1 })}
                          className="text-2xs text-surface-muted no-underline hover:text-accent"
                        >
                          view
                        </Link>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {cov.missing_dates.length > 0 ? (
            <div className="px-4 pt-3">
              <Notice
                tone="neutral"
                title={`${cov.missing_dates.length} tracked session(s) hold no contracts`}
                detail={`${cov.missing_dates.join(", ")} — the exchange serves the tape only for the live session, so these cannot be recovered retroactively. They are listed as missing rather than shown as zero-trade days.`}
              />
            </div>
          ) : null}
        </Panel>

        <Panel title="Filters">
          <form action="/floorsheet" method="get" className="flex flex-wrap items-end gap-2 px-4 py-3">
            <label className="flex flex-col gap-1 text-2xs uppercase tracking-wider text-surface-muted">
              Session
              <input
                type="date"
                name="date"
                defaultValue={date}
                className="rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200 focus:border-accent/60 focus:outline-none"
              />
            </label>
            <label className="flex flex-col gap-1 text-2xs uppercase tracking-wider text-surface-muted">
              Symbol
              <input
                type="text"
                name="symbol"
                defaultValue={symbol}
                placeholder="any"
                className="w-28 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs uppercase text-slate-200 placeholder:normal-case placeholder:text-surface-muted focus:border-accent/60 focus:outline-none"
              />
            </label>
            <label className="flex flex-col gap-1 text-2xs uppercase tracking-wider text-surface-muted">
              Broker (member id)
              <input
                type="text"
                name="broker"
                defaultValue={broker}
                placeholder="any"
                className="w-32 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200 placeholder:text-surface-muted focus:border-accent/60 focus:outline-none"
              />
            </label>
            <label className="flex flex-col gap-1 text-2xs uppercase tracking-wider text-surface-muted">
              Side
              <select
                name="side"
                defaultValue={side}
                className="rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200 focus:border-accent/60 focus:outline-none"
              >
                <option value="">either</option>
                <option value="buy">buyer</option>
                <option value="sell">seller</option>
              </select>
            </label>
            <label className="flex flex-col gap-1 text-2xs uppercase tracking-wider text-surface-muted">
              Min amount
              <input
                type="text"
                inputMode="decimal"
                name="min_amount"
                defaultValue={minAmount}
                placeholder="any"
                className="w-24 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200 placeholder:text-surface-muted focus:border-accent/60 focus:outline-none"
              />
            </label>
            <label className="flex flex-col gap-1 text-2xs uppercase tracking-wider text-surface-muted">
              Max amount
              <input
                type="text"
                inputMode="decimal"
                name="max_amount"
                defaultValue={maxAmount}
                placeholder="any"
                className="w-24 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200 placeholder:text-surface-muted focus:border-accent/60 focus:outline-none"
              />
            </label>
            <label className="flex flex-col gap-1 text-2xs uppercase tracking-wider text-surface-muted">
              Sort
              <select
                name="sort"
                defaultValue={sort}
                className="rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200 focus:border-accent/60 focus:outline-none"
              >
                {Object.entries(SORTS).map(([key, label]) => (
                  <option key={key} value={key}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-1 text-2xs uppercase tracking-wider text-surface-muted">
              Rows
              <select
                name="limit"
                defaultValue={String(limit)}
                className="rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200 focus:border-accent/60 focus:outline-none"
              >
                {PAGE_SIZES.map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="submit"
              className="rounded border border-accent/50 px-3 py-1.5 text-xs text-accent hover:bg-accent/10"
            >
              Apply
            </button>
          </form>
        </Panel>

        <div className="grid gap-4 lg:grid-cols-[1fr_320px]">
          <Panel title="Contracts">
            {rows.length === 0 ? (
              <UnavailableState
                title={total === 0 ? "No contracts match this filter" : "No rows on this page"}
                detail={
                  total === 0
                    ? "Zero matches is a filter result, not a data gap. Clear a filter to see the session's contracts; sessions listed as missing above genuinely hold none."
                    : `The filter matches ${num(total)} contracts; this page is past the end.`
                }
                action={{ href: href({ page: 1 }), label: "Back to page 1" }}
              />
            ) : (
              <>
                <div className="overflow-x-auto">
                  <table className="w-full text-xs">
                    <thead className="text-surface-muted">
                      <tr>
                        <th className="cell-left font-normal">
                          <Link href={sortLink("time_desc")} className="no-underline hover:text-slate-200">
                            Time {sort === "time_desc" ? "↓" : sort === "time_asc" ? "↑" : ""}
                          </Link>
                        </th>
                        <th className="cell-left font-normal">Symbol</th>
                        <th className="cell font-normal">Qty</th>
                        <th className="cell font-normal">Rate</th>
                        <th className="cell font-normal">
                          <Link href={sortLink("amount_desc")} className="no-underline hover:text-slate-200">
                            Amount {sort === "amount_desc" ? "↓" : sort === "amount_asc" ? "↑" : ""}
                          </Link>
                        </th>
                        <th className="cell-left font-normal">Buyer</th>
                        <th className="cell-left font-normal">Seller</th>
                        <th className="cell-left font-normal">Contract</th>
                      </tr>
                    </thead>
                    <tbody>
                      {rows.map((t) => (
                        <tr key={t.contract_id} className="row">
                          <td className="cell-left text-surface-muted">{timeOnly(t.executed_at)}</td>
                          <td className="cell-left">
                            <Link
                              href={`/companies/${t.symbol}`}
                              className="text-slate-200 no-underline hover:text-accent"
                            >
                              {t.symbol}
                            </Link>
                          </td>
                          <td className="cell">{num(t.quantity, "0")}</td>
                          <td className="cell">{num(t.rate)}</td>
                          <td className="cell">{money(t.amount)}</td>
                          <td className="cell-left text-surface-muted">{t.buyer_broker ?? "not disclosed"}</td>
                          <td className="cell-left text-surface-muted">{t.seller_broker ?? "not disclosed"}</td>
                          <td className="cell-left text-2xs text-surface-muted">{t.contract_id}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 text-xs">
                  <span className="text-surface-muted">
                    {num(first)}–{num(last)} of <span className="tnum text-slate-300">{num(total)}</span> ·
                    page value <span className="tnum text-slate-300">{money(pageAmount)}</span>
                  </span>
                  <span className="flex items-center gap-2">
                    {page > 1 ? (
                      <Link
                        href={href({ page: page - 1 })}
                        className="rounded border border-surface-line px-3 py-1 no-underline hover:border-accent/60 hover:text-accent"
                      >
                        ← Newer
                      </Link>
                    ) : (
                      <span className="rounded border border-surface-line/40 px-3 py-1 text-surface-muted">← Newer</span>
                    )}
                    <span className="text-surface-muted">
                      {page} / {num(pages)}
                    </span>
                    {page < pages ? (
                      <Link
                        href={href({ page: page + 1 })}
                        className="rounded border border-surface-line px-3 py-1 no-underline hover:border-accent/60 hover:text-accent"
                      >
                        Older →
                      </Link>
                    ) : (
                      <span className="rounded border border-surface-line/40 px-3 py-1 text-surface-muted">Older →</span>
                    )}
                  </span>
                </div>
              </>
            )}
          </Panel>

          <Panel
            title="Broker net flow"
            action={<span className="text-2xs text-surface-muted">{session || "no session"}</span>}
          >
            {brokers.data.brokers.length === 0 ? (
              <UnavailableState
                title="No member totals for this session"
                detail="Broker totals exist only once contracts carrying a member id have been ingested. Nothing is estimated from names."
              />
            ) : (
              <>
                <table className="w-full text-xs">
                  <thead className="text-surface-muted">
                    <tr>
                      <th className="cell-left font-normal">Member</th>
                      <th className="cell font-normal">Buy</th>
                      <th className="cell font-normal">Sell</th>
                      <th className="cell font-normal">Net</th>
                    </tr>
                  </thead>
                  <tbody>
                    {brokers.data.brokers.map((b) => (
                      <tr key={b.broker_code} className="row">
                        <td className="cell-left">
                          <Link
                            href={href({ broker: b.broker_code, side: "", page: 1 })}
                            className="text-slate-200 no-underline hover:text-accent"
                            title={b.broker_name ?? undefined}
                          >
                            {b.broker_code}
                          </Link>
                          <span className="ml-1 text-2xs text-surface-muted">{b.broker_name ?? ""}</span>
                        </td>
                        <td className="cell">{money(b.buy_turnover)}</td>
                        <td className="cell">{money(b.sell_turnover)}</td>
                        <td className={`cell ${Number(b.net_turnover) >= 0 ? "text-up" : "text-down"}`}>
                          {money(b.net_turnover)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                <p className="px-4 py-2 text-2xs leading-relaxed text-surface-muted">
                  {brokerCoverage
                    ? `${num(brokerCoverage.attributed)} of ${num(brokerCoverage.contracts)} contracts carry a member id; ${num(
                        brokerCoverage.unattributed,
                      )} are excluded from every total. ${num(brokerCoverage.brokers)} members seen.`
                    : "Coverage unavailable."}
                  {run ? ` Sweep: ${run.pages_present}/${run.pages_expected} pages, ${run.converged ? "converged" : "not converged"}.` : ""}
                </p>
              </>
            )}
          </Panel>
        </div>

        <Provenance
          source={trades.meta.source}
          asOf={trades.meta.as_of ? dateTime(trades.meta.as_of) : undefined}
          license={trades.meta.license_class}
        />
        <p className="px-1 text-2xs leading-relaxed text-surface-muted">
          Execution times are the exchange&apos;s own values, preserved at the precision supplied
          {sessionCoverage?.timestamp_precision ? ` (${sessionCoverage.timestamp_precision})` : ""} and stored in UTC;
          {sessionCoverage?.timezone_basis ? ` ${sessionCoverage.timezone_basis}.` : " no instant is invented where the source supplied none."}
        </p>
      </div>
    );
  } catch (err) {
    const e = err instanceof ApiError ? err : null;
    return (
      <ErrorState
        title="The floorsheet could not be loaded"
        detail={e ? `${e.message} (${e.code})` : "Unknown error."}
      />
    );
  }
}
