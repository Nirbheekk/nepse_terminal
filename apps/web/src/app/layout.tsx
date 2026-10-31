import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "NEPSE Terminal",
  description:
    "Market terminal for the Nepal Stock Exchange: session summary, movers, sectors, company profiles, the trade-level floorsheet, screeners and research.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-screen">
        {children}
        <footer className="mx-auto max-w-7xl px-4 pb-8 pt-4">
          <p className="border-t border-surface-line pt-4 text-2xs leading-relaxed text-surface-muted">
            Market data is collected from the Nepal Stock Exchange through an unofficial client and is
            provided for personal research only. It is unverified, may be delayed, and is not licensed
            for redistribution. Figures are shown with their capture time and coverage; where the
            database holds no observation, the terminal says so instead of estimating.
          </p>
        </footer>
      </body>
    </html>
  );
}
