"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { API_BASE } from "@/lib/api";
import { ago } from "@/lib/utils/format";

type MarketStatus = {
  state: string;
  reported_status: string;
  as_of: string | null;
  observed_at: string | null;
  age_seconds: number | null;
  session_date: string;
  is_open: boolean;
  is_current_session: boolean;
};

/**
 * ConnectionStatus is the one part of the page that keeps watching after render.
 *
 * Server components cannot re-check the API on their own, so without this a page
 * opened hours ago looks exactly as current as one opened a second ago. It polls
 * the exchange state endpoint, and its verdict is about the API connection and
 * the exchange — never about whether the figures on screen look plausible.
 */
export function ConnectionStatus() {
  const router = useRouter();
  const [status, setStatus] = useState<MarketStatus | null>(null);
  const [online, setOnline] = useState<boolean | null>(null);
  const [checkedAt, setCheckedAt] = useState<Date | null>(null);
  const [busy, setBusy] = useState(false);

  const poll = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/v1/market/status`, {
        cache: "no-store",
        headers: { Accept: "application/json" },
        signal: AbortSignal.timeout(6000),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const body = (await res.json()) as { data: MarketStatus };
      setStatus(body.data);
      setOnline(true);
    } catch {
      setOnline(false);
    }
    setCheckedAt(new Date());
  }, []);

  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      if (cancelled) return;
      await poll();
    };
    void tick();
    const id = window.setInterval(tick, 30_000);
    // A laptop that was asleep comes back with a stale poll interval; re-check
    // as soon as the tab is visible again.
    const onVisible = () => {
      if (document.visibilityState === "visible") void tick();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      cancelled = true;
      window.clearInterval(id);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [poll]);

  const refresh = () => {
    setBusy(true);
    router.refresh();
    window.setTimeout(() => setBusy(false), 1200);
  };

  const stateLabel = status
    ? status.state === "open"
      ? "market open"
      : status.state === "preopen"
        ? "pre-open"
        : status.state === "closed"
          ? "market closed"
          : status.reported_status
            ? `state: ${status.reported_status.toLowerCase()}`
            : "state unknown"
    : "state unknown";

  return (
    <div className="flex items-center gap-2 text-2xs" aria-live="polite">
      <span
        className={`h-1.5 w-1.5 rounded-full ${
          online === null ? "bg-surface-muted" : online ? "bg-up" : "bg-down"
        }`}
        aria-hidden
      />
      <span className={online === false ? "text-down" : "text-surface-muted"}>
        {online === null ? "checking API" : online ? "API connected" : "API unreachable"}
      </span>
      <span className="text-surface-line" aria-hidden>
        |
      </span>
      <span className={status?.is_open ? "text-up" : "text-slate-300"}>{stateLabel}</span>
      {status?.observed_at ? (
        <span className="text-surface-muted" title={`Exchange timestamp ${status.as_of ?? "unknown"}`}>
          · {ago(status.observed_at)}
        </span>
      ) : null}
      <button
        type="button"
        onClick={refresh}
        disabled={busy}
        title={checkedAt ? `Last checked ${checkedAt.toLocaleTimeString("en-GB")}` : "Reload this page"}
        className="rounded border border-surface-line px-1.5 py-0.5 text-2xs text-surface-muted hover:border-accent/60 hover:text-accent disabled:opacity-50"
      >
        {busy ? "refreshing…" : "Refresh"}
      </button>
    </div>
  );
}
