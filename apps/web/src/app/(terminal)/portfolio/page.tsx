"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { portfolios, ApiError } from "@/lib/api";
import { useAction, useResource } from "@/hooks/use-terminal";
import { money, num, pct } from "@/lib/utils/format";
import { Badge, Notice, Panel, Provenance, UnavailableState } from "@/components/ui";
import type { Portfolio, PortfolioAnalytics } from "@/lib/schemas/workspace";

/**
 * Portfolios.
 *
 * Analytics are loaded only for the selected portfolio, because the API refuses
 * to compute risk metrics from fewer than two daily observations — showing that
 * refusal as the explanation is more useful than an empty metrics grid.
 */
export default function PortfolioPage() {
  const list = useResource(portfolios.list);
  const action = useAction();

  const [name, setName] = useState("");
  const [selected, setSelected] = useState<string>("");
  const [analytics, setAnalytics] = useState<PortfolioAnalytics | null>(null);
  const [analyticsError, setAnalyticsError] = useState<string | null>(null);
  const [loadingAnalytics, setLoadingAnalytics] = useState(false);

  const loadAnalytics = useCallback(async (id: string) => {
    setLoadingAnalytics(true);
    setAnalyticsError(null);
    try {
      const res = await portfolios.analytics(id);
      setAnalytics(res.data);
    } catch (err) {
      setAnalytics(null);
      const e = err instanceof ApiError ? err : null;
      setAnalyticsError(
        e?.status === 422
          ? "Risk analytics need at least two daily observations for this portfolio. Record transactions and cash flows across two or more sessions, then reload."
          : (e?.message ?? "Analytics unavailable"),
      );
    } finally {
      setLoadingAnalytics(false);
    }
  }, []);

  useEffect(() => {
    if (selected) void loadAnalytics(selected);
  }, [selected, loadAnalytics]);

  async function create() {
    if (!name.trim()) return;
    const ok = await action.run(() => portfolios.create({ name: name.trim() }));
    if (ok) {
      setName("");
      await list.refresh();
    }
  }

  if (list.unauthenticated) {
    return (
      <div className="space-y-4">
        <Heading />
        <Notice tone="warn" title="Sign in to see portfolios" detail="Positions, lots and analytics are owned by an account and never shared between tenants." />
        <Link href="/login" className="inline-block rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/60 hover:text-accent">
          Sign in
        </Link>
      </div>
    );
  }

  const items: Portfolio[] = list.data ?? [];
  const perf = analytics?.performance;

  return (
    <div className="space-y-5">
      <Heading />

      <Panel title="New portfolio">
        <div className="flex flex-wrap items-end gap-2 p-4">
          <label className="text-2xs uppercase tracking-widest text-surface-muted">
            Name
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void create();
              }}
              placeholder="Long-term holdings"
              className="mt-1 block w-64 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200"
            />
          </label>
          <button
            type="button"
            onClick={create}
            disabled={action.busy || !name.trim()}
            className="rounded border border-accent/50 bg-accent/10 px-4 py-1.5 text-xs font-medium text-accent disabled:opacity-50"
          >
            {action.busy ? "Creating…" : "Create portfolio"}
          </button>
          {action.error ? <span className="text-2xs text-down">{action.error}</span> : null}
        </div>
      </Panel>

      {list.error && !list.unauthenticated ? (
        <Notice tone="bad" title="Portfolios could not be loaded" detail={list.error} />
      ) : null}

      {items.length === 0 && !list.loading ? (
        <Panel title="No portfolios">
          <UnavailableState
            title="Nothing tracked yet"
            detail="Create a portfolio above, then record buys and sells through the API. Positions, weighted average cost and realised P&L are derived from those transactions — nothing is estimated."
          />
        </Panel>
      ) : null}

      {items.length > 0 ? (
        <Panel title={`Portfolios · ${items.length}`}>
          <ul className="divide-y divide-surface-line/60">
            {items.map((p) => (
              <li key={p.id} className="flex flex-wrap items-center gap-3 px-4 py-2.5">
                <button
                  type="button"
                  onClick={() => setSelected(p.id)}
                  className={`text-xs font-medium ${
                    selected === p.id ? "text-accent" : "text-slate-200 hover:text-accent"
                  }`}
                >
                  {p.name}
                </button>
                <Badge tone="neutral">{p.cost_basis_method}</Badge>
                <span className="text-2xs text-surface-muted">cash {money(p.cash_balance)}</span>
                <span className="ml-auto flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => setSelected(p.id)}
                    className="rounded border border-surface-line px-2 py-1 text-2xs text-slate-300 hover:border-accent/50 hover:text-accent"
                  >
                    Analytics
                  </button>
                  <button
                    type="button"
                    onClick={async () => {
                      await action.run(() => portfolios.destroy(p.id));
                      if (selected === p.id) {
                        setSelected("");
                        setAnalytics(null);
                      }
                      await list.refresh();
                    }}
                    className="rounded border border-surface-line px-2 py-1 text-2xs text-surface-muted hover:border-down/50 hover:text-down"
                  >
                    Delete
                  </button>
                </span>
              </li>
            ))}
          </ul>
        </Panel>
      ) : null}

      {selected ? (
        <Panel
          title={`Analytics · ${items.find((p) => p.id === selected)?.name ?? selected}`}
          action={loadingAnalytics ? <span className="text-2xs text-surface-muted">loading…</span> : null}
        >
          {analyticsError ? (
            <div className="p-4">
              <Notice tone="warn" title="Risk analytics not available" detail={analyticsError} />
            </div>
          ) : !analytics ? (
            <p className="px-4 py-6 text-xs text-surface-muted">Loading analytics…</p>
          ) : (
            <div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-4">
              <Metric label="Time-weighted return" value={pct(perf?.portfolio_twr)} />
              <Metric label="Annualised (CAGR)" value={pct(perf?.portfolio_cagr)} />
              <Metric label="Benchmark" value={pct(perf?.benchmark_cagr)} />
              <Metric label="Excess return" value={pct(perf?.excess_return)} />
              <Metric label="Volatility" value={pct(analytics.risk?.portfolio_volatility as string)} />
              <Metric label="Beta" value={num(analytics.risk?.beta as string, "—")} />
              <Metric label="Sharpe" value={num(analytics.risk?.sharpe_ratio as string, "—")} />
              <Metric label="Max drawdown" value={pct(analytics.drawdown?.max_drawdown as string)} />
            </div>
          )}
        </Panel>
      ) : null}

      <Provenance source="postgres:portfolios + postgres:portfolio_transactions" license="internal" />
    </div>
  );
}

function Heading() {
  return (
    <div>
      <h1 className="text-xl font-semibold tracking-tight text-slate-100">Portfolio</h1>
      <p className="mt-1 text-xs text-surface-muted">
        Positions and returns computed from recorded transactions, with the NEPSE fee schedule applied to
        each trade.
      </p>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-surface-line px-3 py-2">
      <div className="text-2xs uppercase tracking-widest text-surface-muted">{label}</div>
      <div className="mt-0.5 text-sm font-semibold tnum text-slate-100">{value}</div>
    </div>
  );
}
