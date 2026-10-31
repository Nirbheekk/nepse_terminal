import Link from "next/link";
import { api, ApiError, exportUrl } from "@/lib/api";
import { dateTime, money, num, pct, trendClass } from "@/lib/utils/format";
import type { SectorOption } from "@/lib/schemas";
import { ErrorState, FreshnessBadge, Panel, Provenance, UnavailableState } from "@/components/ui";

export const dynamic = "force-dynamic";

const PAGE_SIZE = 50;

const SORTS = [
  "symbol",
  "name",
  "sector",
  "close",
  "change",
  "volume",
  "turnover",
  "transactions",
  "market_cap",
  "float_market_cap",
] as const;

type Sort = (typeof SORTS)[number];

const INSTRUMENTS = [
  { value: "equity", label: "Equity" },
  { value: "non-convertible debentures", label: "Debentures" },
  { value: "mutual funds", label: "Mutual funds" },
  { value: "preference shares", label: "Preference shares" },
];

type SearchParams = Promise<Record<string, string | string[] | undefined>>;

type Filters = {
  q: string;
  sector: string;
  instrument: string;
  status: string;
  sort: Sort;
  order: "asc" | "desc";
};

function one(v: string | string[] | undefined): string {
  return (Array.isArray(v) ? v[0] : v) ?? "";
}

function buildHref(params: Record<string, string | number | undefined>): string {
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") qs.set(key, String(value));
  }
  const q = qs.toString();
  return q ? `/companies?${q}` : "/companies";
}

