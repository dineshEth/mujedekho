"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { Card, CardHeader, CardBody, StatCard } from "@/components/ui/card";
import { StateBadge } from "@/components/ui/state-badge";
import { EmptyState, ErrorState, TableSkeleton } from "@/components/ui/feedback";
import { Input, Select } from "@/components/ui/input";
import { fmtRelative, fmtUptime } from "@/lib/format";
import { apiPath } from "@/lib/api";
import { useApi } from "@/lib/use-api";
import { useWorkspace } from "@/lib/workspace";
import type { Monitor } from "@/lib/types";

type StateFilter = "all" | "up" | "down" | "paused";

export default function MonitorsPage() {
  const ws = useWorkspace();
  const orgID = ws.org?.id ?? "";
  const projID = ws.project?.id ?? "";
  const ready = !ws.loading && !!orgID && !!projID;

  const [search, setSearch] = useState("");
  const [state, setState] = useState<StateFilter>("all");

  // Debounce search into the API query.
  const debounced = useDebounced(search, 300);
  const query = useMemo(() => {
    const q = new URLSearchParams({ limit: "200" });
    if (debounced) q.set("search", debounced);
    if (state !== "all") q.set("state", state);
    return q.toString();
  }, [debounced, state]);

  const monitors = useApi<{ monitors: Monitor[]; total: number }>(
    ready ? `${apiPath.monitors(orgID, projID)}?${query}` : null,
    { pollMs: 30_000 },
  );

  const list = monitors.data?.monitors ?? [];
  const up = list.filter((m) => m.state === "up").length;
  const down = list.filter((m) => m.state === "down").length;
  const paused = list.filter((m) => m.state === "paused").length;

  return (
    <div>
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-lg font-semibold">Monitors</h1>
          <p className="text-xs text-ink-muted">HTTP availability checks for {ws.project?.name ?? "this project"}.</p>
        </div>
        <Link
          href="/monitors/new"
          className="inline-flex items-center gap-1.5 rounded-md bg-accent px-3 py-2 text-xs font-semibold text-accent-ink hover:bg-accent-hover"
        >
          Add monitor
        </Link>
      </div>

      <div className="mb-6 grid gap-4 sm:grid-cols-3">
        <StatCard label="Up" value={up} tone="up" />
        <StatCard label="Down" value={down} tone={down > 0 ? "down" : "neutral"} />
        <StatCard label="Paused" value={paused} />
      </div>

      <Card>
        <CardHeader
          title="All monitors"
          subtitle={monitors.data ? `${monitors.data.total} total` : undefined}
          actions={
            <div className="flex items-center gap-2">
              <Input
                placeholder="Search name or URL…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="!w-56 !py-1.5 text-xs"
                aria-label="Search monitors"
              />
              <Select
                aria-label="Filter by state"
                value={state}
                onValueChange={(v) => setState(v as StateFilter)}
                className="!py-1.5 text-xs"
                options={[
                  { value: "all", label: "All states" },
                  { value: "up", label: "Up" },
                  { value: "down", label: "Down" },
                  { value: "paused", label: "Paused" },
                ]}
              />
            </div>
          }
        />
        <MonitorsTable monitors={monitors} />
      </Card>
    </div>
  );
}

function MonitorsTable({ monitors }: { monitors: ReturnType<typeof useApi<{ monitors: Monitor[]; total: number }>> }) {
  if (monitors.error) {
    return <ErrorState message={monitors.error.message} onRetry={() => void monitors.refresh()} />;
  }
  if (monitors.loading) return <TableSkeleton rows={6} />;

  const list = monitors.data?.monitors ?? [];
  if (list.length === 0) {
    return (
      <EmptyState
        title="No monitors found"
        message="Adjust the filters, or add a monitor to start checking availability."
        action={<Link href="/monitors/new" className="rounded-md bg-accent px-4 py-2 text-sm font-semibold text-accent-ink hover:bg-accent-hover">Add monitor</Link>}
      />
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="table-base">
        <thead>
          <tr>
            <th>Name</th>
            <th>State</th>
            <th>URL</th>
            <th className="text-right">Interval</th>
            <th className="text-right">Last change</th>
          </tr>
        </thead>
        <tbody>
          {list.map((m) => (
            <tr key={m.id} className="row-link">
              <td>
                <Link href={`/monitors/${m.id}`} className="block">
                  <span className="font-medium text-ink">{m.name}</span>
                  <span className="sr-only"> — open details</span>
                </Link>
              </td>
              <td><StateBadge state={m.state} /></td>
              <td><Link href={`/monitors/${m.id}`} className="block truncate font-mono text-2xs text-ink-muted max-w-[260px]">{m.url}</Link></td>
              <td className="text-right tabular-nums text-ink-muted">{m.interval_seconds}s</td>
              <td className="text-right text-2xs text-ink-faint">{fmtRelative(m.updated_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="px-4 py-2 text-2xs text-ink-faint">
        Uptime details are per monitor — open one to see its {fmtUptime(null) === "no data" ? "24h/7d/30d/90d" : ""} windows.
      </p>
    </div>
  );
}

function useDebounced(value: string, delayMs: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(t);
  }, [value, delayMs]);
  return debounced;
}
