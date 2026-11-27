"use client";

import { useLayoutEffect, useMemo, useRef, useState } from "react";
import type { CheckResult, UptimeBucket } from "@/lib/types";
import { ERROR_CLASS_LABEL, fmtDateTime, fmtMs } from "@/lib/format";

/** Classification used by every check visualisation so colors stay consistent. */
export type CellKind = "up" | "slow" | "down" | "empty";

export const CELL_CLASS: Record<CellKind, string> = {
  up: "bg-up hover:ring-up/60",
  slow: "bg-warn hover:ring-warn/60",
  down: "bg-down hover:ring-down/60",
  empty: "bg-surface-sunken ring-1 ring-inset ring-border/70",
};

/** A check counts as "slow" when it succeeded but took far longer than usual. */
export function slowThreshold(checks: CheckResult[]): number {
  const ups = checks.filter((c) => c.status === "up").map((c) => c.duration_ms).sort((a, b) => a - b);
  if (ups.length < 5) return Number.POSITIVE_INFINITY;
  const median = ups[Math.floor(ups.length / 2)];
  return Math.max(median * 2.5, 1000);
}

export function classify(c: CheckResult, slowMs: number): CellKind {
  if (c.status !== "up") return "down";
  return c.duration_ms > slowMs ? "slow" : "up";
}

interface Tip {
  x: number;
  y: number;
  content: React.ReactNode;
}

/** Floating tooltip anchored to a cell; rendered fixed so it escapes overflow. */
function useTooltip() {
  const [tip, setTip] = useState<Tip | null>(null);
  const show = (el: HTMLElement, content: React.ReactNode) => {
    const r = el.getBoundingClientRect();
    setTip({ x: r.left + r.width / 2, y: r.top, content });
  };
  const node = tip ? (
    <div
      role="tooltip"
      className="pointer-events-none fixed z-50 -translate-x-1/2 -translate-y-full whitespace-nowrap rounded-md border border-border bg-surface-raised px-2.5 py-1.5 text-2xs text-ink shadow-overlay"
      style={{ left: tip.x, top: tip.y - 6 }}
    >
      {tip.content}
    </div>
  ) : null;
  return { show, hide: () => setTip(null), node };
}

function CheckTip({ c, kind }: { c: CheckResult; kind: CellKind }) {
  const label = kind === "up" ? "Up" : kind === "slow" ? "Up (slow)" : c.status === "timeout" ? "Timeout" : "Down";
  const color = kind === "up" ? "text-up" : kind === "slow" ? "text-warn" : "text-down";
  return (
    <div className="space-y-0.5">
      <p className="font-medium">{fmtDateTime(c.checked_at)}</p>
      <p>
        <span className={`font-semibold ${color}`}>{label}</span>
        {" · "}HTTP {c.http_status_code ?? "—"}
        {" · "}{fmtMs(c.duration_ms)}
      </p>
      {c.error_class && c.error_class !== "none" ? (
        <p className="max-w-[260px] truncate text-down">
          {ERROR_CLASS_LABEL[c.error_class] ?? c.error_class}
          {c.error_detail ? `: ${c.error_detail}` : ""}
        </p>
      ) : null}
      {c.region ? <p className="text-ink-faint">region {c.region}</p> : null}
    </div>
  );
}

/**
 * GitHub contribution-style grid: one square per check, oldest top-left,
 * newest bottom-right, filled column by column (7 rows). Empty slots pad the
 * front so the grid is always full-width and the newest check sits at the end.
 */
