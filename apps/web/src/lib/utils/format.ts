/**
 * Formatting helpers.
 *
 * Values arrive as NUMERIC strings. They are converted with Number() only for
 * display, never for arithmetic that feeds back into the API, so precision loss
 * cannot travel back into stored data.
 */

const plain = new Intl.NumberFormat("en-IN", { maximumFractionDigits: 2 });

export function num(value: string | number | null | undefined, fallback = "—"): string {
  if (value === null || value === undefined || value === "") return fallback;
  const n = Number(value);
  if (!Number.isFinite(n)) return fallback;
  return plain.format(n);
}

/** Nepali market convention: crore/lakh read better than million/billion. */
export function money(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === "") return "—";
  const n = Number(value);
  if (!Number.isFinite(n)) return "—";
  if (Math.abs(n) >= 1e7) return `Rs ${(n / 1e7).toFixed(2)} cr`;
  if (Math.abs(n) >= 1e5) return `Rs ${(n / 1e5).toFixed(2)} lakh`;
  return `Rs ${plain.format(n)}`;
}

/**
 * Compact magnitude with explicit, unambiguous units.
 *
 * Intl's `en-IN` compact notation suffixes thousands with "T" and thousands of
 * crores with "T Cr". On a trading screen "T" reads as trillion, so 13,683
 * shares were displayed as "13.68T". The units below are spelled out instead:
 * lakh and crore for the Nepali convention, K for the sub-lakh range.
 */
export function compactNumber(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === "") return "—";
  const n = Number(value);
  if (!Number.isFinite(n)) return "—";
  const abs = Math.abs(n);
  if (abs >= 1e7) return `${plain.format(n / 1e7)} cr`;
  if (abs >= 1e5) return `${plain.format(n / 1e5)} lakh`;
  if (abs >= 1e3) return `${plain.format(n / 1e3)}K`;
  return plain.format(n);
}

export function pct(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === "") return "—";
  const n = Number(value);
  if (!Number.isFinite(n)) return "—";
  return `${n > 0 ? "+" : ""}${n.toFixed(2)}%`;
}

export function signed(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === "") return "—";
  const n = Number(value);
  if (!Number.isFinite(n)) return "—";
  return `${n > 0 ? "+" : ""}${plain.format(n)}`;
}

/** Tailwind colour class for a signed change. Zero is neutral, not green. */
export function trendClass(value: string | number | null | undefined): string {
  const n = Number(value ?? 0);
  if (!Number.isFinite(n) || n === 0) return "text-slate-300";
  return n > 0 ? "text-up" : "text-down";
}

export function shortDate(value: string | null | undefined): string {
  if (!value) return "—";
  const iso = value.length > 10 ? value.slice(0, 10) : value;
  return iso;
}

export function dateTime(value: string | null | undefined): string {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString("en-GB", {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * "12 minutes ago" for a timestamp. A future timestamp (clock skew between the
 * API host and the browser) reads as "just now" rather than a negative age.
 */
export function ago(value: string | null | undefined): string {
  if (!value) return "—";
  const t = new Date(value).getTime();
  if (Number.isNaN(t)) return "—";
  const seconds = Math.max(0, Math.round((Date.now() - t) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 36) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

export function timeOnly(value: string | null | undefined): string {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}
