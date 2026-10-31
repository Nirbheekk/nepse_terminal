import Link from "next/link";
import { ops, ApiError } from "@/lib/api";
import { ago, dateTime, num } from "@/lib/utils/format";
import { Badge, ErrorState, Notice, Panel, Stat } from "@/components/ui";

export const dynamic = "force-dynamic";

/**
 * Operational view of the data plane.
 *
 * This answers "can I trust what the terminal shows?" rather than "is the server
 * up": row counts per dataset, how old each capture is, whether the last ingest
 * wrote rows or skipped them, and what raw payloads are on disk to re-derive
 * from. All of it comes from the public health endpoints.
 */
export default async function AdminPage() {
  let health;
  let freshness;
  let runs;
  let archives;
  let error: string | null = null;

  try {
    [health, freshness, runs, archives] = await Promise.all([
      ops.health(),
      ops.freshness(),
      ops.ingestRuns(8),
      ops.archives(),
    ]);
  } catch (err) {
    error = err instanceof ApiError ? err.message : "The data health API is unavailable";
  }

  if (error) {
    return <ErrorState title="Data health is unavailable" detail={`${error} Check that the API is running, then reload.`} />;
  }

  const h = health!.data;
  const streams = freshness!.data.datasets;
  const stale = streams.filter((s) => s.status !== "fresh");

  return (
    <main className="mx-auto max-w-7xl px-4 py-6">
      <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-slate-100">Data operations</h1>
          <p className="mt-1 text-xs text-surface-muted">
            Coverage, freshness and provenance of every dataset the terminal reads.
          </p>
        </div>
        <Link href="/market" className="text-xs text-accent no-underline hover:underline">
          ← Back to the terminal
        </Link>
      </div>

      <div className="mb-5 grid grid-cols-2 divide-x divide-surface-line rounded-lg border border-surface-line md:grid-cols-4">
        <Stat label="Companies" value={num(h.companies)} />
        <Stat label="Securities" value={num(h.securities)} />
        <Stat label="Stored sessions" value={num(h.sessions)} hint={`latest ${h.latest_session}`} />
        <Stat label="Floorsheet trades" value={num(h.trades)} hint={`last ingest ${ago(h.latest_ingest)}`} />
      </div>

      {stale.length > 0 ? (
        <div className="mb-5">
          <Notice
            tone="warn"
            title={`${stale.length} dataset${stale.length === 1 ? " is" : "s are"} outside their freshness budget`}
            detail={stale
              .map((s) => `${s.stream}: ${s.status}${s.age_seconds ? ` (${ago(s.latest_at ?? undefined)})` : ""}`)
              .join(" · ")}
          />
        </div>
      ) : null}

      <div className="space-y-5">
        <Panel title="Dataset freshness">
          <div className="overflow-auto">
            <table className="w-full border-collapse text-xs">
              <thead>
                <tr className="text-2xs uppercase tracking-wider text-surface-muted">
                  <th className="border-b border-surface-line px-4 py-2 text-left font-medium">Stream</th>
                  <th className="border-b border-surface-line px-3 py-2 text-left font-medium">Owner</th>
                  <th className="border-b border-surface-line px-3 py-2 text-right font-medium">Rows</th>
                  <th className="border-b border-surface-line px-3 py-2 text-right font-medium">Budget</th>
                  <th className="border-b border-surface-line px-3 py-2 text-left font-medium">Latest capture</th>
                  <th className="border-b border-surface-line px-3 py-2 text-left font-medium">State</th>
                </tr>
              </thead>
              <tbody>
                {streams.map((s) => (
                  <tr key={s.stream} className="border-b border-surface-line/60">
                    <td className="px-4 py-1.5">
                      <div className="text-slate-200">{s.stream}</div>
                      <div className="text-2xs text-surface-muted">{s.description}</div>
                    </td>
                    <td className="px-3 py-1.5 text-surface-muted">{s.owner ?? "—"}</td>
                    <td className="px-3 py-1.5 text-right tnum">{num(s.rows ?? 0)}</td>
                    <td className="px-3 py-1.5 text-right tnum text-surface-muted">
                      {s.max_age_seconds ? `${Math.round(s.max_age_seconds / 3600)}h` : "—"}
                    </td>
                    <td className="px-3 py-1.5 text-surface-muted">
                      {s.latest_at ? dateTime(s.latest_at) : "never"}
                    </td>
                    <td className="px-3 py-1.5">
                      <Badge tone={s.status === "fresh" ? "good" : s.status === "stale" ? "bad" : "warn"}>
                        {s.status}
                      </Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Panel>

        <Panel title="Recent ingestion runs">
          {runs!.data.length === 0 ? (
            <p className="px-4 py-6 text-xs text-surface-muted">No ingest runs recorded yet.</p>
          ) : (
            <div className="overflow-auto">
              <table className="w-full border-collapse text-xs">
                <thead>
                  <tr className="text-2xs uppercase tracking-wider text-surface-muted">
                    <th className="border-b border-surface-line px-4 py-2 text-left font-medium">Source</th>
                    <th className="border-b border-surface-line px-3 py-2 text-left font-medium">Stream</th>
                    <th className="border-b border-surface-line px-3 py-2 text-left font-medium">Status</th>
                    <th className="border-b border-surface-line px-3 py-2 text-right font-medium">Read</th>
                    <th className="border-b border-surface-line px-3 py-2 text-right font-medium">Written</th>
                    <th className="border-b border-surface-line px-3 py-2 text-right font-medium">Rejected</th>
                    <th className="border-b border-surface-line px-3 py-2 text-left font-medium">Started</th>
                  </tr>
                </thead>
                <tbody>
                  {runs!.data.map((r, i) => (
                    <tr key={`${r.stream}-${r.started_at}-${i}`} className="border-b border-surface-line/60">
                      <td className="px-4 py-1.5 text-slate-200">{r.source}</td>
                      <td className="px-3 py-1.5 text-surface-muted">{r.stream}</td>
                      <td className="px-3 py-1.5">
                        <Badge tone={r.status === "success" ? "good" : r.status === "skipped" ? "neutral" : "warn"}>
                          {r.status}
                        </Badge>
                      </td>
                      <td className="px-3 py-1.5 text-right tnum">{num(r.rows_read)}</td>
                      <td className="px-3 py-1.5 text-right tnum">{num(r.rows_written)}</td>
                      <td className="px-3 py-1.5 text-right tnum">{num(r.rows_rejected)}</td>
                      <td className="px-3 py-1.5 text-surface-muted">{dateTime(r.started_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Panel>

        <Panel title="Raw archives on disk">
          {archives!.data.length === 0 ? (
            <p className="px-4 py-6 text-xs text-surface-muted">
              No raw payloads archived. Ingestion keeps the original response so any derived row can be
              re-derived without re-fetching.
            </p>
          ) : (
            <div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3">
              {archives!.data.map((a) => (
                <div key={a.stream} className="rounded border border-surface-line px-3 py-2">
                  <div className="text-2xs uppercase tracking-widest text-surface-muted">{a.stream}</div>
                  <div className="mt-1 text-sm tnum text-slate-200">
                    {a.archives} file{a.archives === 1 ? "" : "s"} · {num(a.rows)} rows
                  </div>
                  <div className="mt-0.5 text-2xs text-surface-muted">
                    {(a.bytes / 1024).toFixed(0)} KiB · latest {a.latest_at ? dateTime(a.latest_at) : "never"}
                  </div>
                </div>
              ))}
            </div>
          )}
        </Panel>
      </div>
    </main>
  );
}
