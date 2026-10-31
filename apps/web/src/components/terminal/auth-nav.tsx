"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { auth } from "@/lib/api";

/**
 * Session-aware masthead links.
 *
 * The header used to invite "Sign in" even while a session was active, which
 * read as a broken login. It resolves against /auth/me on mount; a 401 renders
 * the signed-out links, and `clientFetch` has already tried one silent renewal,
 * so an expired access cookie with a valid refresh cookie still counts as in.
 */
export function AuthNav() {
  const [state, setState] = useState<"checking" | "in" | "out">("checking");

  useEffect(() => {
    let active = true;
    auth
      .me()
      .then(() => active && setState("in"))
      .catch(() => active && setState("out"));
    return () => {
      active = false;
    };
  }, []);

  if (state === "checking") {
    // Render nothing rather than a sign-in prompt that may be wrong a moment
    // later; the flash of "Sign in" was the bug being fixed.
    return <span className="w-24" aria-hidden />;
  }

  if (state === "in") {
    return (
      <Link
        href="/account"
        className="rounded border border-surface-line px-2.5 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/50 hover:text-accent"
      >
        Account
      </Link>
    );
  }

  return (
    <>
      <Link
        href="/login"
        className="rounded px-2.5 py-1.5 text-xs text-surface-muted no-underline hover:text-slate-200"
      >
        Sign in
      </Link>
      <Link
        href="/register"
        className="rounded border border-surface-line px-2.5 py-1.5 text-xs text-slate-200 no-underline hover:border-accent/50 hover:text-accent"
      >
        Create account
      </Link>
    </>
  );
}
