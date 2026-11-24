import type { ReactNode } from "react";

export type BadgeTone = "up" | "down" | "paused" | "warn" | "neutral";

const tones: Record<BadgeTone, string> = {
  up: "bg-up/10 text-up ring-1 ring-inset ring-up/30",
  down: "bg-down/10 text-down ring-1 ring-inset ring-down/30",
  paused: "bg-paused/10 text-ink-muted ring-1 ring-inset ring-paused/40",
  warn: "bg-warn/10 text-warn ring-1 ring-inset ring-warn/30",
  neutral: "bg-surface-sunken text-ink-muted ring-1 ring-inset ring-border",
};

export function Badge({ tone = "neutral", children }: { tone?: BadgeTone; children: ReactNode }) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-2xs font-semibold uppercase tracking-wide ${tones[tone]}`}
    >
      {children}
    </span>
  );
}

const DOT: Record<BadgeTone, string> = {
  up: "bg-up",
  down: "bg-down",
  paused: "bg-paused",
  warn: "bg-warn",
  neutral: "bg-ink-faint",
};

export function StatusDot({ tone, pulse }: { tone: BadgeTone; pulse?: boolean }) {
  return (
    <span className="relative inline-flex h-2.5 w-2.5">
      {pulse ? <span className={`absolute inline-flex h-full w-full animate-ping rounded-full opacity-60 ${DOT[tone]}`} /> : null}
      <span className={`relative inline-flex h-2.5 w-2.5 rounded-full ${DOT[tone]}`} />
    </span>
  );
}
