"use client";

import Link from "next/link";
import { OrgProjectSelector } from "./org-project-selector";
import { ThemeToggle } from "./theme-toggle";
import { UserMenu } from "./user-menu";

export function Topbar({ onMenu }: { onMenu: () => void }) {
  return (
    <header className="flex h-14 items-center gap-3 border-b border-border bg-surface px-4 sm:px-6">
      <button
        type="button"
        onClick={onMenu}
        aria-label="Open navigation"
        className="inline-flex h-9 w-9 items-center justify-center rounded-md text-ink-muted hover:bg-surface-sunken lg:hidden"
      >
        <svg viewBox="0 0 20 20" fill="none" className="h-5 w-5" aria-hidden>
          <path d="M3 5h14M3 10h14M3 15h14" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        </svg>
      </button>

      <OrgProjectSelector />

      <div className="flex-1" />

      <Link
        href="/monitors/new"
        className="hidden items-center gap-1.5 rounded-md bg-accent px-3 py-1.5 text-xs font-semibold text-accent-ink transition-colors hover:bg-accent-hover sm:inline-flex"
      >
        <svg viewBox="0 0 14 14" fill="none" className="h-3 w-3" aria-hidden>
          <path d="M7 2v10M2 7h10" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" />
        </svg>
        New monitor
      </Link>

      <ThemeToggle />
      <UserMenu />
    </header>
  );
}
