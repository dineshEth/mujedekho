"use client";

import { useEffect, useState } from "react";

/** Toggles the `dark` class on <html>; persisted in localStorage. Renders
 * nothing until mounted to avoid hydration mismatch of the icon. */
export function ThemeToggle() {
  const [dark, setDark] = useState<boolean | null>(null);

  useEffect(() => {
    setDark(document.documentElement.classList.contains("dark"));
  }, []);

  function toggle() {
    const next = !dark;
    setDark(next);
    document.documentElement.classList.toggle("dark", next);
    try {
      window.localStorage.setItem("theme", next ? "dark" : "light");
    } catch {
      /* private mode: ignore */
    }
  }

  if (dark === null) return <div className="h-9 w-9" aria-hidden />;

  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={dark ? "Switch to light theme" : "Switch to dark theme"}
      className="inline-flex h-9 w-9 items-center justify-center rounded-md text-ink-muted transition-colors hover:bg-surface-sunken hover:text-ink"
    >
      {dark ? (
        <svg viewBox="0 0 20 20" fill="none" className="h-[18px] w-[18px]" aria-hidden><circle cx="10" cy="10" r="3.5" stroke="currentColor" strokeWidth="1.5"/><path d="M10 2v2m0 12v2M2 10h2m12 0h2M4.3 4.3l1.4 1.4m8.6 8.6 1.4 1.4m0-11.4-1.4 1.4M5.7 14.3l-1.4 1.4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>
      ) : (
        <svg viewBox="0 0 20 20" fill="none" className="h-[18px] w-[18px]" aria-hidden><path d="M16 12.5A6.5 6.5 0 0 1 7.5 4a7 7 0 1 0 8.5 8.5Z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/></svg>
      )}
    </button>
  );
}
