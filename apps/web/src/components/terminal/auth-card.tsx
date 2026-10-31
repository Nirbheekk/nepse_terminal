"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { z } from "zod";
import { ApiError, auth } from "@/lib/api";
import { CredentialsSchema, RegisterSchema } from "@/lib/schemas";

type Mode = "login" | "register";

/**
 * One form for both flows. Validation runs client-side for responsiveness and
 * again on the server, which is the only check that counts.
 */
export function AuthCard({ mode }: { mode: Mode }) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const isRegister = mode === "register";

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    setFieldErrors({});

    const schema = isRegister ? RegisterSchema : CredentialsSchema;
    const parsed = schema.safeParse(
      isRegister
        ? { email, password, display_name: displayName }
        : { email, password },
    );
    if (!parsed.success) {
      const flat: Record<string, string> = {};
      for (const issue of parsed.error.issues) {
        const key = String(issue.path[0] ?? "form");
        flat[key] ??= issue.message;
      }
      setFieldErrors(flat);
      return;
    }

    setBusy(true);
    try {
      if (isRegister) {
        await auth.register({
          email: parsed.data.email,
          password: parsed.data.password,
          display_name: displayName,
        });
      } else {
        await auth.login({ email: parsed.data.email, password: parsed.data.password });
      }
      router.push("/account");
      router.refresh();
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.code === "CONFLICT"
            ? "That email is already registered. Try signing in instead."
            : err.message
          : "Something went wrong. Please try again.";
      setFormError(message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-md space-y-4">
      <div>
        <h1 className="text-lg font-semibold tracking-tight text-slate-100">
          {isRegister ? "Create an account" : "Sign in"}
        </h1>
        <p className="mt-1 text-xs leading-relaxed text-surface-muted">
          Accounts are needed for watchlists, saved screens and exports. Market data stays readable
          without signing in.
        </p>
      </div>

      <form onSubmit={onSubmit} className="panel space-y-3 p-4" noValidate>
        {isRegister ? (
          <Field label="Name" error={fieldErrors.display_name}>
            <input
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              autoComplete="name"
              className="w-full rounded border border-surface-line bg-surface-base px-2.5 py-2 text-sm text-slate-200 focus:border-accent/60 focus:outline-none"
            />
          </Field>
        ) : null}

        <Field label="Email" error={fieldErrors.email}>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="email"
            className="w-full rounded border border-surface-line bg-surface-base px-2.5 py-2 text-sm text-slate-200 focus:border-accent/60 focus:outline-none"
          />
        </Field>

        <Field label="Password" error={fieldErrors.password} hint={isRegister ? "At least 10 characters." : undefined}>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete={isRegister ? "new-password" : "current-password"}
            className="w-full rounded border border-surface-line bg-surface-base px-2.5 py-2 text-sm text-slate-200 focus:border-accent/60 focus:outline-none"
          />
        </Field>

        {formError ? (
          <p className="rounded border border-down/40 bg-down/10 px-3 py-2 text-xs text-down" role="alert">
            {formError}
          </p>
        ) : null}

        <button
          type="submit"
          disabled={busy}
          className="w-full rounded border border-surface-line bg-white/[0.04] px-3 py-2 text-sm font-medium text-slate-100 transition-colors hover:border-accent/60 hover:text-accent disabled:opacity-50"
        >
          {busy ? "Working…" : isRegister ? "Create account" : "Sign in"}
        </button>

        <p className="text-2xs leading-relaxed text-surface-muted">
          Sessions use HttpOnly cookies: the access token is never stored in JavaScript or
          localStorage, and mutations require a CSRF token.
        </p>
      </form>

      <p className="text-center text-xs text-surface-muted">
        {isRegister ? (
          <>
            Already registered?{" "}
            <Link href="/login" className="text-slate-300 hover:text-accent">
              Sign in
            </Link>
          </>
        ) : (
          <>
            No account?{" "}
            <Link href="/register" className="text-slate-300 hover:text-accent">
              Create one
            </Link>
          </>
        )}
      </p>
    </div>
  );
}

function Field({
  label,
  error,
  hint,
  children,
}: {
  label: string;
  error?: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-2xs uppercase tracking-widest text-surface-muted">{label}</span>
      {children}
      {error ? (
        <span className="mt-1 block text-2xs text-down">{error}</span>
      ) : hint ? (
        <span className="mt-1 block text-2xs text-surface-muted">{hint}</span>
      ) : null}
    </label>
  );
}
