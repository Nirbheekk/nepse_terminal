import Link from "next/link";
import { Nav } from "@/components/terminal/nav";
import { ConnectionStatus } from "@/components/terminal/connection-status";
import { AuthNav } from "@/components/terminal/auth-nav";

/**
 * Shell for every authenticated-or-public terminal screen.
 *
 * The masthead lives here rather than in the root layout so the auth group can
 * render a bare form without market chrome around it.
 */
export default function TerminalLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <header className="sticky top-0 z-20 border-b border-surface-line bg-surface-base/95 backdrop-blur">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-2.5">
          <Link href="/market" className="flex items-baseline gap-2 no-underline">
            <span className="text-sm font-semibold tracking-tight text-slate-100">NEPSE</span>
            <span className="text-2xs uppercase tracking-[0.2em] text-accent">Terminal</span>
          </Link>
          <Nav />
          <div className="ml-auto flex items-center gap-3">
            <ConnectionStatus />
            <AuthNav />
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-5">{children}</main>
    </>
  );
}
