"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { ApiError, auth } from "@/lib/api";
import { dateTime } from "@/lib/utils/format";
import type { User, UserSession } from "@/lib/schemas";
import { Panel, UnavailableState } from "@/components/ui";

export default function AccountPage() {
  const [user, setUser] = useState<User | null>(null);
  const [sessions, setSessions] = useState<UserSession[]>([]);
  const [state, setState] = useState<"loading" | "anonymous" | "ready" | "error">("loading");
  const [message, setMessage] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const me = await auth.me();
      setUser(me.data);
      const list = await auth.sessions();
      setSessions(list.data);
      setState("ready");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("anonymous");
        return;
      }
      setMessage(err instanceof ApiError ? err.message : "Could not load your account.");
      setState("error");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function revoke(id: string) {
    setMessage(null);
    try {
      await auth.revokeSession(id);
      await load();
      setMessage("Session revoked.");
    } catch (err) {
      setMessage(err instanceof ApiError ? err.message : "Could not revoke that session.");
    }
  }

  async function signOut() {
    try {
      await auth.logout();
    } finally {
      setUser(null);
      setSessions([]);
      setState("anonymous");
    }
  }

  if (state === "loading") {
    return <p className="text-xs text-surface-muted">Loading account…</p>;
  }

  if (state === "anonymous") {
    return (
      <div className="mx-auto max-w-md space-y-4">
        <Panel title="Account">
          <div className="px-4 py-6 text-center">
            <p className="text-sm text-slate-300">You are not signed in.</p>
            <p className="mt-1 text-xs text-surface-muted">
              Sign in to manage watchlists, sessions and exports. Public market data remains readable
              without an account.
            </p>
            <div className="mt-4 flex justify-center gap-2">
              <Link
                href="/login"
                className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/60 hover:text-accent"
              >
                Sign in
              </Link>
              <Link
                href="/register"
                className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/60 hover:text-accent"
              >
                Create account
              </Link>
            </div>
          </div>
        </Panel>
      </div>
    );
  }

  if (state === "error" || !user) {
    return (
      <Panel title="Account">
        <UnavailableState title="Account unavailable" detail={message ?? "The API did not respond."} />
      </Panel>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h1 className="text-lg font-semibold tracking-tight text-slate-100">Account</h1>
          <p className="text-xs text-surface-muted">
            {user.display_name || user.email} · roles: {user.roles.join(", ") || "none"}
          </p>
        </div>
        <button
          onClick={signOut}
          className="rounded border border-surface-line px-3 py-1.5 text-xs text-slate-200 hover:border-down/60 hover:text-down"
        >
          Sign out
        </button>
      </div>

      {message ? (
        <p className="rounded border border-surface-line px-3 py-2 text-xs text-slate-300">{message}</p>
      ) : null}

      <div className="grid gap-4 lg:grid-cols-3">
        <Panel title="Identity" className="lg:col-span-1">
          <dl className="divide-y divide-surface-line/60 text-xs">
            <Row label="Email" value={user.email} />
            <Row label="Status" value={user.status} />
            <Row label="Roles" value={user.roles.join(", ") || "—"} />
            <Row label="Created" value={dateTime(user.created_at)} />
            <Row label="Last sign-in" value={user.last_login_at ? dateTime(user.last_login_at) : "—"} />
          </dl>
        </Panel>

        <Panel title="Active sessions" className="lg:col-span-2">
          {sessions.length === 0 ? (
            <UnavailableState
              title="No active sessions"
              detail="Sessions appear here after a sign-in and disappear when revoked or expired."
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead className="text-surface-muted">
                  <tr>
                    <th className="cell-left font-normal">Started</th>
                    <th className="cell-left font-normal">Device</th>
                    <th className="cell-left font-normal">Expires</th>
                    <th className="cell font-normal" />
                  </tr>
                </thead>
                <tbody>
                  {sessions.map((s) => (
                    <tr key={s.id} className="row">
                      <td className="cell-left text-surface-muted">{dateTime(s.created_at)}</td>
                      <td className="cell-left max-w-[20rem] truncate text-slate-300">
                        {s.user_agent || "unknown client"}
                      </td>
                      <td className="cell-left text-surface-muted">{dateTime(s.expires_at)}</td>
                      <td className="cell text-right">
                        <button
                          onClick={() => revoke(s.id)}
                          className="rounded border border-surface-line px-2 py-1 text-2xs text-surface-muted hover:border-down/60 hover:text-down"
                        >
                          Revoke
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="px-4 py-3 text-2xs leading-relaxed text-surface-muted">
            Revoking a session invalidates its refresh token immediately; the access cookie expires within
            one access-token lifetime.
          </p>
        </Panel>
      </div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 px-4 py-1.5">
      <dt className="text-surface-muted">{label}</dt>
      <dd className="truncate text-slate-300">{value}</dd>
    </div>
  );
}
