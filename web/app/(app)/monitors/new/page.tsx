"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ErrorState } from "@/components/ui/feedback";
import { MonitorForm, type MonitorPayload } from "@/components/monitor/monitor-form";
import { api, apiPath } from "@/lib/api";
import { useWorkspace } from "@/lib/workspace";
import type { Monitor } from "@/lib/types";

export default function NewMonitorPage() {
  const ws = useWorkspace();
  const router = useRouter();

  async function createAndArm(payload: MonitorPayload) {
    if (!ws.org || !ws.project) throw new Error("Select an organization and project first.");
    const monitor = await api<Monitor>(apiPath.monitors(ws.org.id, ws.project.id), {
      method: "POST",
      body: JSON.stringify(payload),
    });
    // Backend creates monitors paused; arm it immediately so checks begin.
    await api(apiPath.monitorAction(ws.org.id, ws.project.id, monitor.id, "resume"), {
      method: "POST",
      body: JSON.stringify({}),
    });
    router.push(`/monitors/${monitor.id}`);
  }

  if (ws.loading) return null;
  if (!ws.org || !ws.project) {
    return <ErrorState message="Create or select an organization and project before adding a monitor." />;
  }

  return (
    <div className="mx-auto max-w-2xl">
      <p className="mb-2 text-xs text-ink-muted">
        <Link href="/monitors" className="hover:text-ink">Monitors</Link> / New
      </p>
      <h1 className="mb-1 text-lg font-semibold">Add monitor</h1>
      <p className="mb-6 text-xs text-ink-muted">
        Adding to <span className="font-medium text-ink">{ws.project.name}</span>. Checks begin as soon as the monitor is
        created. Private and loopback URLs are rejected by SSRF guards.
      </p>
      <MonitorForm
        key={ws.project.id}
        submitLabel="Create and start monitoring"
        submittingLabel="Creating…"
        onSubmit={createAndArm}
      />
    </div>
  );
}
