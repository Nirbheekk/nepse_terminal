import Link from "next/link";

/**
 * Auth group shell: no market chrome, because a sign-in form is the whole page.
 */
export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <main className="mx-auto flex min-h-[80vh] max-w-md flex-col justify-center px-4">
      <Link href="/market" className="mb-6 flex items-baseline gap-2 no-underline">
        <span className="text-sm font-semibold tracking-tight text-slate-100">NEPSE</span>
        <span className="text-2xs uppercase tracking-[0.2em] text-accent">Terminal</span>
      </Link>
      {children}
    </main>
  );
}
