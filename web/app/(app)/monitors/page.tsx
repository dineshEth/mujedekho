"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { Card, CardHeader, CardBody, StatCard } from "@/components/ui/card";
import { StateBadge } from "@/components/ui/state-badge";
import { EmptyState, ErrorState, TableSkeleton } from "@/components/ui/feedback";
import { Input, Select } from "@/components/ui/input";
import { CheckStrip } from "@/components/monitor/check-grid";
import { fmtInterval, fmtMs, fmtRelative } from "@/lib/format";
import { apiPath } from "@/lib/api";
import { useApi } from "@/lib/use-api";
import { useWorkspace } from "@/lib/workspace";
import type { CheckResult, Monitor } from "@/lib/types";

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
            <th>Recent checks</th>
            <th className="text-right">Uptime</th>
            <th className="text-right">Response</th>
            <th className="text-right">Interval</th>
          </tr>
        </thead>
        <tbody>
          {list.map((m) => <MonitorRow key={m.id} m={m} />)}
        </tbody>
      </table>
    </div>
  );
}

/** One row; fetches its own last 30 checks so the strip stays live. */
function MonitorRow({ m }: { m: Monitor }) {
  const ws = useWorkspace();
  const history = useApi<{ checks: CheckResult[] }>(
    ws.org && ws.project ? apiPath.history(ws.org.id, ws.project.id, m.id, "?limit=30&window_minutes=129600") : null,
    { pollMs: 30_000 },
  );
  const checks = history.data?.checks ?? [];
  const up = checks.filter((c) => c.status === "up").length;
  const pct = checks.length ? (up / checks.length) * 100 : null;
  const last = checks[0];
  return (
    <tr className="row-link">
      <td className="max-w-[280px]">
        <Link href={`/monitors/${m.id}`} className="block">
          <span className="block truncate font-medium text-ink">{m.name}</span>
          <span className="block truncate font-mono text-2xs text-ink-faint">{m.url}</span>
        </Link>
      </td>
      <td><StateBadge state={m.state} /></td>
      <td>
        {history.loading ? <div className="h-6 w-[210px] animate-pulse rounded bg-surface-sunken" /> : <CheckStrip checks={checks} />}
      </td>
      <td className={`text-right tabular-nums ${pct == null ? "text-ink-faint" : pct >= 99 ? "text-up" : pct >= 95 ? "text-warn" : "text-down"}`}>
        {pct == null ? "—" : `${pct.toFixed(pct === 100 ? 0 : 1)}%`}
      </td>
      <td className="text-right">
        <span className="tabular-nums">{last ? fmtMs(last.duration_ms) : "—"}</span>
        <span className="block text-2xs text-ink-faint">{last ? fmtRelative(last.checked_at) : "no checks"}</span>
      </td>
      <td className="text-right tabular-nums text-ink-muted">{fmtInterval(m.interval_seconds)}</td>
    </tr>
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
