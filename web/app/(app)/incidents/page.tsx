"use client";

import { useState } from "react";
import { Card, CardHeader, StatCard } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { EmptyState, ErrorState, TableSkeleton } from "@/components/ui/feedback";
import { Select } from "@/components/ui/input";
import { fmtDateTime, fmtDuration, ERROR_CLASS_LABEL } from "@/lib/format";
import { apiPath } from "@/lib/api";
import { useApi } from "@/lib/use-api";
import { useWorkspace } from "@/lib/workspace";
import type { Incident } from "@/lib/types";

type StatusFilter = "" | "open" | "resolved";

export default function IncidentsPage() {
  const ws = useWorkspace();
  const orgID = ws.org?.id ?? "";
  const projID = ws.project?.id ?? "";
  const ready = !ws.loading && !!orgID && !!projID;

  const [status, setStatus] = useState<StatusFilter>("");

  const query = status ? `?status=${status}&limit=100` : "?limit=100";
  const incidents = useApi<{ incidents: Incident[]; total: number }>(
    ready ? `${apiPath.incidents(orgID, projID)}${query}` : null,
    { pollMs: 30_000 },
  );

  if (!ready) return null;

  const list = incidents.data?.incidents ?? [];
  const openCount = list.filter((i) => i.status === "open").length;

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-lg font-semibold">Incident management</h1>
        <p className="text-xs text-ink-muted">
          An incident opens when a monitor fails its confirmation threshold and resolves on the first successful check.
        </p>
      </div>

      <div className="mb-6 grid gap-4 sm:grid-cols-3">
        <StatCard label="Total" value={incidents.data?.total ?? (incidents.loading ? "…" : 0)} />
        <StatCard label="Open" value={openCount} tone={openCount > 0 ? "down" : "neutral"} />
        <StatCard label="Resolved (recent)" value={list.length - openCount} tone="up" />
      </div>

      <Card>
        <CardHeader
          title="Incident history"
          subtitle={incidents.data ? `${incidents.data.total} incidents` : undefined}
          actions={
            <Select
              aria-label="Filter incidents by status"
              value={status}
              onValueChange={(v) => setStatus(v as StatusFilter)}
              className="!py-1.5 text-xs"
              options={[
                { value: "", label: "All" },
                { value: "open", label: "Open" },
                { value: "resolved", label: "Resolved" },
              ]}
            />
          }
        />
        <IncidentsTable incidents={incidents} />
      </Card>
    </div>
  );
}

function IncidentsTable({ incidents }: { incidents: ReturnType<typeof useApi<{ incidents: Incident[]; total: number }>> }) {
  if (incidents.error) return <ErrorState message={incidents.error.message} onRetry={() => void incidents.refresh()} />;
  if (incidents.loading) return <TableSkeleton rows={8} />;

  const list = incidents.data?.incidents ?? [];
  if (list.length === 0) {
    return (
      <EmptyState
        title="No incidents"
        message="Nothing has failed its confirmation threshold yet. Incidents appear here as soon as a monitor goes down — with duration, cause and resolution."
      />
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="table-base">
        <thead>
          <tr>
            <th>Started</th>
            <th>Status</th>
            <th>Cause</th>
            <th className="text-right">Duration</th>
            <th className="text-right">Resolved</th>
          </tr>
        </thead>
        <tbody>
          {list.map((i) => (
            <tr key={i.id} className="row-link">
              <td className="whitespace-nowrap text-sm">{fmtDateTime(i.started_at)}</td>
              <td>
                {i.status === "open" ? <Badge tone="down">open</Badge> : <Badge tone="neutral">resolved</Badge>}
              </td>
              <td className="text-xs text-ink-muted">
                {i.cause_class ? ERROR_CLASS_LABEL[i.cause_class] ?? i.cause_class : "—"}
              </td>
              <td className="text-right tabular-nums text-xs">
                {i.status === "open" ? "ongoing" : fmtDuration(i.resolved_duration_s)}
              </td>
              <td className="text-right text-2xs text-ink-faint">
                {i.resolved_at ? fmtDateTime(i.resolved_at) : "—"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
