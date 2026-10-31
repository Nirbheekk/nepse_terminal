"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { screener, type ScreenRule } from "@/lib/api";
import { useAction, useResource } from "@/hooks/use-terminal";
import { compactNumber, money, num, pct, signed, trendClass } from "@/lib/utils/format";
import { Badge, Notice, Panel, Provenance, UnavailableState } from "@/components/ui";
import { DataTable, type Column } from "@/components/tables/data-table";
import type { ScreenerRow } from "@/lib/schemas/workspace";

type Draft = { metric: string; op: string; value: string };

const OPS = [
  { key: "gte", label: "≥ at least" },
  { key: "lte", label: "≤ at most" },
  { key: "gt", label: "> above" },
  { key: "lt", label: "< below" },
  { key: "eq", label: "= equal to" },
];

const SORTABLE = ["market_cap", "close", "change_pct", "volume", "turnover", "free_float_pct"];

export default function ScreenerPage() {
  const meta = useResource(screener.metrics);
  const saved = useResource(screener.screens);
  const run = useAction();
  const save = useAction();

  const [filters, setFilters] = useState<Draft[]>([]);
  const [draft, setDraft] = useState<Draft>({ metric: "market_cap", op: "gte", value: "" });
  const [sort, setSort] = useState("market_cap");
  const [rows, setRows] = useState<ScreenerRow[] | null>(null);
  const [coverage, setCoverage] = useState<Awaited<ReturnType<typeof screener.run>>["data"]["coverage"] | null>(null);
  const [saveName, setSaveName] = useState("");

  const metrics = meta.data?.metrics ?? [];
  const label = useMemo(() => {
    const map = new Map(metrics.map((m) => [m.key, m]));
    return (key: string) => map.get(key)?.label ?? key;
  }, [metrics]);

  // Technical metrics need history. Saying so before the run is more useful than
  // returning an empty table after it.
  const needsHistory = filters.some((f) => metrics.find((m) => m.key === f.metric)?.min_bars);
  const historyStarved = needsHistory && (meta.data?.coverage.with_20_sessions ?? 0) === 0;

  const rule: ScreenRule = {
    all: filters.map((f) => ({ metric: f.metric, op: f.op, value: f.value })),
    sort,
    order: "desc",
    limit: 100,
  };

  function addFilter() {
    if (!draft.metric || !draft.value.trim()) return;
    setFilters((prev) => [...prev, draft]);
    setDraft({ metric: draft.metric, op: draft.op, value: "" });
  }

  async function execute() {
    const res = await run.run(() => screener.run(rule));
    if (res) {
      setRows(res.data.rows);
      setCoverage(res.data.coverage);
    }
  }

  async function persist() {
    if (!saveName.trim()) return;
    const res = await save.run(() => screener.save({ name: saveName.trim(), definition: rule }));
    if (res) {
      setSaveName("");
      await saved.refresh();
    }
  }

  const columns: Column<ScreenerRow>[] = [
    {
      key: "symbol",
      label: "Symbol",
      render: (r) => (
        <Link href={`/stocks/${encodeURIComponent(r.symbol)}`} className="font-medium text-accent no-underline">
          {r.symbol}
        </Link>
      ),
      sortValue: (r) => r.symbol,
      defaultDesc: false,
    },
    {
      key: "name",
      label: "Company",
      render: (r) => <span className="text-surface-muted">{r.name}</span>,
      sortValue: (r) => r.name,
      defaultDesc: false,
    },
    { key: "sector", label: "Sector", render: (r) => r.sector, sortValue: (r) => r.sector, defaultDesc: false },
    {
      key: "close",
      label: "Close",
      align: "right",
      render: (r) => money(r.close),
      sortValue: (r) => r.close,
    },
    {
      key: "change_pct",
      label: "Change",
      align: "right",
      render: (r) => <span className={trendClass(r.change_pct)}>{pct(r.change_pct)}</span>,
      sortValue: (r) => r.change_pct,
    },
    { key: "volume", label: "Volume", align: "right", render: (r) => compactNumber(r.volume), sortValue: (r) => r.volume },
    {
      key: "turnover",
      label: "Turnover",
      align: "right",
      render: (r) => compactNumber(r.turnover),
      sortValue: (r) => r.turnover,
    },
    {
      key: "market_cap",
      label: "Market cap",
      align: "right",
      render: (r) => compactNumber(r.market_cap),
      sortValue: (r) => r.market_cap,
    },
    {
      key: "free_float_pct",
      label: "Free float",
      align: "right",
      render: (r) => (r.free_float_pct ? `${num(r.free_float_pct, "—")}%` : "—"),
      sortValue: (r) => r.free_float_pct,
    },
    {
      key: "bars",
      label: "Sessions",
      align: "right",
      render: (r) => (
        <span title={`${r.bars} stored bars ending ${r.trading_date}`} className="text-surface-muted tnum">
          {r.bars}
        </span>
      ),
      sortValue: (r) => r.bars,
    },
  ];

  if (meta.unauthenticated) {
    return (
      <div className="space-y-4">
        <Heading />
        <Notice
          tone="warn"
          title="Sign in to use the screener"
          detail="Screening runs against the stored session and is scoped to your account so saved rules stay yours."
        />
        <Link href="/login" className="inline-block rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/60 hover:text-accent">
          Sign in
        </Link>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <Heading />

      {meta.error ? <Notice tone="bad" title="Screener metadata unavailable" detail={meta.error} /> : null}

      {meta.data ? (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
          <Cov label="Securities" value={num(meta.data.coverage.securities)} />
          <Cov label="With 20 sessions" value={num(meta.data.coverage.with_20_sessions)} />
          <Cov label="With float cap" value={num(meta.data.coverage.with_float_cap)} />
          <Cov label="Latest session" value={meta.data.coverage.latest_session} />
        </div>
      ) : null}

      {historyStarved ? (
        <Notice
          tone="warn"
          title="Technical filters have no history to read"
          detail={`The database holds at most ${meta.data?.coverage.max_sessions ?? 0} sessions per security. A 20-period average needs 20. Price, volume and fundamental filters work today; technical ones return nothing until daily history is backfilled.`}
        />
      ) : null}

      <Panel title="Build a rule">
        <div className="space-y-3 p-4">
          <div className="flex flex-wrap items-end gap-2">
            <label className="text-2xs uppercase tracking-widest text-surface-muted">
              Metric
              <select
                value={draft.metric}
                onChange={(e) => setDraft({ ...draft, metric: e.target.value })}
                className="mt-1 block w-56 rounded border border-surface-line bg-surface-raised px-2 py-1.5 text-xs text-slate-200"
              >
                {["price", "volume", "fundamental", "technical"].map((kind) => {
                  const group = metrics.filter((m) => m.kind === kind);
                  if (group.length === 0) return null;
                  return (
                    <optgroup key={kind} label={kind}>
                      {group.map((m) => (
                        <option key={m.key} value={m.key}>
                          {m.label}
                        </option>
                      ))}
                    </optgroup>
                  );
                })}
              </select>
            </label>
            <label className="text-2xs uppercase tracking-widest text-surface-muted">
              Condition
              <select
                value={draft.op}
                onChange={(e) => setDraft({ ...draft, op: e.target.value })}
                className="mt-1 block w-32 rounded border border-surface-line bg-surface-raised px-2 py-1.5 text-xs text-slate-200"
              >
                {OPS.map((o) => (
                  <option key={o.key} value={o.key}>
                    {o.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="text-2xs uppercase tracking-widest text-surface-muted">
              Value
              <input
                value={draft.value}
                onChange={(e) => setDraft({ ...draft, value: e.target.value })}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addFilter();
                  }
                }}
                inputMode="decimal"
                placeholder="e.g. 1000000"
                className="mt-1 block w-32 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200"
              />
            </label>
            <button
              type="button"
              onClick={addFilter}
              className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 hover:border-accent/60 hover:text-accent"
            >
              Add condition
            </button>
          </div>

          {filters.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {filters.map((f, i) => (
                <span
                  key={`${f.metric}-${i}`}
                  className="inline-flex items-center gap-2 rounded border border-surface-line bg-surface-raised px-2 py-1 text-2xs text-slate-200"
                >
                  {label(f.metric)} {OPS.find((o) => o.key === f.op)?.label.split(" ")[0]} {f.value}
                  <button
                    type="button"
                    aria-label={`Remove ${label(f.metric)} filter`}
                    onClick={() => setFilters((prev) => prev.filter((_, j) => j !== i))}
                    className="text-surface-muted hover:text-down"
                  >
                    ×
                  </button>
                </span>
              ))}
            </div>
          ) : (
            <p className="text-2xs text-surface-muted">
              No conditions yet — running now returns every security, sorted by the chosen column.
            </p>
          )}

          <div className="flex flex-wrap items-end gap-2 border-t border-surface-line pt-3">
            <label className="text-2xs uppercase tracking-widest text-surface-muted">
              Sort by
              <select
                value={sort}
                onChange={(e) => setSort(e.target.value)}
                className="mt-1 block w-44 rounded border border-surface-line bg-surface-raised px-2 py-1.5 text-xs text-slate-200"
              >
                {SORTABLE.map((k) => (
                  <option key={k} value={k}>
                    {label(k)}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              onClick={execute}
              disabled={run.busy}
              className="rounded border border-accent/50 bg-accent/10 px-4 py-1.5 text-xs font-medium text-accent disabled:opacity-50"
            >
              {run.busy ? "Running…" : "Run screen"}
            </button>

            <div className="ml-auto flex items-end gap-2">
              <label className="text-2xs uppercase tracking-widest text-surface-muted">
                Save as
                <input
                  value={saveName}
                  onChange={(e) => setSaveName(e.target.value)}
                  placeholder="Rule name"
                  className="mt-1 block w-44 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200"
                />
              </label>
              <button
                type="button"
                onClick={persist}
                disabled={save.busy || !saveName.trim()}
                className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 hover:border-accent/60 hover:text-accent disabled:opacity-50"
              >
                {save.busy ? "Saving…" : "Save rule"}
              </button>
            </div>
          </div>

          {run.error ? <Notice tone="bad" title="The screen was rejected" detail={run.error} /> : null}
          {save.error ? <Notice tone="bad" title="The rule was not saved" detail={save.error} /> : null}
        </div>
      </Panel>

      <Panel
        title={rows === null ? "Results" : `Results · ${rows.length} of ${coverage?.securities ?? 0} securities`}
        action={
          coverage ? (
            <span className="text-2xs text-surface-muted">
              {coverage.latest_session} · max {coverage.max_sessions} sessions
            </span>
          ) : null
        }
      >
        <DataTable
          rows={rows ?? []}
          columns={columns}
          rowKey={(r) => r.security_id}
          empty={
            rows === null ? (
              <UnavailableState
                title="No screen has been run"
                detail="Add conditions and run the screen. Every result carries its own session count so a thin history is visible rather than hidden behind a single number."
              />
            ) : (
              <UnavailableState
                title="No security matched"
                detail="That is a real answer, not an error: the filters excluded every security with a stored observation. Loosen a condition, or check the coverage figures above for the dataset the filter depends on."
              />
            )
          }
        />
      </Panel>

      <Panel title={`Saved rules${saved.data ? ` · ${saved.data.length}` : ""}`}>
        {saved.unauthenticated ? (
          <p className="px-4 py-6 text-xs text-surface-muted">Sign in to keep saved rules.</p>
        ) : (saved.data ?? []).length === 0 ? (
          <p className="px-4 py-6 text-xs text-surface-muted">
            No saved rules yet. Build one above and give it a name.
          </p>
        ) : (
          <ul className="divide-y divide-surface-line/60">
            {(saved.data ?? []).map((s) => {
              const def = s.definition as ScreenRule | null;
              const count = def?.all?.length ?? 0;
              return (
                <li key={s.id} className="flex flex-wrap items-center gap-3 px-4 py-2.5">
                  <span className="text-xs font-medium text-slate-200">{s.name}</span>
                  <Badge tone="neutral">{count} condition{count === 1 ? "" : "s"}</Badge>
                  {def?.sort ? (
                    <span className="text-2xs text-surface-muted">sorted by {label(def.sort)}</span>
                  ) : null}
                  <div className="ml-auto flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => {
                        setFilters((def?.all ?? []).map((c) => ({ metric: c.metric, op: c.op, value: String(c.value) })));
                        setSort(def?.sort ?? "market_cap");
                      }}
                      className="rounded border border-surface-line px-2 py-1 text-2xs text-slate-300 hover:border-accent/50 hover:text-accent"
                    >
                      Load
                    </button>
                    <button
                      type="button"
                      onClick={async () => {
                        await save.run(() => screener.remove(s.id));
                        await saved.refresh();
                      }}
                      className="rounded border border-surface-line px-2 py-1 text-2xs text-surface-muted hover:border-down/50 hover:text-down"
                    >
                      Delete
                    </button>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </Panel>

      <Provenance source="postgres:daily_ohlcv + postgres:companies" license="public_unverified" />
    </div>
  );
}

function Heading() {
  return (
    <div>
      <h1 className="text-xl font-semibold tracking-tight text-slate-100">Screener</h1>
      <p className="mt-1 text-xs text-surface-muted">
        Filter the stored session by price, volume, fundamentals and technicals, then keep the rule.
        Alert rules can watch the same definition.
      </p>
    </div>
  );
}

function Cov({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-surface-line px-3 py-2">
      <div className="text-2xs uppercase tracking-widest text-surface-muted">{label}</div>
      <div className="mt-0.5 text-sm font-semibold tnum text-slate-100">{value}</div>
    </div>
  );
}