export function CheckGrid({ checks, rows = 7, capacity }: { checks: CheckResult[]; rows?: number; capacity?: number }) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const [cols, setCols] = useState(30);
  const tip = useTooltip();
  const CELL = 12;
  const GAP = 3;

  // Fit as many columns as the container allows.
  useLayoutEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      setCols(Math.max(8, Math.floor((el.clientWidth + GAP) / (CELL + GAP))));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const slow = useMemo(() => slowThreshold(checks), [checks]);
  const total = capacity ?? cols * rows;
  // API returns newest-first; render oldest → newest.
  const ordered = useMemo(() => [...checks].slice(0, total).reverse(), [checks, total]);
  const pad = Math.max(0, total - ordered.length);

  return (
    <div ref={wrapRef} className="w-full" onMouseLeave={tip.hide}>
      <div
        className="grid grid-flow-col"
        style={{
          gridTemplateRows: `repeat(${rows}, ${CELL}px)`,
          gridAutoColumns: `${CELL}px`,
          gap: GAP,
        }}
      >
        {Array.from({ length: pad }).map((_, i) => (
          <span key={`pad-${i}`} className={`rounded-[3px] ${CELL_CLASS.empty}`} />
        ))}
        {ordered.map((c, i) => {
          const kind = classify(c, slow);
          return (
            <span
              key={`${c.checked_at}-${i}`}
              tabIndex={0}
              aria-label={`${fmtDateTime(c.checked_at)}: ${c.status}, ${fmtMs(c.duration_ms)}`}
              onMouseEnter={(e) => tip.show(e.currentTarget, <CheckTip c={c} kind={kind} />)}
              onFocus={(e) => tip.show(e.currentTarget, <CheckTip c={c} kind={kind} />)}
              onBlur={tip.hide}
              className={`cursor-default rounded-[3px] outline-none transition-transform hover:scale-125 hover:ring-2 focus-visible:ring-2 focus-visible:ring-accent ${CELL_CLASS[kind]}`}
            />
          );
        })}
      </div>
      {tip.node}
    </div>
  );
}

export function GridLegend() {
  const items: { kind: CellKind; label: string }[] = [
    { kind: "up", label: "Up" },
    { kind: "slow", label: "Slow" },
    { kind: "down", label: "Down" },
    { kind: "empty", label: "No check" },
  ];
  return (
    <div className="flex flex-wrap items-center gap-3 text-2xs text-ink-faint">
      {items.map((i) => (
        <span key={i.kind} className="inline-flex items-center gap-1.5">
          <span className={`h-2.5 w-2.5 rounded-[3px] ${CELL_CLASS[i.kind]}`} />
          {i.label}
        </span>
      ))}
    </div>
  );
}

/** Compact one-line strip of the latest checks, for list rows. */
export function CheckStrip({ checks, count = 30 }: { checks: CheckResult[]; count?: number }) {
  const tip = useTooltip();
  const slow = useMemo(() => slowThreshold(checks), [checks]);
  const ordered = [...checks].slice(0, count).reverse();
  const pad = Math.max(0, count - ordered.length);
  return (
    <div className="flex h-6 items-stretch gap-[2px]" onMouseLeave={tip.hide}>
      {Array.from({ length: pad }).map((_, i) => (
        <span key={`p${i}`} className="w-[5px] rounded-[2px] bg-surface-sunken ring-1 ring-inset ring-border/60" />
      ))}
      {ordered.map((c, i) => {
        const kind = classify(c, slow);
        return (
          <span
            key={`${c.checked_at}-${i}`}
            onMouseEnter={(e) => tip.show(e.currentTarget, <CheckTip c={c} kind={kind} />)}
            className={`w-[5px] rounded-[2px] transition-opacity hover:opacity-70 ${CELL_CLASS[kind].split(" ")[0]}`}
          />
        );
      })}
      {tip.node}
    </div>
  );
}

/**
 * Availability bar from aggregated buckets (works for 7/30/90 day windows
 * where per-check squares would be too many). Each bar = one bucket.
 */
export function AvailabilityBar({ series }: { series: UptimeBucket[] }) {
  const tip = useTooltip();
  if (series.length === 0) return null;
  return (
    <div className="flex h-8 items-stretch gap-[2px]" onMouseLeave={tip.hide}>
      {series.map((b) => {
        const pct = b.checks_total ? (b.checks_up / b.checks_total) * 100 : null;
        const cls =
          pct == null ? "bg-surface-sunken" : pct >= 100 ? "bg-up" : pct >= 95 ? "bg-warn" : "bg-down";
        return (
          <span
            key={b.bucket_start}
            onMouseEnter={(e) =>
              tip.show(
                e.currentTarget,
                <div className="space-y-0.5">
                  <p className="font-medium">{fmtDateTime(b.bucket_start)}</p>
                  <p>
                    {pct == null ? "No data" : `${pct.toFixed(2)}% up`} · {b.checks_up}/{b.checks_total} checks
                  </p>
                  <p className="text-ink-faint">avg {fmtMs(Math.round(b.avg_response_ms))}</p>
                </div>,
              )
            }
            className={`min-w-[2px] flex-1 rounded-[2px] transition-opacity hover:opacity-70 ${cls}`}
          />
        );
      })}
      {tip.node}
    </div>
  );
}
