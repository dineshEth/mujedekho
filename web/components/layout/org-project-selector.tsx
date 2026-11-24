"use client";

import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { apiPath } from "@/lib/api";
import type { Project } from "@/lib/types";
import { useWorkspace } from "@/lib/workspace";

/** Organization / project pair selector in the topbar. Selection drives every
 * page's API calls; persisted per browser via the workspace context. */
export function OrgProjectSelector() {
  const ws = useWorkspace();
  const [projects, setProjects] = useState<Project[]>(ws.projects);
  const [open, setOpen] = useState<"org" | "project" | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => setProjects(ws.projects), [ws.projects]);

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

  async function createProject() {
    const name = window.prompt("New project name");
    if (!name || !ws.org) return;
    try {
      const res = await api<Project>(apiPath.projects(ws.org.id), {
        method: "POST",
        body: JSON.stringify({ name }),
      });
      ws.setOrg(ws.org.id); // reloads projects through the context
      setProjects((p) => [...p, res]);
      setOpen(null);
    } catch (e) {
      window.alert(e instanceof Error ? e.message : "Failed to create project");
    }
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
        <MenuItem onClick={() => void createProject()}>+ New project</MenuItem>
      </MenuButton>
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
