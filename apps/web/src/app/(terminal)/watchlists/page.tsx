"use client";

import { useState } from "react";
import Link from "next/link";
import { watchlists } from "@/lib/api";
import { useAction, useResource } from "@/hooks/use-terminal";
import { compactNumber, money, pct, trendClass } from "@/lib/utils/format";
import { Badge, Notice, Panel, Provenance, UnavailableState } from "@/components/ui";
import { DataTable, type Column } from "@/components/tables/data-table";
import type { WatchlistItem } from "@/lib/schemas/workspace";

/**
 * Watchlists.
 *
 * Each list is loaded with its items, so the quote shown is the one stored for
 * that symbol at the last session — not a live tick. A symbol with no stored
 * session shows a dash rather than a zero.
 */
export default function WatchlistsPage() {
  const lists = useResource(watchlists.list);
  const action = useAction();

  const [newName, setNewName] = useState("");
  const [target, setTarget] = useState<Record<string, string>>({});

  async function create() {
    if (!newName.trim()) return;
    const ok = await action.run(() => watchlists.create(newName.trim()));
    if (ok) {
      setNewName("");
      await lists.refresh();
    }
  }

  async function addSymbol(listId: string) {
    const symbol = (target[listId] ?? "").trim().toUpperCase();
    if (!symbol) return;
    const ok = await action.run(() => watchlists.add(listId, symbol));
    if (ok) {
      setTarget((prev) => ({ ...prev, [listId]: "" }));
      await lists.refresh();
    }
  }

  // Built per list because removing an item needs the id of the list it
  // belongs to, and a stale closure here would delete from the wrong one.
  const columnsFor = (listId: string): Column<WatchlistItem>[] => [
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
    { key: "name", label: "Company", render: (r) => <span className="text-surface-muted">{r.name}</span> },
    { key: "close", label: "Last close", align: "right", render: (r) => money(r.close), sortValue: (r) => r.close },
    {
      key: "change_pct",
      label: "Change",
      align: "right",
      render: (r) => <span className={trendClass(r.change_pct)}>{pct(r.change_pct)}</span>,
      sortValue: (r) => r.change_pct,
    },
    { key: "volume", label: "Volume", align: "right", render: (r) => compactNumber(r.volume), sortValue: (r) => r.volume },
    {
      key: "note",
      label: "Note",
      render: (r) => <span className="text-surface-muted">{r.note || "—"}</span>,
    },
    {
      key: "actions",
      label: "",
      align: "right",
      render: (r) => (
        <button
          type="button"
          onClick={async () => {
            await action.run(() => watchlists.remove(listId, r.symbol));
            await lists.refresh();
          }}
          className="text-2xs text-surface-muted hover:text-down"
        >
          remove
        </button>
      ),
    },
  ];

  if (lists.unauthenticated) {
    return (
      <div className="space-y-4">
        <Heading />
        <Notice tone="warn" title="Sign in to keep watchlists" detail="Lists are stored against your account, so they follow you across devices." />
        <Link href="/login" className="inline-block rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/60 hover:text-accent">
          Sign in
        </Link>
      </div>
    );
  }

  const all = lists.data ?? [];

  return (
    <div className="space-y-5">
      <Heading />

      <Panel title="New list">
        <div className="flex flex-wrap items-end gap-2 p-4">
          <label className="text-2xs uppercase tracking-widest text-surface-muted">
            Name
            <input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void create();
              }}
              placeholder="e.g. Banks to watch"
              className="mt-1 block w-64 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs text-slate-200"
            />
          </label>
          <button
            type="button"
            onClick={create}
            disabled={action.busy || !newName.trim()}
            className="rounded border border-accent/50 bg-accent/10 px-4 py-1.5 text-xs font-medium text-accent disabled:opacity-50"
          >
            {action.busy ? "Creating…" : "Create list"}
          </button>
          {action.error ? <span className="text-2xs text-down">{action.error}</span> : null}
        </div>
      </Panel>

      {lists.error && !lists.unauthenticated ? (
        <Notice tone="bad" title="Watchlists could not be loaded" detail={lists.error} />
      ) : null}

      {all.length === 0 && !lists.loading ? (
        <Panel title="No lists yet">
          <UnavailableState title="Nothing tracked" detail="Create a list, add a symbol, and its last stored close will appear here after the next session is captured." />
        </Panel>
      ) : null}

      {all.map((list) => (
        <Panel
          key={list.id}
          title={list.name}
          action={
            <div className="flex items-center gap-2">
              <Badge tone="neutral">{list.item_count} symbols</Badge>
              <button
                type="button"
                onClick={async () => {
                  await action.run(() => watchlists.destroy(list.id));
                  await lists.refresh();
                }}
                className="text-2xs text-surface-muted hover:text-down"
              >
                delete list
              </button>
            </div>
          }
        >
          <div className="flex flex-wrap items-end gap-2 border-b border-surface-line px-4 py-3">
            <label className="text-2xs uppercase tracking-widest text-surface-muted">
              Add symbol
              <input
                value={target[list.id] ?? ""}
                onChange={(e) => setTarget((prev) => ({ ...prev, [list.id]: e.target.value }))}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void addSymbol(list.id);
                }}
                placeholder="NABIL"
                className="mt-1 block w-32 rounded border border-surface-line bg-surface-base px-2 py-1.5 text-xs uppercase text-slate-200"
              />
            </label>
            <button
              type="button"
              onClick={() => addSymbol(list.id)}
              disabled={action.busy}
              className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 hover:border-accent/60 hover:text-accent disabled:opacity-50"
            >
              Add
            </button>
          </div>

          <DataTable
            rows={list.items}
            columns={columnsFor(list.id)}
            rowKey={(r) => r.symbol}
            maxHeight="40vh"
            empty={
              <p className="px-4 py-6 text-xs text-surface-muted">
                This list is empty. Add a symbol above — it does not need to be trading to be tracked.
              </p>
            }
          />
        </Panel>
      ))}

      <Provenance source="postgres:watchlists + postgres:daily_ohlcv" license="public_unverified" />
    </div>
  );
}

function Heading() {
  return (
    <div>
      <h1 className="text-xl font-semibold tracking-tight text-slate-100">Watchlists</h1>
      <p className="mt-1 text-xs text-surface-muted">
        Track securities and read the last stored close, change and volume for each.
      </p>
    </div>
  );
}
