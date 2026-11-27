"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import {
  ResponsiveContainer, LineChart, Line, XAxis, YAxis, Tooltip, CartesianGrid,
} from "recharts";
import { Card, CardHeader, CardBody, StatCard } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { StateBadge } from "@/components/ui/state-badge";
import { EmptyState, ErrorState, TableSkeleton, Skeleton } from "@/components/ui/feedback";
import { fmtDateTime, fmtDuration, fmtMs, fmtUptime, fmtRelative, fmtBytes, ERROR_CLASS_LABEL } from "@/lib/format";
import { api, apiPath, ApiError } from "@/lib/api";
import { useApi } from "@/lib/use-api";
import { useWorkspace } from "@/lib/workspace";
import type { CheckResult, Incident, Monitor, UptimeBucket, UptimeSummary } from "@/lib/types";

type WindowKey = "24h" | "7d" | "30d" | "90d";
const WINDOWS: { key: WindowKey; label: string }[] = [
  { key: "24h", label: "24h" },
  { key: "7d", label: "7 days" },
  { key: "30d", label: "30 days" },
  { key: "90d", label: "90 days" },
];

export default function MonitorDetailsPage() {
  const params = useParams<{ id: string }>();
  const monitorID = params.id;
  const ws = useWorkspace();
  const router = useRouter();
  const ready = !ws.loading && !!ws.org?.id && !!ws.project?.id;

  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<CheckResult | null>(null);

  const monitor = useApi<Monitor & { latest_check: CheckResult | null }>(
    ready ? apiPath.monitor(ws.org!.id, ws.project!.id, monitorID) : null,
    { pollMs: 15_000 },
  );

  const mon = monitor.data;

  async function doDelete() {
    if (!ws.org || !ws.project) return;
    if (!window.confirm("Delete this monitor? Checks stop immediately; history is kept for audit.")) return;
    setBusy(true);
    setActionError(null);
    try {
      await api(apiPath.monitor(ws.org.id, ws.project.id, monitorID), { method: "DELETE" });
      router.push("/monitors");
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Failed to delete monitor");
      setBusy(false);
    }
  }

  async function doAction(action: "pause" | "resume" | "test") {
    if (!ws.org || !ws.project) return;
    setBusy(true);
    setActionError(null);
    try {
      if (action === "test") {
        const res = await api<{ result: CheckResult }>(apiPath.monitorTest(ws.org.id, ws.project.id, monitorID), { method: "POST", body: JSON.stringify({}) });
        setTestResult(res.result);
      } else {
        await api(apiPath.monitorAction(ws.org.id, ws.project.id, monitorID, action), { method: "POST", body: JSON.stringify({}) });
        await monitor.refresh();
      }
    } catch (e) {
      setActionError(e instanceof Error ? e.message : `Failed to ${action} monitor`);
    } finally {
      setBusy(false);
    }
  }

  if (!ready) return null;
  if (monitor.error) {
    const notFound = monitor.error.status === 404;
    return <ErrorState message={notFound ? "Monitor not found in this project." : monitor.error.message} onRetry={() => void monitor.refresh()} />;
  }
  if (monitor.loading || !mon) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-72" />
        <div className="grid gap-4 sm:grid-cols-4">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-24" />)}</div>
        <Skeleton className="h-72" />
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2.5">
            <h1 className="truncate text-lg font-semibold">{mon.name}</h1>
            <StateBadge state={mon.state} />
          </div>
          <p className="mt-0.5 truncate font-mono text-xs text-ink-muted">{mon.url}</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button variant="secondary" size="sm" disabled={busy} onClick={() => void doAction("test")}>
            Run test now
          </Button>
          {mon.state === "paused" ? (
            <Button size="sm" disabled={busy} onClick={() => void doAction("resume")}>Resume</Button>
          ) : (
            <Button variant="secondary" size="sm" disabled={busy} onClick={() => void doAction("pause")}>Pause</Button>
          )}
          <Button variant="danger" size="sm" disabled={busy} onClick={() => void doDelete()}>Delete</Button>
        </div>
      </div>

      {actionError ? <p role="alert" className="mb-4 rounded-md bg-down/10 px-3 py-2 text-xs text-down">{actionError}</p> : null}

      {testResult ? <TestResultCard result={testResult} onDismiss={() => setTestResult(null)} /> : null}

      <MonitorSections orgID={ws.org!.id} projID={ws.project!.id} monitorID={monitorID} intervalSeconds={mon.interval_seconds} />

      <section className="mt-6">
        <Card>
          <CardHeader title="Configuration" subtitle="Values set at creation" />
          <CardBody className="grid gap-x-8 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
            <ConfigRow label="Type" value={mon.monitor_type} />
            <ConfigRow label="Interval" value={`${mon.interval_seconds}s`} />
            <ConfigRow label="Timeout" value={fmtMs(mon.timeout_ms)} />
            <ConfigRow label="Failure threshold" value={`${mon.failure_threshold} checks`} />
            <ConfigRow label="Method" value={mon.request_config?.method ?? "GET"} />
            <ConfigRow label="Follow redirects" value={mon.request_config?.follow_redirects === false ? "No" : "Yes"} />
            <ConfigRow label="Expected status" value={`${mon.request_config?.expected_status_min ?? 200}–${mon.request_config?.expected_status_max ?? 299}`} />
            <ConfigRow label="Consecutive failures" value={String(mon.consecutive_failures ?? 0)} />
            <ConfigRow label="Created" value={fmtDateTime(mon.created_at)} />
          </CardBody>
        </Card>
      </section>
    </div>
  );
}

