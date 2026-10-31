"use client";

import { useState } from "react";
import { research } from "@/lib/api";
import { useAction } from "@/hooks/use-terminal";
import { num } from "@/lib/utils/format";
import { Badge, Notice, Panel, Provenance, UnavailableState } from "@/components/ui";
import type { ResearchResponse } from "@/lib/schemas/workspace";

/**
 * Research.
 *
 * Every answer is assembled from stored rows and shipped with its citations, so
 * the screen renders the evidence next to the claim. There is no free-form
 * generation here: if the database holds nothing for a question, the response
 * says so and the confidence score reflects it.
 */
export default function ResearchPage() {
  const action = useAction();
  const [text, setText] = useState("");
  const [scope, setScope] = useState("market");
  const [result, setResult] = useState<ResearchResponse | null>(null);

  async function ask() {
    if (!text.trim()) return;
    const res = await action.run(() => research.query({ query_text: text.trim(), scope }));
    if (res) setResult(res.data);
  }

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-slate-100">Research</h1>
        <p className="mt-1 text-xs text-surface-muted">
          Ask a question about a listed company, the market, or the filings on record. Answers are
          assembled from stored data and cite the rows they used.
        </p>
      </div>

      <Panel title="Query">
        <div className="space-y-3 p-4">
          <div className="flex flex-wrap items-end gap-2">
            <label className="flex-1 text-2xs uppercase tracking-widest text-surface-muted">
              Question
              <input
                value={text}
                onChange={(e) => setText(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void ask();
                }}
                placeholder="e.g. What is the free float of NABIL?"
                className="mt-1 block w-full rounded border border-surface-line bg-surface-base px-3 py-2 text-xs text-slate-200"
              />
            </label>
            <label className="text-2xs uppercase tracking-widest text-surface-muted">
              Scope
              <select
                value={scope}
                onChange={(e) => setScope(e.target.value)}
                className="mt-1 block w-32 rounded border border-surface-line bg-surface-raised px-2 py-2 text-xs text-slate-200"
              >
                {["market", "company", "regulatory"].map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              onClick={ask}
              disabled={action.busy || !text.trim()}
              className="rounded border border-accent/50 bg-accent/10 px-4 py-2 text-xs font-medium text-accent disabled:opacity-50"
            >
              {action.busy ? "Searching…" : "Ask"}
            </button>
          </div>
          {action.error ? <Notice tone="bad" title="The query failed" detail={action.error} /> : null}
        </div>
      </Panel>

      {result ? (
        <>
          <Panel
            title="Answer"
            action={
              <div className="flex items-center gap-2">
                <Badge tone={result.supported_by_proof ? "good" : "warn"}>
                  {result.supported_by_proof ? "evidence-backed" : "partially supported"}
                </Badge>
                <span className="text-2xs text-surface-muted">
                  confidence {num(result.confidence_score, "—")}
                </span>
              </div>
            }
          >
            <div className="space-y-3 p-4">
              <p className="text-sm leading-relaxed text-slate-100">{result.summary_answer}</p>
              <p className="text-2xs leading-relaxed text-surface-muted">{result.disclaimer}</p>
            </div>
          </Panel>

          {[
            { title: "Facts", items: result.facts },
            { title: "Calculations", items: result.calculations },
            { title: "Interpretations", items: result.interpretations },
          ].map((group) =>
            group.items.length === 0 ? null : (
              <Panel key={group.title} title={`${group.title} · ${group.items.length}`}>
                <ul className="divide-y divide-surface-line/60">
                  {group.items.map((f, i) => (
                    <li key={`${group.title}-${i}`} className="px-4 py-3">
                      <p className="text-xs leading-relaxed text-slate-200">{f.statement}</p>
                      {f.formula ? (
                        <p className="mt-1 font-mono text-2xs text-surface-muted">{f.formula}</p>
                      ) : null}
                      {(f.citations ?? []).length > 0 ? (
                        <ul className="mt-2 space-y-1">
                          {(f.citations ?? []).map((c, j) => (
                            <li key={j} className="flex flex-wrap items-center gap-2 text-2xs text-surface-muted">
                              <Badge tone="neutral">{c.authority}</Badge>
                              <span>{c.citation_text}</span>
                              {c.as_of_date ? <span>· as of {c.as_of_date}</span> : null}
                              {c.attachment_url ? (
                                <a
                                  href={c.attachment_url}
                                  target="_blank"
                                  rel="noreferrer noopener"
                                  className="text-accent no-underline hover:underline"
                                >
                                  source →
                                </a>
                              ) : null}
                            </li>
                          ))}
                        </ul>
                      ) : null}
                    </li>
                  ))}
                </ul>
              </Panel>
            ),
          )}

          <Provenance source="postgres:filings + postgres:daily_ohlcv" license={result.license_notice} />
        </>
      ) : (
        <Panel title="No query yet">
          <UnavailableState
            title="Ask a question to begin"
            detail="The assistant only answers from rows the database holds. A question it cannot ground returns an explicit low-confidence answer rather than a plausible guess."
          />
        </Panel>
      )}
    </div>
  );
}
