import Link from "next/link";
import type { ReactNode } from "react";

/** Small presentational primitives shared by every page. */

export function Panel({
  title,
  action,
  children,
  className = "",
}: {
  title: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`panel ${className}`}>
      <header className="panel-header">
        <h2 className="panel-title">{title}</h2>
        {action}
      </header>
      {children}
    </section>
  );
}

export function Stat({
  label,
  value,
  hint,
  tone = "",
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  tone?: string;
}) {
  return (
    <div className="px-4 py-3">
      <div className="text-2xs uppercase tracking-widest text-surface-muted">{label}</div>
      <div className={`mt-1 text-lg font-semibold tnum ${tone}`}>{value}</div>
      {hint ? <div className="mt-0.5 text-2xs text-surface-muted">{hint}</div> : null}
    </div>
  );
}

export function Badge({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "neutral" | "good" | "warn" | "bad";
}) {
  const tones = {
    neutral: "border-surface-line text-surface-muted",
    good: "border-up/40 text-up",
    warn: "border-accent/40 text-accent",
    bad: "border-down/40 text-down",
  } as const;
  return (
    <span
      className={`inline-flex items-center rounded border px-1.5 py-0.5 text-2xs uppercase tracking-wider ${tones[tone]}`}
    >
      {children}
    </span>
  );
}

/**
 * UnavailableState is shown instead of a chart or table when the database has no
 * rows for a window. That is a data limit, not an error: the UI says so plainly
 * rather than plotting an empty line that looks like a flat market.
 */
export function UnavailableState({
  title,
  detail,
  action,
}: {
  title: string;
  detail: string;
  action?: { href: string; label: string };
}) {
  return (
    <div className="px-4 py-8 text-center">
      <p className="text-sm font-medium text-slate-300">{title}</p>
      <p className="mx-auto mt-1 max-w-md text-xs leading-relaxed text-surface-muted">{detail}</p>
      {action ? (
        <Link
          href={action.href}
          className="mt-3 inline-block rounded border border-surface-line px-3 py-1.5 text-xs text-slate-300 hover:border-accent/50 hover:text-accent"
        >
          {action.label}
        </Link>
      ) : null}
    </div>
  );
}

export function ErrorState({
  title,
  detail,
  action,
}: {
  title: string;
  detail: string;
  action?: { href: string; label: string };
}) {
  return (
    <div className="panel border-down/40 px-4 py-6">
      <p className="text-sm font-semibold text-down">{title}</p>
      <p className="mt-1 text-xs leading-relaxed text-surface-muted">{detail}</p>
      {action ? (
        <Link
          href={action.href}
          className="mt-3 inline-block rounded border border-surface-line px-3 py-1.5 text-xs text-slate-300 no-underline hover:border-accent/50 hover:text-accent"
        >
          {action.label}
        </Link>
      ) : null}
    </div>
  );
}

/**
 * FreshnessBadge renders the API's freshness verdict.
 *
 * "final" is not a warning: after the close, the session's numbers will not
 * change again. "stale" is the warning, and it carries the reason so the reader
 * can judge it rather than trusting a colour.
 */
export function FreshnessBadge({
  freshness,
  reason,
}: {
  freshness?: string;
  reason?: string;
}) {
  const tone: "good" | "warn" | "bad" | "neutral" =
    freshness === "live" ? "good" : freshness === "delayed" ? "warn" : freshness === "stale" ? "bad" : "neutral";
  const label =
    freshness === "final" ? "final · market closed" : freshness === "live" ? "live" : freshness ?? "freshness unknown";
  return (
    <span
      title={reason}
      className={`inline-flex items-center gap-1 rounded border px-1.5 py-0.5 text-2xs uppercase tracking-wider ${
        tone === "good"
          ? "border-up/40 text-up"
          : tone === "warn"
            ? "border-accent/40 text-accent"
            : tone === "bad"
              ? "border-down/40 text-down"
              : "border-surface-line text-surface-muted"
      }`}
    >
      {label}
    </span>
  );
}

/** Notice is an inline explanation shown above content it qualifies. */
export function Notice({
  tone = "warn",
  title,
  detail,
}: {
  tone?: "warn" | "bad" | "neutral";
  title: string;
  detail: string;
}) {
  const border =
    tone === "bad" ? "border-down/40" : tone === "warn" ? "border-accent/40" : "border-surface-line";
  return (
    <div className={`rounded-lg border ${border} bg-surface-raised px-4 py-3`}>
      <p className={`text-xs font-semibold ${tone === "bad" ? "text-down" : tone === "warn" ? "text-accent" : "text-slate-200"}`}>
        {title}
      </p>
      <p className="mt-1 text-2xs leading-relaxed text-surface-muted">{detail}</p>
    </div>
  );
}

/** ProvenanceFooter states where a figure came from and how fresh it is. */
export function Provenance({ source, asOf, license }: { source?: string; asOf?: string; license?: string }) {
  return (
    <p className="px-1 pt-2 text-2xs leading-relaxed text-surface-muted">
      {source ? <span>source: {source}</span> : null}
      {asOf ? <span> · as of {asOf}</span> : null}
      {license ? <span> · licence: {license}</span> : null}
    </p>
  );
}
