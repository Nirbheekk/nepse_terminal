"use client";

import { useState } from "react";
import Link from "next/link";
import { alerts } from "@/lib/api";
import { useAction, useResource } from "@/hooks/use-terminal";
import { ago, dateTime, money, num } from "@/lib/utils/format";
import { Badge, Notice, Panel, Provenance, UnavailableState } from "@/components/ui";
import type { AlertDelivery, AlertEvent } from "@/lib/schemas/workspace";

/**
 * Alerts.
 *
 * Three panels, because they answer different questions: the rules are the
 * intent, the event ledger is what actually matched (and how many times a
 * cooldown suppressed a repeat), and deliveries show whether the owner was told
 * and what the retry state is.
 */
const METRICS = ["close", "change_pct", "volume", "turnover", "market_cap", "free_float_pct"];
const COMPARATORS = [
  { key: "gte", label: "≥" },
  { key: "lte", label: "≤" },
  { key: "gt", label: ">" },
  { key: "lt", label: "<" },
  { key: "eq", label: "=" },
];

export default function AlertsPage() {
  const rules = useResource(alerts.list);
  const events = useResource(alerts.events);
  const deliveries = useResource(alerts.deliveries);
  const action = useAction();

  const [form, setForm] = useState({
    name: "",
    symbol: "",
    metric: "close",
    comparator: "gte",
    threshold: "",
    cooldown_seconds: 3600,
  });

  async function create() {
    if (!form.name.trim() || !form.symbol.trim() || !form.threshold.trim()) return;
    const ok = await action.run(() =>
      alerts.create({
        name: form.name.trim(),
        symbol: form.symbol.trim().toUpperCase(),
        metric: form.metric,
        comparator: form.comparator,
        threshold: form.threshold,
        cooldown_seconds: Number(form.cooldown_seconds) || 3600,
      }),
    );
    if (ok) {
      setForm({ ...form, name: "", symbol: "", threshold: "" });
      await refreshAll();
    }
  }

  async function evaluate() {
    await action.run(() => alerts.evaluate());
    await Promise.all([rules.refresh(), events.refresh(), deliveries.refresh()]);
  }

  // Deleting a rule cascades its events and deliveries, so every panel has to
  // be reloaded together; refreshing only the rules left orphan rows on screen
  // for events the database no longer held.
  async function refreshAll() {
    await Promise.all([rules.refresh(), events.refresh(), deliveries.refresh()]);
  }

  if (rules.unauthenticated) {
    return (
      <div className="space-y-4">
        <Heading />
        <Notice tone="warn" title="Sign in to manage alerts" detail="Alert rules belong to an account: the worker only evaluates rules whose owner is signed up, and every event is scoped to that owner." />
        <Link href="/login" className="inline-block rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/60 hover:text-accent">
          Sign in
        </Link>
      </div>
    );
  }

  const ruleList = rules.data ?? [];
  const eventList = events.data ?? [];
  const deliveryList = deliveries.data ?? [];

  return (
    <div className="space-y-5">
      <Heading />

      {/* A failed row action (pause, delete) previously rendered nothing at all,
          which looked like the click had been ignored. */}
      {action.error ? (
        <Notice tone="bad" title="The last action failed" detail={action.error} />
      ) : null}

      <Panel
        title="New alert rule"
        action={
          <button
            type="button"
            onClick={evaluate}
            disabled={action.busy}
            className="rounded border border-surface-line px-2.5 py-1 text-2xs text-slate-300 hover:border-accent/50 hover:text-accent disabled:opacity-50"
          >
            {action.busy ? "Evaluating…" : "Evaluate now"}
          </button>
        }
      >
        <div className="flex flex-wrap items-end gap-2 p-4">
          <label className="text-2xs uppercase tracking-widest text-surface-muted">
            Name
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="NABIL above 600"
              className="mt-1 block w-48 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200"
            />
          </label>
          <label className="text-2xs uppercase tracking-widest text-surface-muted">
            Symbol
            <input
              value={form.symbol}
              onChange={(e) => setForm({ ...form, symbol: e.target.value })}
              placeholder="NABIL"
              className="mt-1 block w-28 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs uppercase text-slate-200"
            />
          </label>
          <label className="text-2xs uppercase tracking-widest text-surface-muted">
            Metric
            <select
              value={form.metric}
              onChange={(e) => setForm({ ...form, metric: e.target.value })}
              className="mt-1 block w-36 rounded border border-surface-line bg-surface-raised px-2 py-1.5 text-xs text-slate-200"
            >
              {METRICS.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
          </label>
          <label className="text-2xs uppercase tracking-widest text-surface-muted">
            When
            <select
              value={form.comparator}
              onChange={(e) => setForm({ ...form, comparator: e.target.value })}
              className="mt-1 block w-20 rounded border border-surface-line bg-surface-raised px-2 py-1.5 text-xs text-slate-200"
            >
              {COMPARATORS.map((c) => (
                <option key={c.key} value={c.key}>
                  {c.label}
                </option>
              ))}
            </select>
          </label>
          <label className="text-2xs uppercase tracking-widest text-surface-muted">
            Threshold
            <input
              value={form.threshold}
              onChange={(e) => setForm({ ...form, threshold: e.target.value })}
              inputMode="decimal"
              placeholder="600"
              className="mt-1 block w-28 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200"
            />
          </label>
          <label className="text-2xs uppercase tracking-widest text-surface-muted">
            Cooldown
            <select
              value={form.cooldown_seconds}
              onChange={(e) => setForm({ ...form, cooldown_seconds: Number(e.target.value) })}
              className="mt-1 block w-32 rounded border border-surface-line bg-surface-raised px-2 py-1.5 text-xs text-slate-200"
            >
              <option value={0}>no cooldown</option>
              <option value={900}>15 minutes</option>
              <option value={3600}>1 hour</option>
              <option value={86400}>1 day</option>
            </select>
          </label>
          <button
            type="button"
            onClick={create}
            disabled={action.busy || !form.name.trim() || !form.symbol.trim() || !form.threshold.trim()}
            className="rounded border border-accent/50 bg-accent/10 px-4 py-1.5 text-xs font-medium text-accent disabled:opacity-50"
          >
            Create rule
          </button>
        </div>
      </Panel>

      <Panel title={`Rules${ruleList.length ? ` · ${ruleList.length}` : ""}`}>
        {ruleList.length === 0 ? (
          <UnavailableState
            title="No alert rules"
            detail="A rule watches one metric of one symbol against a threshold. The worker evaluates every enabled rule and records an event the first time the condition holds."
          />
        ) : (
          <ul className="divide-y divide-surface-line/60">
            {ruleList.map((r) => (
              <li key={r.id} className="flex flex-wrap items-center gap-3 px-4 py-2.5">
                <Badge tone={r.enabled ? "good" : "neutral"}>{r.enabled ? "enabled" : "paused"}</Badge>
                <span className="text-xs font-medium text-slate-200">{r.name}</span>
                <span className="font-mono text-2xs text-surface-muted">
                  {r.symbol ?? "*"} {r.metric} {COMPARATORS.find((c) => c.key === r.comparator)?.label ?? r.comparator}{" "}
                  {num(r.threshold)}
                </span>
                <span className="text-2xs text-surface-muted">
                  cooldown {r.cooldown_seconds === 0 ? "none" : `${Math.round(r.cooldown_seconds / 60)}m`} ·{" "}
                  {r.max_attempts} attempts
                </span>
                <div className="ml-auto flex items-center gap-2">
                  <button
                    type="button"
                    onClick={async () => {
                      await action.run(() => alerts.toggle(r.id, !r.enabled));
                      await rules.refresh();
                    }}
                    className="rounded border border-surface-line px-2 py-1 text-2xs text-slate-300 hover:border-accent/50 hover:text-accent"
                  >
                    {r.enabled ? "Pause" : "Enable"}
                  </button>
                  <button
                    type="button"
                    onClick={async () => {
                      await action.run(() => alerts.remove(r.id));
                      await refreshAll();
                    }}
                    className="rounded border border-surface-line px-2 py-1 text-2xs text-surface-muted hover:border-down/50 hover:text-down"
                  >
                    Delete
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <Panel title={`Matched events${eventList.length ? ` · ${eventList.length}` : ""}`}>
        {eventList.length === 0 ? (
          <p className="px-4 py-6 text-xs text-surface-muted">
            No condition has matched yet. Events are created by the worker, so a rule that never matched
            shows nothing here rather than a pending row.
          </p>
        ) : (
          <div className="overflow-auto">
            <table className="w-full border-collapse text-xs">
              <thead>
                <tr className="text-2xs uppercase tracking-wider text-surface-muted">
                  <Th>Rule</Th>
                  <Th>Symbol</Th>
                  <Th>Session</Th>
                  <Th right>Value</Th>
                  <Th>Condition</Th>
                  <Th right>Suppressed</Th>
                  <Th>First seen</Th>
                </tr>
              </thead>
              <tbody>
                {eventList.map((e: AlertEvent) => (
                  <tr key={e.id} className="border-b border-surface-line/60">
                    <Td>{e.rule_name}</Td>
                    <Td>
                      <Link href={`/stocks/${encodeURIComponent(e.symbol)}`} className="text-accent no-underline">
                        {e.symbol}
                      </Link>
                    </Td>
                    <Td muted>{e.session_date}</Td>
                    <Td right>{money(e.metric_value)}</Td>
                    <Td muted>
                      {e.metric} {e.comparator} {num(e.threshold)}
                    </Td>
                    <Td right>{e.suppressed_count}</Td>
                    <Td muted>{dateTime(e.first_seen_at)}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      <Panel title={`Delivery history${deliveryList.length ? ` · ${deliveryList.length}` : ""}`}>
        {deliveryList.length === 0 ? (
          <p className="px-4 py-6 text-xs text-surface-muted">
            No delivery attempts recorded. A delivery is written when an event is raised, and its status
            tracks retries until it succeeds or runs out of attempts.
          </p>
        ) : (
          <div className="overflow-auto">
            <table className="w-full border-collapse text-xs">
              <thead>
                <tr className="text-2xs uppercase tracking-wider text-surface-muted">
                  <Th>Rule</Th>
                  <Th>Symbol</Th>
                  <Th>Channel</Th>
                  <Th>Status</Th>
                  <Th right>Attempts</Th>
                  <Th>Next attempt</Th>
                  <Th>Delivered</Th>
                  <Th>Last error</Th>
                </tr>
              </thead>
              <tbody>
                {deliveryList.map((d: AlertDelivery) => (
                  <tr key={d.id} className="border-b border-surface-line/60">
                    <Td>{d.rule_name}</Td>
                    <Td>{d.symbol}</Td>
                    <Td muted>{d.channel}</Td>
                    <Td>
                      <Badge tone={d.status === "delivered" ? "good" : d.status === "failed" ? "bad" : "warn"}>
                        {d.status}
                      </Badge>
                    </Td>
                    <Td right>
                      {d.attempts}/{d.max_attempts}
                    </Td>
                    <Td muted>{d.next_attempt_at ? ago(d.next_attempt_at) : "—"}</Td>
                    <Td muted>{d.delivered_at ? dateTime(d.delivered_at) : "not yet"}</Td>
                    <Td muted>{d.last_error ? d.last_error.slice(0, 60) : "—"}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      <Provenance source="postgres:alert_rules + alert_events + alert_deliveries" license="internal" />
    </div>
  );
}

function Heading() {
  return (
    <div>
      <h1 className="text-xl font-semibold tracking-tight text-slate-100">Alerts</h1>
      <p className="mt-1 text-xs text-surface-muted">
        Rules are evaluated by the worker on each cycle. An event is de-duplicated per session and value,
        and a repeat inside the cooldown is counted rather than re-sent.
      </p>
    </div>
  );
}

function Th({ children, right = false }: { children: React.ReactNode; right?: boolean }) {
  return (
    <th
      className={`border-b border-surface-line px-3 py-2 font-medium ${right ? "text-right" : "text-left"}`}
    >
      {children}
    </th>
  );
}

function Td({
  children,
  right = false,
  muted = false,
}: {
  children: React.ReactNode;
  right?: boolean;
  muted?: boolean;
}) {
  return (
    <td
      className={`whitespace-nowrap px-3 py-1.5 ${right ? "text-right tnum" : ""} ${
        muted ? "text-surface-muted" : "text-slate-200"
      }`}
    >
      {children}
    </td>
  );
}
