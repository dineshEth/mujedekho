"use client";

import Link from "next/link";
import { Card, CardHeader, CardBody, StatCard } from "@/components/ui/card";
import { StateBadge } from "@/components/ui/state-badge";
import { EmptyState, ErrorState, TableSkeleton } from "@/components/ui/feedback";
import { fmtRelative, fmtUptime, fmtMs } from "@/lib/format";
import { api, apiPath, ApiError } from "@/lib/api";
import { useApi } from "@/lib/use-api";
import { useWorkspace } from "@/lib/workspace";
import type { Monitor, Incident } from "@/lib/types";

/** Dashboard aggregates client-side from project endpoints: the backend does
 * not (yet) expose a dedicated /dashboard roll-up endpoint. Totals come from
 * the monitors list; recent incidents from the incidents endpoint. */
export default function DashboardPage() {
  const ws = useWorkspace();

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold">Dashboard</h1>
          <p className="text-xs text-ink-muted">
            {ws.org ? <>Overview for <span className="font-medium">{ws.org.name}</span>{ws.project ? <> / {ws.project.name}</> : null}</> : null}
          </p>
        </div>
      </div>
      <DashboardBody wsReady={!ws.loading && !!ws.org && !!ws.project} />
    </div>
  );
}

function DashboardBody({ wsReady }: { wsReady: boolean }) {
  const ws = useWorkspace();
  const orgID = ws.org?.id ?? "";
  const projID = ws.project?.id ?? "";

  const monitors = useApi<{ monitors: Monitor[]; total: number }>(
    wsReady ? apiPath.monitors(orgID, projID) : null,
    { pollMs: 30_000 },
  );
  const incidents = useApi<{ incidents: Incident[]; total: number }>(
    wsReady ? `${apiPath.incidents(orgID, projID)}?limit=8` : null,
    { pollMs: 30_000 },
  );

  if (!wsReady) return null;

  if (monitors.error || incidents.error) {
    return (
      <ErrorState
        message={monitors.error?.message ?? incidents.error?.message ?? "Failed to load dashboard"}
        onRetry={() => {
          void monitors.refresh();
          void incidents.refresh();
        }}
      />
    );
  }

  if (monitors.loading) {
    return <TableSkeleton rows={4} />;
  }

  const list = monitors.data?.monitors ?? [];
  if (list.length === 0) {
    return (
      <EmptyState
        title="No monitors yet"
        message='Create your first monitor to start tracking availability, response time and incidents. Tip: press "New monitor" in the top bar.'
        action={<Link href="/monitors/new" className="rounded-md bg-accent px-4 py-2 text-sm font-semibold text-accent-ink hover:bg-accent-hover">Add monitor</Link>}
      />
    );
  }

  const up = list.filter((m) => m.state === "up").length;
  const down = list.filter((m) => m.state === "down").length;
  const paused = list.filter((m) => m.state === "paused").length;
  const incidentsList = incidents.data?.incidents ?? [];
  const openIncidents = incidentsList.filter((i) => i.status === "open");

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Monitors" value={list.length} hint={`${up} up · ${down} down · ${paused} paused`} />
        <StatCard label="Operational" value={up} tone="up" />
        <StatCard label="Down" value={down} tone={down > 0 ? "down" : "neutral"} />
        <StatCard label="Open incidents" value={openIncidents.length} tone={openIncidents.length > 0 ? "down" : "neutral"} />
      </div>

      <div className="grid gap-6 xl:grid-cols-5">
        <Card className="xl:col-span-3">
          <CardHeader title="Monitors" subtitle="Latest observed state" actions={<Link href="/monitors" className="text-xs font-medium text-accent hover:underline">View all</Link>} />
          <div className="overflow-x-auto">
            <table className="table-base">
              <thead>
                <tr><th>Monitor</th><th>State</th><th className="text-right">Interval</th></tr>
              </thead>
              <tbody>
                {list.slice(0, 8).map((m) => (
                  <tr key={m.id} className="row-link">
                    <td>
                      <Link href={`/monitors/${m.id}`} className="block">
                        <span className="font-medium text-ink">{m.name}</span>
                        <span className="block truncate text-2xs text-ink-faint">{m.url}</span>
                      </Link>
                    </td>
                    <td><StateBadge state={m.state} /></td>
                    <td className="text-right tabular-nums text-ink-muted">{m.interval_seconds}s</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>

        <Card className="xl:col-span-2">
          <CardHeader title="Recent incidents" subtitle="Most recent across the project" actions={<Link href="/incidents" className="text-xs font-medium text-accent hover:underline">All incidents</Link>} />
          {incidents.loading ? (
            <div className="space-y-3 p-4">
              {[0, 1, 2].map((i) => <div key={i} className="h-9 animate-pulse rounded-md bg-surface-sunken" />)}
            </div>
          ) : incidentsList.length === 0 ? (
            <CardBody className="py-8 text-center text-xs text-ink-muted">No incidents recorded.</CardBody>
          ) : (
            <ul className="divide-y divide-border/60">
              {incidentsList.slice(0, 6).map((i) => (
                <li key={i.id} className="px-5 py-3">
                  <IncidentRow incident={i} />
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
    </div>
  );
}

function IncidentRow({ incident }: { incident: Incident }) {
  const ws = useWorkspace();
  const orgID = ws.org?.id ?? "";
  const projID = ws.project?.id ?? "";
  const monitor = useMonitorLite(orgID, projID, incident.monitor_id);

  return (
    <Link href={`/incidents`} className="block">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">
            {monitor?.name ?? incident.monitor_id.slice(0, 8)}
          </p>
          <p className="text-2xs text-ink-faint">
            Started {fmtRelative(incident.started_at)}
            {incident.status === "resolved" ? ` · resolved ${fmtRelative(incident.resolved_at ?? null)}` : ""}
          </p>
        </div>
        {incident.status === "open"
          ? <span className="shrink-0 text-2xs font-semibold uppercase tracking-wide text-down">open</span>
          : <span className="shrink-0 text-2xs font-semibold uppercase tracking-wide text-ink-faint">resolved</span>}
      </div>
    </Link>
  );
}

/** Lightweight fetch of monitor names for incident rows (no cache layer yet). */
function useMonitorLite(orgID: string, projID: string, monitorID: string) {
  const monitors = useApi<{ monitors: Monitor[] }>(apiPath.monitors(orgID, projID));
  return monitors.data?.monitors?.find((m) => m.id === monitorID) ?? null;
}
