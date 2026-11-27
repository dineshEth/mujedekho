"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useWorkspace } from "@/lib/workspace";
import { NewProjectDialog } from "@/components/projects/new-project-dialog";

/** Organization / project pair selector in the topbar. Selection drives every
 * page's API calls; persisted per browser via the workspace context. */
export function OrgProjectSelector() {
  const ws = useWorkspace();
  const projects = ws.projects;
  const [creating, setCreating] = useState(false);
  const [open, setOpen] = useState<"org" | "project" | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(null);
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  if (!ws.org) {
    return <span className="text-xs text-ink-faint">No organization</span>;
  }

  function pickOrg(id: string) {
    ws.setOrg(id);
    setOpen(null);
  }

  async function pickProject(id: string) {
    ws.setProject(id);
    setOpen(null);
  }

  return (
    <div className="flex items-center gap-1 text-sm" ref={ref}>
      <MenuButton
        label={ws.org.name}
        open={open === "org"}
        onToggle={() => setOpen(open === "org" ? null : "org")}
      >
        {ws.orgs.map((o) => (
          <MenuItem key={o.id} active={o.id === ws.org?.id} onClick={() => pickOrg(o.id)}>
            {o.name}
          </MenuItem>
        ))}
      </MenuButton>

      <span className="text-ink-faint">/</span>

      <MenuButton
        label={ws.project?.name ?? "Select project"}
        muted={!ws.project}
        open={open === "project"}
        onToggle={() => setOpen(open === "project" ? null : "project")}
      >
        {projects.map((p) => (
          <MenuItem key={p.id} active={p.id === ws.project?.id} onClick={() => void pickProject(p.id)}>
            {p.name}
          </MenuItem>
        ))}
        <div className="my-1 border-t border-border" />
        <MenuItem onClick={() => { setOpen(null); setCreating(true); }}>+ New project</MenuItem>
        <Link href="/projects" onClick={() => setOpen(null)} role="menuitem"
          className="block w-full px-3 py-1.5 text-left text-sm text-ink-muted hover:bg-surface-sunken hover:text-ink">
          Manage projects
        </Link>
      </MenuButton>
      <NewProjectDialog open={creating} onClose={() => setCreating(false)} />
    </div>
  );
}

function MenuButton({
  label,
  muted,
  open,
  onToggle,
  children,
}: {
  label: string;
  muted?: boolean;
  open: boolean;
  onToggle: () => void;
  children: React.ReactNode;
}) {
  return (
    <div className="relative">
      <button
        type="button"
        onClick={onToggle}
        aria-haspopup="menu"
        aria-expanded={open}
        className={`flex items-center gap-1 rounded-md px-2.5 py-1.5 font-medium transition-colors hover:bg-surface-sunken ${
          muted ? "text-ink-faint" : "text-ink"
        }`}
      >
        {label}
        <svg viewBox="0 0 12 12" className={`h-3 w-3 text-ink-faint transition-transform ${open ? "rotate-180" : ""}`} fill="none" aria-hidden>
          <path d="m2.5 4.5 3.5 3 3.5-3" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>
      {open ? (
        <div className="absolute left-0 z-20 mt-1 min-w-44 overflow-hidden rounded-lg border border-border bg-surface-raised py-1 shadow-overlay animate-fade-in" role="menu">
          {children}
        </div>
      ) : null}
    </div>
  );
}

function MenuItem({ active, onClick, children }: { active?: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className={`block w-full px-3 py-1.5 text-left text-sm hover:bg-surface-sunken ${
        active ? "text-accent" : "text-ink"
      }`}
    >
      {children}
    </button>
  );
}
