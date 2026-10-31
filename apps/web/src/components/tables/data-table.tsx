"use client";

import { useMemo, useState, type ReactNode } from "react";

/**
 * A dense, sortable table.
 *
 * Sorting happens here rather than on the server because every caller already
 * holds the full result set (a screen is capped by its own limit), and
 * re-sorting client-side keeps the sort stable while the user compares columns.
 *
 * `sortValue` lets a column sort numerically even when it renders a formatted
 * string: "1,399.00" must sort as 1399, not as text.
 */
export type Column<T> = {
  key: string;
  label: string;
  align?: "left" | "right";
  render: (row: T) => ReactNode;
  sortValue?: (row: T) => string | number | null | undefined;
  defaultDesc?: boolean;
};

export function DataTable<T>({
  rows,
  columns,
  rowKey,
  empty,
  maxHeight = "70vh",
}: {
  rows: T[];
  columns: Column<T>[];
  rowKey: (row: T) => string;
  empty: ReactNode;
  maxHeight?: string;
}) {
  const [sort, setSort] = useState<{ key: string; desc: boolean } | null>(null);

  const sorted = useMemo(() => {
    if (!sort) return rows;
    const col = columns.find((c) => c.key === sort.key);
    if (!col?.sortValue) return rows;
    const value = col.sortValue;
    return [...rows].sort((a, b) => {
      const av = value(a);
      const bv = value(b);
      // Nulls always sink: a missing metric is not "the smallest value", it is
      // absent, and sorting it above real numbers would misrepresent the screen.
      if (av === null || av === undefined) return 1;
      if (bv === null || bv === undefined) return -1;
      const an = typeof av === "number" ? av : Number(av);
      const bn = typeof bv === "number" ? bv : Number(bv);
      const cmp =
        Number.isFinite(an) && Number.isFinite(bn)
          ? an - bn
          : String(av).localeCompare(String(bv));
      return sort.desc ? -cmp : cmp;
    });
  }, [rows, columns, sort]);

  if (rows.length === 0) {
    return <>{empty}</>;
  }

  return (
    <div className="overflow-auto" style={{ maxHeight }}>
      <table className="w-full border-collapse text-xs">
        <thead className="sticky top-0 z-10 bg-surface-raised">
          <tr>
            {columns.map((c) => {
              const active = sort?.key === c.key;
              const sortable = Boolean(c.sortValue);
              return (
                <th
                  key={c.key}
                  scope="col"
                  aria-sort={active ? (sort?.desc ? "descending" : "ascending") : "none"}
                  className={`border-b border-surface-line px-3 py-2 font-medium text-2xs uppercase tracking-wider text-surface-muted ${
                    c.align === "right" ? "text-right" : "text-left"
                  }`}
                >
                  {sortable ? (
                    <button
                      type="button"
                      onClick={() =>
                        setSort((prev) =>
                          prev?.key === c.key
                            ? prev.desc
                              ? { key: c.key, desc: false }
                              : null
                            : { key: c.key, desc: c.defaultDesc ?? true },
                        )
                      }
                      className={`inline-flex items-center gap-1 hover:text-slate-200 ${
                        active ? "text-slate-100" : ""
                      }`}
                    >
                      {c.label}
                      <span aria-hidden className="text-2xs">
                        {active ? (sort?.desc ? "▼" : "▲") : "↕"}
                      </span>
                    </button>
                  ) : (
                    c.label
                  )}
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {sorted.map((row) => (
            <tr key={rowKey(row)} className="border-b border-surface-line/60 hover:bg-white/[0.02]">
              {columns.map((c) => (
                <td
                  key={c.key}
                  className={`whitespace-nowrap px-3 py-1.5 ${c.align === "right" ? "text-right tnum" : "text-left"}`}
                >
                  {c.render(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
