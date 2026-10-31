"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, type Result } from "@/lib/api";

/**
 * Load one API resource in the browser.
 *
 * Two behaviours are deliberate:
 *
 *   - `unauthenticated` is reported separately from `error`. An anonymous
 *     visitor hitting /alerts has not hit a failure, they just have no account
 *     yet, and the screen should offer sign-in rather than an error box.
 *   - a reload never clears the previous data, so a refresh in flight does not
 *     make a populated table flicker empty.
 */
export function useResource<T>(loader: () => Promise<Result<T>>) {
  const [data, setData] = useState<T | null>(null);
  const [meta, setMeta] = useState<Result<T>["meta"] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [unauthenticated, setUnauthenticated] = useState(false);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const res = await loader();
      if (!mounted.current) return;
      setData(res.data);
      setMeta(res.meta);
      setError(null);
      setUnauthenticated(false);
    } catch (err) {
      if (!mounted.current) return;
      const e = err instanceof ApiError ? err : null;
      setUnauthenticated(e?.status === 401);
      setError(e ? e.message : String(err));
    } finally {
      if (mounted.current) setLoading(false);
    }
  }, [loader]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { data, meta, loading, error, unauthenticated, refresh, setData };
}

/**
 * Run a mutating call and keep its busy/error state local to the control that
 * triggered it, so one failing button does not blank the page.
 */
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = useCallback(async <T,>(fn: () => Promise<T>): Promise<T | null> => {
    setBusy(true);
    setError(null);
    try {
      return await fn();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
      return null;
    } finally {
      setBusy(false);
    }
  }, []);

  return { busy, error, run, setError };
}
