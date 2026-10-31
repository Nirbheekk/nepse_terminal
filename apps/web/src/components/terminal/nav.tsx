"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

/**
 * Primary terminal navigation.
 *
 * Order follows the research workflow: the market as a whole, then a single
 * instrument, then analysis (charts, indices, sectors), then the tools that
 * depend on a signed-in account last.
 */
const groups: { label: string; href: string }[] = [
  { href: "/market", label: "Market" },
  { href: "/stocks", label: "Stocks" },
  { href: "/charts", label: "Charts" },
  { href: "/indices", label: "Indices" },
  { href: "/sectors", label: "Sectors" },
  { href: "/screener", label: "Screener" },
  { href: "/floorsheet", label: "Floorsheet" },
  { href: "/watchlists", label: "Watchlists" },
  { href: "/alerts", label: "Alerts" },
  { href: "/portfolio", label: "Portfolio" },
  { href: "/news", label: "News" },
  { href: "/research", label: "Research" },
];

export function Nav() {
  const pathname = usePathname();
  return (
    <nav className="flex flex-wrap items-center gap-1">
      {groups.map((l) => {
        const active = pathname === l.href || pathname.startsWith(`${l.href}/`);
        return (
          <Link
            key={l.href}
            href={l.href}
            className={`rounded px-2.5 py-1.5 text-xs font-medium no-underline transition-colors ${
              active
                ? "bg-white/[0.06] text-slate-100"
                : "text-surface-muted hover:bg-white/[0.03] hover:text-slate-200"
            }`}
          >
            {l.label}
          </Link>
        );
      })}
    </nav>
  );
}
