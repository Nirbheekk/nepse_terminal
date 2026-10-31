import { disclosures, ApiError } from "@/lib/api";
import { dateTime } from "@/lib/utils/format";
import { Badge, ErrorState, Notice, Panel, Provenance, UnavailableState } from "@/components/ui";

export const dynamic = "force-dynamic";

/**
 * News is the disclosure feed, not a scraped media feed.
 *
 * Every row here came from an authority document that was stored with its
 * source URL and capture time, so a headline can always be traced back. There is
 * no sentiment or summary generated locally: what is shown is what was filed.
 */
export default async function NewsPage() {
  let items: Awaited<ReturnType<typeof disclosures.list>>["data"] = [];
  let error: string | null = null;

  try {
    items = (await disclosures.list(50)).data;
  } catch (err) {
    error = err instanceof ApiError ? err.message : "The disclosure feed could not be loaded";
  }

  const rows = items ?? [];

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-slate-100">News &amp; disclosures</h1>
        <p className="mt-1 text-xs text-surface-muted">
          Company filings and regulatory disclosures, in publication order with their source link
          preserved.
        </p>
      </div>

      {error ? (
        <ErrorState title="The disclosure feed is unavailable" detail={error} />
      ) : (
        <Notice
          tone="neutral"
          title="This is a filings feed"
          detail="Only documents that were actually captured appear here. An empty list means nothing has been ingested yet, not that no announcements exist."
        />
      )}

      <Panel title={`Disclosures${rows.length ? ` · ${rows.length}` : ""}`}>
        {rows.length === 0 ? (
          <UnavailableState
            title="No disclosures stored"
            detail="The database holds no announcement records. Run the disclosure ingestion job against the exchange, then reload."
          />
        ) : (
          <ul className="divide-y divide-surface-line/60">
            {rows.map((d) => (
              <li key={d.id} className="px-4 py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone="neutral">{d.category}</Badge>
                  <span className="text-2xs uppercase tracking-widest text-surface-muted">{d.authority}</span>
                  {d.symbol ? <span className="text-xs font-medium text-slate-200">{d.symbol}</span> : null}
                  <span className="ml-auto text-2xs text-surface-muted">
                    {d.published_at_npt || dateTime(d.published_at)}
                  </span>
                </div>
                <p className="mt-1.5 text-sm font-medium text-slate-100">{d.title}</p>
                {d.summary ? (
                  <p className="mt-1 max-w-3xl text-xs leading-relaxed text-surface-muted">{d.summary}</p>
                ) : null}
                {d.source_url ? (
                  <a
                    href={d.source_url}
                    target="_blank"
                    rel="noreferrer noopener"
                    className="mt-1.5 inline-block text-2xs text-accent no-underline hover:underline"
                  >
                    Open source document →
                  </a>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <Provenance source="postgres:disclosures" license="public_unverified" />
    </div>
  );
}
