"use client";

import { useState, type FormEvent } from "react";
import { Modal } from "@/components/ui/modal";
import { Button } from "@/components/ui/button";
import { Input, Label } from "@/components/ui/input";
import { useWorkspace } from "@/lib/workspace";
import type { Project } from "@/lib/types";

const SUGGESTIONS = ["Production", "Staging", "Marketing sites", "Internal tools", "APIs"];

export function NewProjectDialog({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated?: (p: Project) => void;
}) {
  const ws = useWorkspace();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const taken = new Set(ws.projects.map((p) => p.name.toLowerCase()));

  function close() {
    setName("");
    setError(null);
    onClose();
  }

  async function submit(e?: FormEvent) {
    e?.preventDefault();
    const n = name.trim();
    if (!n) return;
    setBusy(true);
    setError(null);
    try {
      const p = await ws.createProject(n);
      onCreated?.(p);
      close();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create project");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={close}
      title="New project"
      subtitle={ws.org ? `Group monitors inside ${ws.org.name}` : undefined}
      footer={
        <>
          <Button variant="ghost" size="sm" onClick={close} disabled={busy}>Cancel</Button>
          <Button size="sm" onClick={() => void submit()} disabled={busy || !name.trim()}>
            {busy ? "Creating…" : "Create project"}
          </Button>
        </>
      }
    >
      <form onSubmit={submit} className="space-y-3">
        <div>
          <Label htmlFor="p-name" hint="1–100 characters">Project name</Label>
          <Input id="p-name" autoFocus maxLength={100} value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Staging" />
        </div>
        <div className="flex flex-wrap gap-1.5">
          {SUGGESTIONS.filter((s) => !taken.has(s.toLowerCase())).map((s) => (
            <button key={s} type="button" onClick={() => setName(s)}
              className="rounded-full border border-border px-2.5 py-0.5 text-2xs text-ink-muted hover:border-accent hover:text-accent">
              {s}
            </button>
          ))}
        </div>
        {error ? <p role="alert" className="rounded-md bg-down/10 px-3 py-2 text-xs text-down">{error}</p> : null}
      </form>
    </Modal>
  );
}
