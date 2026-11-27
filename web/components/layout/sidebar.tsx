"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useWorkspace } from "@/lib/workspace";

const NAV = [
  { href: "/dashboard", label: "Dashboard", icon: "grid" },
  { href: "/monitors", label: "Monitors", icon: "monitor" },
  { href: "/projects", label: "Projects", icon: "folder" },
  { href: "/incidents", label: "Incidents", icon: "alert" },
  { href: "/notifications", label: "Notifications", icon: "bell" },
  { href: "/status-pages", label: "Status Pages", icon: "page" },
  { href: "/settings", label: "Settings", icon: "gear" },
] as const;

function Icon({ name }: { name: (typeof NAV)[number]["icon"] }) {
  const cls = "h-4.5 w-4.5 h-[18px] w-[18px]";
  switch (name) {
    case "grid":
      return <svg viewBox="0 0 20 20" fill="none" className={cls} aria-hidden><rect x="3" y="3" width="6" height="6" rx="1" stroke="currentColor" strokeWidth="1.5"/><rect x="11" y="3" width="6" height="6" rx="1" stroke="currentColor" strokeWidth="1.5"/><rect x="3" y="11" width="6" height="6" rx="1" stroke="currentColor" strokeWidth="1.5"/><rect x="11" y="11" width="6" height="6" rx="1" stroke="currentColor" strokeWidth="1.5"/></svg>;
    case "monitor":
      return <svg viewBox="0 0 20 20" fill="none" className={cls} aria-hidden><rect x="2.5" y="4" width="15" height="10" rx="1.5" stroke="currentColor" strokeWidth="1.5"/><path d="M7 17h6M10 14v3" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>;
    case "folder":
      return <svg viewBox="0 0 20 20" fill="none" className={cls} aria-hidden><path d="M2.5 6a1.5 1.5 0 0 1 1.5-1.5h3.5l2 2H16A1.5 1.5 0 0 1 17.5 8v6.5A1.5 1.5 0 0 1 16 16H4a1.5 1.5 0 0 1-1.5-1.5V6Z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/></svg>;
    case "alert":
      return <svg viewBox="0 0 20 20" fill="none" className={cls} aria-hidden><path d="M10 3 2.5 16h15L10 3Z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/><path d="M10 8v4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/><circle cx="10" cy="14" r="0.8" fill="currentColor"/></svg>;
    case "bell":
      return <svg viewBox="0 0 20 20" fill="none" className={cls} aria-hidden><path d="M10 3a5 5 0 0 0-5 5v3l-1.5 3h13L15 11V8a5 5 0 0 0-5-5Z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/><path d="M8.5 16.5a1.5 1.5 0 0 0 3 0" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>;
    case "page":
      return <svg viewBox="0 0 20 20" fill="none" className={cls} aria-hidden><rect x="3" y="4" width="14" height="12" rx="1.5" stroke="currentColor" strokeWidth="1.5"/><path d="M3 8h14" stroke="currentColor" strokeWidth="1.5"/></svg>;
    case "gear":
      return <svg viewBox="0 0 20 20" fill="none" className={cls} aria-hidden><circle cx="10" cy="10" r="2.5" stroke="currentColor" strokeWidth="1.5"/><path d="M10 2.5v2M10 15.5v2M2.5 10h2M15.5 10h2M4.7 4.7l1.4 1.4M13.9 13.9l1.4 1.4M15.3 4.7l-1.4 1.4M6.1 13.9l-1.4 1.4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>;
  }
}

export function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname();
  const { org, project } = useWorkspace();

  return (
    <nav className="flex h-full flex-col bg-surface-sunken" aria-label="Main navigation">
      <div className="flex h-14 items-center gap-2.5 border-b border-border px-5">
        <span className="flex h-7 w-7 items-center justify-center rounded-md bg-accent font-mono text-xs font-bold text-accent-ink">24</span>
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold leading-tight">site24x7</p>
          <p className="truncate text-2xs text-ink-faint">
            {org ? `${org.name}${project ? ` / ${project.name}` : ""}` : "Website monitoring"}
          </p>
        </div>
      </div>
      <ul className="flex-1 space-y-0.5 overflow-y-auto p-3">
        {NAV.map((item) => {
          const active = pathname === item.href || pathname.startsWith(item.href + "/");
          return (
            <li key={item.href}>
              <Link
                href={item.href}
                onClick={onNavigate}
                aria-current={active ? "page" : undefined}
                className={`flex items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                  active
                    ? "bg-accent/10 text-accent"
                    : "text-ink-muted hover:bg-surface hover:text-ink"
                }`}
              >
                <Icon name={item.icon} />
                {item.label}
              </Link>
            </li>
          );
        })}
      </ul>
      <p className="border-t border-border px-5 py-3 text-2xs text-ink-faint">
        Website monitoring only
      </p>
    </nav>
  );
}
