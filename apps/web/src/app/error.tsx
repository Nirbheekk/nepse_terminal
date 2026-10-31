"use client";

import { useEffect } from "react";

/**
 * Route-level failure boundary.
 *
 * A server component that cannot reach the API throws, and without this boundary
 * the reader gets Next's development overlay instead of the one fact that
 * matters: the page could not be built because the API did not answer.
 */
export default function RouteError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error("route render failed", error);
  }, [error]);

  return (
    <div className="panel border-down/40 px-4 py-6">
      <p className="text-sm font-semibold text-down">This page could not be built</p>
      <p className="mt-1 max-w-xl text-xs leading-relaxed text-surface-muted">
        A request to the market API failed while rendering. The figures were not cached, so nothing
        partial is shown. Check that the Go API is running on port 8081, then try again.
      </p>
      <button
        type="button"
        onClick={reset}
        className="mt-3 rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 hover:border-accent/60 hover:text-accent"
      >
        Try again
      </button>
    </div>
  );
}
