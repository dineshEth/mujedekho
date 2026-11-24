"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useWorkspace } from "@/lib/workspace";
import { ThemeToggle } from "./theme-toggle";

function initials(name: string, email: string): string {
  const src = name.trim() || email;
  const parts = src.split(/\s+/);
  const chars = parts.length >= 2 ? parts[0][0] + parts[1][0] : src.slice(0, 2);
  return chars.toUpperCase();
}

export function UserMenu() {
  const { user } = useWorkspace();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  async function logout() {
    await fetch("/api/auth/logout", { method: "POST" });
    router.replace("/login");
    router.refresh();
  }

  if (!user) return <div className="h-9 w-9" aria-hidden />;

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        className="flex h-9 w-9 items-center justify-center rounded-full bg-accent/10 text-xs font-semibold text-accent transition-colors hover:bg-accent/20"
      >
        {initials(user.full_name, user.email)}
      </button>
      {open ? (
        <div
          role="menu"
          className="absolute right-0 z-20 mt-2 w-56 overflow-hidden rounded-lg border border-border bg-surface-raised py-1 shadow-overlay animate-fade-in"
        >
          <div className="border-b border-border px-3 py-2">
            <p className="truncate text-sm font-medium">{user.full_name || "Signed in"}</p>
            <p className="truncate text-xs text-ink-muted">{user.email}</p>
          </div>
          <a
            role="menuitem"
            href="/settings/account"
            onClick={() => setOpen(false)}
            className="block px-3 py-2 text-sm text-ink hover:bg-surface-sunken"
          >
            Account settings
          </a>
          <button
            role="menuitem"
            type="button"
            onClick={logout}
            className="block w-full px-3 py-2 text-left text-sm text-down hover:bg-surface-sunken"
          >
            Sign out
          </button>
        </div>
      ) : null}
    </div>
  );
}