function TestResultCard({ result, onDismiss }: { result: CheckResult; onDismiss: () => void }) {
  const ok = result.status === "up";
  return (
    <Card className="mb-6 border-l-4 border-l-accent">
      <CardBody>
        <div className="flex items-start justify-between">
          <div>
            <p className="text-sm font-semibold">
              Manual test:{" "}
              <span className={ok ? "text-up" : "text-down"}>{result.status}</span>
              {" "}in {fmtMs(result.duration_ms)}
            </p>
            <p className="mt-0.5 text-xs text-ink-muted">
              HTTP {result.http_status_code ?? "—"} · DNS {fmtMs(result.dns_ms)} · connect {fmtMs(result.connect_ms)} · TLS {fmtMs(result.tls_ms)} · TTFB {fmtMs(result.ttfb_ms)}
              {result.error_detail ? ` · ${result.error_detail}` : ""}
            </p>
          </div>
          <button type="button" onClick={onDismiss} className="text-ink-faint hover:text-ink" aria-label="Dismiss test result">✕</button>
        </div>
      </CardBody>
    </Card>
  );
}

function MonitorSections({ orgID, projID, monitorID, intervalSeconds }: { orgID: string; projID: string; monitorID: string; intervalSeconds: number }) {
  const [win, setWin] = useState<WindowKey>("24h");
  // Backend shape: { series: Bucket[] | null, window: UptimeSummary }.
  const uptime = useApi<{ series: UptimeBucket[] | null; window: UptimeSummary }>(
    `${apiPath.uptime(orgID, projID, monitorID, `?window=${win}`)}`,
    { pollMs: 60_000 },
  );
  const history = useApi<{ checks: CheckResult[]; next_cursor: string | null }>(
    `${apiPath.history(orgID, projID, monitorID, "?limit=30")}`,
    { pollMs: 30_000 },
  );
  const incidents = useApi<{ incidents: Incident[]; total: number }>(
    `${apiPath.incidents(orgID, projID, `?monitor_id=${monitorID}&limit=10`)}`,
  );

  const summary = uptime.data?.window;
  void intervalSeconds;

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label={`Uptime (${win})`}
          value={summary?.no_data || summary?.uptime_pct == null ? "no data" : fmtUptime(summary.uptime_pct)}
          tone={summary?.uptime_pct == null || summary?.no_data ? "neutral" : summary.uptime_pct >= 99.9 ? "up" : summary.uptime_pct >= 95 ? "neutral" : "down"}
          hint={summary ? `${summary.checks_total} checks · downtime ${fmtDuration(summary.downtime_seconds)}` : undefined}
        />
        <StatCard label="Checks total" value={summary?.checks_total ?? "—"} hint={`${summary?.checks_up ?? "—"} successful`} />
        <StatCard
          label="Avg response"
          value={averageMs(uptime.data?.series ?? undefined)}
          hint={`window ${win}`}
        />
        <StatCard label="Open incidents" value={(incidents.data?.incidents ?? []).filter((i) => i.status === "open").length} tone="neutral" />
      </div>

      <Card>
        <CardHeader
          title="Response time"
          subtitle="Aggregated per bucket; gaps mean no data in that period"
          actions={
            <div className="flex rounded-md border border-border p-0.5">
              {WINDOWS.map((w) => (
                <button
                  key={w.key} type="button"
                  onClick={() => setWin(w.key)}
                  aria-pressed={win === w.key}
                  className={`rounded px-2.5 py-1 text-2xs font-medium transition-colors ${
                    win === w.key ? "bg-accent text-accent-ink" : "text-ink-muted hover:text-ink"
                  }`}
                >
                  {w.label}
                </button>
              ))}
            </div>
          }
        />
        <CardBody>
          {uptime.loading ? <Skeleton className="h-64 w-full" /> :
           uptime.error ? <p className="text-xs text-down">{uptime.error.message}</p> :
           <UptimeChart series={uptime.data?.series ?? []} />}
        </CardBody>
      </Card>

      <div className="grid gap-6 xl:grid-cols-5">
        <Card className="xl:col-span-3">
          <CardHeader title="Recent checks" subtitle="Raw per-check results (latest first)" />
          <CheckHistory history={history} />
        </Card>

        <Card className="xl:col-span-2">
          <CardHeader
            title="Incidents"
            subtitle="For this monitor"
            actions={<Link href="/incidents" className="text-xs font-medium text-accent hover:underline">All incidents</Link>}
          />
          <IncidentsList incidents={incidents} />
        </Card>
      </div>
    </div>
  );
}