export default async function CompaniesPage({ searchParams }: { searchParams: SearchParams }) {
  const sp = await searchParams;

  const q = one(sp.q).trim().slice(0, 64);
  const sector = one(sp.sector).trim().toLowerCase();
  const instrument = one(sp.instrument).trim().toLowerCase();
  const status = one(sp.status).trim().toLowerCase();

  const sortParam = one(sp.sort).trim().toLowerCase();
  const sort: Sort = (SORTS as readonly string[]).includes(sortParam) ? (sortParam as Sort) : "symbol";
  const order = one(sp.order).trim().toLowerCase() === "desc" ? "desc" : "asc";

  const rawPage = Number.parseInt(one(sp.page), 10);
  const page = Number.isFinite(rawPage) && rawPage > 0 ? rawPage : 1;

  const filters: Filters = { q, sector, instrument, status, sort, order };

  let result;
  let sectors: SectorOption[] = [];
  try {
    const [companiesRes, sectorsRes] = await Promise.all([
      api.companies({ ...filters, limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE }),
      // The sector list is a filter convenience; losing it must not blank the page.
      api.sectors().catch(() => null),
    ]);
    result = companiesRes;
    sectors = sectorsRes?.data ?? [];
  } catch (err) {
    const e = err instanceof ApiError ? err : null;
    return (
      <ErrorState
        title={e?.code === "API_UNREACHABLE" ? "The market API is not reachable" : "The company directory could not be loaded"}
        detail={
          e?.code === "API_UNREACHABLE"
            ? `${e.message} Start the Go API, then reload. No cached directory is shown.`
            : `${e?.message ?? "Unknown error"} (${e?.code ?? "UNKNOWN"})`
        }
      />
    );
  }

  const { data, meta } = result;
  const total = meta.total ?? data.length;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const currentPage = Math.min(page, pages);
  const first = total === 0 ? 0 : (currentPage - 1) * PAGE_SIZE + 1;
  const last = Math.min(currentPage * PAGE_SIZE, total);

  const exportHref = exportUrl.companies({ ...filters, limit: 5000, offset: 0 });

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold tracking-tight text-slate-100">Listed companies</h1>
          <p className="text-xs text-surface-muted">
            {total} listed {total === 1 ? "company" : "companies"} · showing {first}–{last}
            {q ? ` matching “${q}”` : ""}
            {sector ? ` · ${sector.replace(/_/g, " ")}` : ""}
            {status ? ` · ${status}` : ""}
          </p>
        </div>
        <a
          href={exportHref}
          className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/60 hover:text-accent"
        >
          Export CSV
        </a>
      </div>

      <form
        action="/companies"
        className="panel flex flex-wrap items-end gap-2 px-4 py-3 text-xs"
        aria-label="Filter companies"
      >
        <input type="hidden" name="sort" value={sort} />
        <input type="hidden" name="order" value={order} />
        <label className="flex flex-col gap-1">
          <span className="text-2xs uppercase tracking-widest text-surface-muted">Search</span>
          <input
            type="search"
            name="q"
            defaultValue={q}
            placeholder="Symbol or name"
            className="w-52 rounded border border-surface-line bg-surface-base px-2.5 py-1.5 text-xs text-slate-200 placeholder:text-surface-muted focus:border-accent/60 focus:outline-none"
          />
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-2xs uppercase tracking-widest text-surface-muted">Sector</span>
          <select
            name="sector"
            defaultValue={sector}
            className="w-48 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200 focus:border-accent/60 focus:outline-none"
          >
            <option value="">All sectors</option>
            {sectors.map((s) => (
              <option key={s.code} value={s.code}>
                {s.name} ({s.companies})
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-2xs uppercase tracking-widest text-surface-muted">Instrument</span>
          <select
            name="instrument"
            defaultValue={instrument}
            className="w-44 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200 focus:border-accent/60 focus:outline-none"
          >
            <option value="">All instruments</option>
            {INSTRUMENTS.map((i) => (
              <option key={i.value} value={i.value}>
                {i.label}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-2xs uppercase tracking-widest text-surface-muted">Status</span>
          <select
            name="status"
            defaultValue={status}
            className="w-36 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200 focus:border-accent/60 focus:outline-none"
          >
            <option value="">Any status</option>
            <option value="active">Active</option>
            <option value="inactive">Inactive</option>
          </select>
        </label>
        <button
          type="submit"
          className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 hover:border-accent/60 hover:text-accent"
        >
          Apply filters
        </button>
        <Link href="/companies" className="px-1 py-1.5 text-xs text-surface-muted no-underline hover:text-accent">
          Reset
        </Link>
      </form>

      <Panel
        title="All listed companies"
        action={
          <span className="text-2xs text-surface-muted">
            <FreshnessBadge freshness={meta.freshness} reason={meta.freshness_reason} />
          </span>
        }
      >
        {data.length === 0 ? (
          <UnavailableState
            title="No company matched"
            detail="Try a shorter query, choose another sector, or clear the filters to list every symbol."
            action={{ href: "/companies", label: "Clear filters" }}
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead className="text-surface-muted">
                <tr>
                  <SortHeader label="Symbol" field="symbol" sort={sort} order={order} filters={filters} />
                  <SortHeader label="Name" field="name" sort={sort} order={order} filters={filters} align="left" />
                  <SortHeader label="Sector" field="sector" sort={sort} order={order} filters={filters} align="left" />
                  <SortHeader label="LTP" field="close" sort={sort} order={order} filters={filters} />
                  <SortHeader label="Chg %" field="change" sort={sort} order={order} filters={filters} />
                  <SortHeader label="Quantity" field="volume" sort={sort} order={order} filters={filters} />
                  <SortHeader label="Turnover" field="turnover" sort={sort} order={order} filters={filters} />
                  <SortHeader label="Txns" field="transactions" sort={sort} order={order} filters={filters} />
                  <SortHeader label="Market cap" field="market_cap" sort={sort} order={order} filters={filters} />
                  <SortHeader
                    label="Float cap"
                    field="float_market_cap"
                    sort={sort}
                    order={order}
                    filters={filters}
                  />
                  <th className="cell-left font-normal">Session</th>
                </tr>
              </thead>
              <tbody>
                {data.map((c) => (
                  <tr key={c.symbol} className="row">
                    <td className="cell-left">
                      <Link href={`/companies/${c.symbol}`} className="text-slate-200 no-underline hover:text-accent">
                        {c.symbol}
                      </Link>
                    </td>
                    <td className="cell-left max-w-[18rem] truncate text-slate-300" title={c.name}>
                      {c.name}
                    </td>
                    <td className="cell-left text-surface-muted">{c.sector}</td>					<td className="cell">{c.trading_date ? num(c.close) : "—"}</td>
					<td className={`cell ${c.trading_date ? trendClass(c.percent_change) : ""}`}>
					  {c.trading_date ? pct(c.percent_change) : "—"}
					</td>
					<td className="cell">{c.trading_date ? num(c.volume, "0") : "—"}</td>
					<td className="cell">
					  {c.trading_date && Number(c.turnover) > 0 ? money(c.turnover) : "—"}
					</td>
					<td className="cell">{c.trading_date ? num(c.transactions, "0") : "—"}</td>
                    <td className="cell" title={c.market_cap_method}>
                      {c.market_cap ? money(c.market_cap) : "—"}
                    </td>
                    <td className="cell" title={c.float_market_cap_method}>
                      {c.float_market_cap ? money(c.float_market_cap) : "—"}
                    </td>
                    <td className="cell-left text-2xs text-surface-muted">{c.trading_date || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      <div className="flex flex-wrap items-center justify-between gap-3 text-xs">
        <p className="text-surface-muted">
          Page {currentPage} of {pages}
          {meta.coverage
            ? ` · capitalisation covers ${meta.coverage.covered} of ${meta.coverage.eligible} companies`
            : ""}
        </p>
        <Pagination page={currentPage} pages={pages} filters={filters} />
      </div>

      <Provenance
        source={meta.source}
        asOf={meta.as_of ? dateTime(meta.as_of) : undefined}
        license={meta.license_class}
      />
      <p className="px-1 text-2xs leading-relaxed text-surface-muted">
        {meta.freshness_reason ?? "Freshness not reported."} Session{" "}
        {meta.session_date ?? data[0]?.trading_date ?? "unknown"}. Market capitalisation is the close
        multiplied by listed shares; the exchange&apos;s own figure is computed at the previous close and
        is kept on each company page.
      </p>
    </div>
  );
}

function SortHeader({
  label,
  field,
  sort,
  order,
  filters,
  align = "right",
}: {
  label: string;
  field: Sort;
  sort: Sort;
  order: "asc" | "desc";
  filters: Filters;
  align?: "left" | "right";
}) {
  const active = sort === field;
  const nextOrder = active && order === "asc" ? "desc" : "asc";
  const href = buildHref({ ...filters, sort: field, order: nextOrder });
  return (
    <th className={`${align === "left" ? "cell-left" : "cell"} font-normal`}>
      <Link
        href={href}
        className={`no-underline hover:text-accent ${active ? "text-slate-200" : "text-surface-muted"}`}
        title={`Sort by ${label} ${nextOrder === "asc" ? "ascending" : "descending"}`}
      >
        {label}
        {active ? (order === "asc" ? " ↑" : " ↓") : ""}
      </Link>
    </th>
  );
}

function Pagination({
  page,
  pages,
  filters,
}: {
  page: number;
  pages: number;
  filters: Filters;
}) {
  if (pages <= 1) return <span className="text-surface-muted">All results on one page</span>;

  const window: number[] = [];
  for (let p = Math.max(1, page - 2); p <= Math.min(pages, page + 2); p++) window.push(p);
  const firstWindow = window[0] ?? page;
  const lastWindow = window.at(-1) ?? page;

  const link = "rounded border border-surface-line px-2.5 py-1 text-xs no-underline hover:border-accent/60 hover:text-accent";

  return (
    <nav className="flex items-center gap-1.5" aria-label="Pagination">
      {page > 1 ? (
        <Link href={buildHref({ ...filters, page: page - 1 })} className={link}>
          ← Prev
        </Link>
      ) : (
        <span className={`${link} border-surface-line/40 text-surface-muted/50`}>← Prev</span>
      )}
      {firstWindow > 1 ? (
        <>
          <Link href={buildHref({ ...filters, page: 1 })} className={link}>
            1
          </Link>
          {firstWindow > 2 ? <span className="text-surface-muted">…</span> : null}
        </>
      ) : null}
      {window.map((p) => (
        <Link
          key={p}
          href={buildHref({ ...filters, page: p })}
          className={`${link} ${p === page ? "border-accent/60 text-accent" : "text-slate-300"}`}
          aria-current={p === page ? "page" : undefined}
        >
          {p}
        </Link>
      ))}
      {lastWindow < pages ? (
        <>
          {lastWindow < pages - 1 ? <span className="text-surface-muted">…</span> : null}
          <Link href={buildHref({ ...filters, page: pages })} className={link}>
            {pages}
          </Link>
        </>
      ) : null}
      {page < pages ? (
        <Link href={buildHref({ ...filters, page: page + 1 })} className={link}>
          Next →
        </Link>
      ) : (
        <span className={`${link} border-surface-line/40 text-surface-muted/50`}>Next →</span>
      )}
    </nav>
  );
}