function UptimeChart({ series }: { series: UptimeBucket[] }) {
  const data = series.map((b) => ({
    t: new Date(b.bucket_start).getTime(),
    label: winLabel(new Date(b.bucket_start)),
    ms: Math.round(b.avg_response_ms),
    status: b.checks_total > 0 ? (b.checks_up >= b.checks_total ? "ok" : "degraded") : "nodata",
  }));

  if (data.length === 0) {
    return (
      <div className="flex h-64 items-center justify-center text-xs text-ink-muted">
        No response-time data in this window yet.
      </div>
    );
  }

  return (
    <div className="h-64" role="img" aria-label="Response time chart">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
          <CartesianGrid strokeDasharray="3 3" className="[&>line]:!stroke-border" stroke="rgb(var(--border))" />
          <XAxis
            dataKey="t"
            type="number"
            scale="time"
            domain={["dataMin", "dataMax"]}
            tickFormatter={(t: number) => winLabel(new Date(t))}
            tick={{ fontSize: 10, fill: "rgb(var(--ink-faint))" }}
            stroke="rgb(var(--border))"
          />
          <YAxis
            tick={{ fontSize: 10, fill: "rgb(var(--ink-faint))" }}
            stroke="rgb(var(--border))"
            tickFormatter={(ms: number) => (ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`)}
            width={56}
          />
          <Tooltip
            labelFormatter={(t) => fmtDateTime(new Date(Number(t)).toISOString())}
            formatter={(value) => [fmtMs(Number(value)), "avg response"]}
            contentStyle={{
              background: "rgb(var(--surface-raised))",
              border: "1px solid rgb(var(--border))",
              borderRadius: 8,
              fontSize: 12,
              color: "rgb(var(--ink))",
            }}
          />
          <Line type="monotone" dataKey="ms" stroke="rgb(var(--accent))" strokeWidth={1.5} dot={false} isAnimationActive={false} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

function winLabel(d: Date): string {
  const now = Date.now() - d.getTime();
  if (now < 26 * 3600_000) return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function averageMs(series: UptimeBucket[] | undefined): string {
  const pts = (series ?? []).filter((b) => b.checks_total > 0);
  if (pts.length === 0) return "—";
  const avg = pts.reduce((acc, b) => acc + b.avg_response_ms, 0) / pts.length;
  return fmtMs(Math.round(avg));
}

function CheckHistory({ history }: { history: ReturnType<typeof useApi<{ checks: CheckResult[]; next_cursor: string | null }>> }) {
  if (history.error) return <ErrorState message={history.error.message} onRetry={() => void history.refresh()} />;
  if (history.loading) return <TableSkeleton rows={6} />;
  const checks = history.data?.checks ?? [];
  if (checks.length === 0) return <EmptyState title="No checks yet" message="Results appear here once the worker completes the first check." />;
  return (
    <div className="overflow-x-auto max-h-96 overflow-y-auto">
      <table className="table-base">
        <thead className="sticky top-0 bg-surface-raised">
          <tr><th>Time</th><th>Status</th><th className="text-right">HTTP</th><th className="text-right">Total</th><th className="text-right">Detail</th></tr>
        </thead>
        <tbody>
          {checks.map((c, i) => (
            <tr key={`${c.checked_at}-${i}`} className="row-link">
              <td className="whitespace-nowrap text-2xs text-ink-muted">{fmtDateTime(c.checked_at)}</td>
              <td><StateBadge state={c.status} /></td>
              <td className="text-right tabular-nums">{c.http_status_code ?? "—"}</td>
              <td className="text-right tabular-nums">{fmtMs(c.duration_ms)}</td>
              <td className="max-w-[220px] truncate text-2xs text-ink-faint">
                {c.error_class && c.error_class !== "none"
                  ? `${ERROR_CLASS_LABEL[c.error_class] ?? c.error_class}${c.error_detail ? `: ${c.error_detail}` : ""}`
                  : fmtBytes(c.response_bytes)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function IncidentsList({ incidents }: { incidents: ReturnType<typeof useApi<{ incidents: Incident[]; total: number }>> }) {
  if (incidents.error) return <ErrorState message={incidents.error.message} onRetry={() => void incidents.refresh()} />;
  if (incidents.loading) return <TableSkeleton rows={3} />;
  const list = incidents.data?.incidents ?? [];
  if (list.length === 0) return <EmptyState title="No incidents" message="No outages recorded for this monitor." />;
  return (
    <ul className="divide-y divide-border/60">
      {list.map((i) => (
        <li key={i.id} className="px-5 py-3">
          <div className="flex items-center justify-between gap-2">
            <div className="min-w-0">
              <p className="text-sm font-medium">{fmtDateTime(i.started_at)}</p>
              <p className="text-2xs text-ink-faint">
                {i.status === "open" ? "Ongoing" : `Resolved in ${fmtDuration(i.resolved_duration_s)}`}
                {i.cause_class ? ` · ${ERROR_CLASS_LABEL[i.cause_class] ?? i.cause_class}` : ""}
              </p>
            </div>
            {i.status === "open" ? <Badge tone="down">open</Badge> : <Badge tone="neutral">resolved</Badge>}
          </div>
        </li>
      ))}
    </ul>
  );
}

function ConfigRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border/40 pb-2">
      <span className="text-xs text-ink-muted">{label}</span>
      <span className="text-xs font-medium text-ink">{value}</span>
    </div>
  );
}
