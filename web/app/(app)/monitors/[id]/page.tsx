"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import {
  ResponsiveContainer, AreaChart, Area, XAxis, YAxis, Tooltip, CartesianGrid,
} from "recharts";
import { Card, CardHeader, CardBody, StatCard } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge, StatusDot } from "@/components/ui/badge";
import { StateBadge } from "@/components/ui/state-badge";
import { EmptyState, ErrorState, TableSkeleton, Skeleton } from "@/components/ui/feedback";
import { CheckGrid, GridLegend, AvailabilityBar, classify, slowThreshold, CELL_CLASS } from "@/components/monitor/check-grid";
import { MonitorForm, type MonitorPayload } from "@/components/monitor/monitor-form";
import { fmtDateTime, fmtDuration, fmtMs, fmtUptime, fmtRelative, fmtBytes, fmtInterval, ERROR_CLASS_LABEL } from "@/lib/format";
import { api, apiPath } from "@/lib/api";
import { useApi } from "@/lib/use-api";
import { useWorkspace } from "@/lib/workspace";
import type { CheckResult, Incident, Monitor, UptimeBucket, UptimeSummary } from "@/lib/types";

type WindowKey = "24h" | "7d" | "30d" | "90d";
const WINDOWS: { key: WindowKey; label: string }[] = [
  { key: "24h", label: "24h" },
  { key: "7d", label: "7d" },
  { key: "30d", label: "30d" },
  { key: "90d", label: "90d" },
];

type MonitorWithLatest = Monitor & { latest_check: CheckResult | null };
type HistoryResp = { checks: CheckResult[]; next_cursor: string | null };

export default function MonitorDetailsPage() {
  const params = useParams<{ id: string }>();
  const monitorID = params.id;
  const ws = useWorkspace();
  const router = useRouter();
  const ready = !ws.loading && !!ws.org?.id && !!ws.project?.id;
  const orgID = ws.org?.id ?? "";
  const projID = ws.project?.id ?? "";

  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<CheckResult | null>(null);
  const [editing, setEditing] = useState(false);

  const monitor = useApi<MonitorWithLatest>(ready ? apiPath.monitor(orgID, projID, monitorID) : null, { pollMs: 15_000 });
  // Last ~90 days of raw checks, capped at 500: feeds the grid and the table.
  const history = useApi<HistoryResp>(
    ready ? apiPath.history(orgID, projID, monitorID, "?limit=500&window_minutes=129600") : null,
    { pollMs: 20_000 },
  );

  const mon = monitor.data;

  async function doDelete() {
    if (!window.confirm("Delete this monitor? Checks stop immediately; history is kept for audit.")) return;
    setBusy(true);
    setActionError(null);
    try {
      await api(apiPath.monitor(orgID, projID, monitorID), { method: "DELETE" });
      router.push("/monitors");
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Failed to delete monitor");
      setBusy(false);
    }
  }

  async function doAction(action: "pause" | "resume" | "test") {
    setBusy(true);
    setActionError(null);
    try {
      if (action === "test") {
        const res = await api<{ result: CheckResult }>(apiPath.monitorTest(orgID, projID, monitorID), { method: "POST", body: JSON.stringify({}) });
        setTestResult(res.result);
        void history.refresh();
      } else {
        await api(apiPath.monitorAction(orgID, projID, monitorID, action), { method: "POST", body: JSON.stringify({}) });
        await monitor.refresh();
      }
    } catch (e) {
      setActionError(e instanceof Error ? e.message : `Failed to ${action} monitor`);
    } finally {
      setBusy(false);
    }
  }

  async function saveConfig(payload: MonitorPayload) {
    await api<Monitor>(apiPath.monitor(orgID, projID, monitorID), { method: "PUT", body: JSON.stringify(payload) });
    await monitor.refresh();
    setEditing(false);
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
        <Skeleton className="h-40" />
        <Skeleton className="h-72" />
      </div>
    );
  }

  const checks = history.data?.checks ?? [];
  const latest = checks[0] ?? mon.latest_check ?? null;

  return (
    <div className="space-y-6">
      <Header
        mon={mon}
        latest={latest}
        busy={busy}
        onTest={() => void doAction("test")}
        onPause={() => void doAction("pause")}
        onResume={() => void doAction("resume")}
        onDelete={() => void doDelete()}
        onEdit={() => setEditing(true)}
      />

      {actionError ? <p role="alert" className="rounded-md bg-down/10 px-3 py-2 text-xs text-down">{actionError}</p> : null}
      {testResult ? <TestResultCard result={testResult} onDismiss={() => setTestResult(null)} /> : null}

      {editing ? (
        <Card className="border-accent/50">
          <CardHeader title="Edit configuration" subtitle="Changes apply from the next scheduled check" />
          <CardBody>
            <MonitorForm initial={mon} compact submitLabel="Save changes" submittingLabel="Saving…"
              onSubmit={saveConfig} onCancel={() => setEditing(false)} />
          </CardBody>
        </Card>
      ) : null}

      <MonitorSections orgID={orgID} projID={projID} monitorID={monitorID} history={history} />

      <Card>
        <CardHeader
          title="Configuration"
          subtitle={`Last updated ${fmtRelative(mon.updated_at)}`}
          actions={!editing ? <Button variant="secondary" size="sm" onClick={() => setEditing(true)}>Edit</Button> : null}
        />
        <CardBody className="grid gap-x-8 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
          <ConfigRow label="Type" value={mon.monitor_type.toUpperCase()} />
          <ConfigRow label="Interval" value={fmtInterval(mon.interval_seconds)} />
          <ConfigRow label="Timeout" value={fmtMs(mon.timeout_ms)} />
          <ConfigRow label="Failure threshold" value={`${mon.failure_threshold} checks`} />
          <ConfigRow label="Method" value={mon.request_config?.method || "GET"} />
          <ConfigRow label="Follow redirects" value={mon.request_config?.follow_redirects === false ? "No" : "Yes"} />
          <ConfigRow label="Expected status" value={`${mon.request_config?.expected_status_min || 200}–${mon.request_config?.expected_status_max || 299}`} />
          <ConfigRow label="Custom headers" value={String(Object.keys(mon.request_config?.headers ?? {}).length)} />
          <ConfigRow label="Consecutive failures" value={String(mon.consecutive_failures ?? 0)} />
          <ConfigRow label="Created" value={fmtDateTime(mon.created_at)} />
        </CardBody>
      </Card>
    </div>
  );
}

function Header({
  mon, latest, busy, onTest, onPause, onResume, onDelete, onEdit,
}: {
  mon: MonitorWithLatest;
  latest: CheckResult | null;
  busy: boolean;
  onTest: () => void;
  onPause: () => void;
  onResume: () => void;
  onDelete: () => void;
  onEdit: () => void;
}) {
  const tone = mon.state === "up" ? "up" : mon.state === "down" ? "down" : "paused";
  return (
    <div>
      <p className="mb-2 text-xs text-ink-muted">
        <Link href="/monitors" className="hover:text-ink">Monitors</Link> / <span className="text-ink">{mon.name}</span>
      </p>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <span className="mt-2"><StatusDot tone={tone} pulse={mon.state !== "paused"} /></span>
          <div className="min-w-0">
            <div className="flex items-center gap-2.5">
              <h1 className="truncate text-xl font-semibold">{mon.name}</h1>
              <StateBadge state={mon.state} />
            </div>
            <a href={mon.url} target="_blank" rel="noreferrer noopener"
              className="mt-0.5 block truncate font-mono text-xs text-ink-muted hover:text-accent">
              {mon.url} ↗
            </a>
            <div className="mt-2 flex flex-wrap gap-1.5">
              <Badge tone="neutral">{mon.request_config?.method || "GET"}</Badge>
              <Badge tone="neutral">every {fmtInterval(mon.interval_seconds)}</Badge>
              <Badge tone="neutral">timeout {fmtMs(mon.timeout_ms)}</Badge>
              {latest ? <Badge tone="neutral">checked {fmtRelative(latest.checked_at)}</Badge> : null}
            </div>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <Button variant="secondary" size="sm" disabled={busy} onClick={onTest}>Run test now</Button>
          <Button variant="secondary" size="sm" disabled={busy} onClick={onEdit}>Edit</Button>
          {mon.state === "paused" ? (
            <Button size="sm" disabled={busy} onClick={onResume}>Resume</Button>
          ) : (
            <Button variant="secondary" size="sm" disabled={busy} onClick={onPause}>Pause</Button>
          )}
          <Button variant="danger" size="sm" disabled={busy} onClick={onDelete}>Delete</Button>
        </div>
      </div>
    </div>
  );
}

function TestResultCard({ result, onDismiss }: { result: CheckResult; onDismiss: () => void }) {
  const ok = result.status === "up";
  return (
    <Card className={`border-l-4 ${ok ? "border-l-up" : "border-l-down"}`}>
      <CardBody>
        <div className="flex items-start justify-between">
          <div>
            <p className="text-sm font-semibold">
              Manual test: <span className={ok ? "text-up" : "text-down"}>{result.status}</span> in {fmtMs(result.duration_ms)}
            </p>
            <TimingBar c={result} />
            <p className="mt-1 text-xs text-ink-muted">
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

/** Stacked waterfall of DNS / connect / TLS / TTFB phases. */
function TimingBar({ c }: { c: CheckResult }) {
  const parts = [
    { k: "DNS", v: c.dns_ms ?? 0, cls: "bg-sky-500" },
    { k: "Connect", v: c.connect_ms ?? 0, cls: "bg-violet-500" },
    { k: "TLS", v: c.tls_ms ?? 0, cls: "bg-amber-500" },
    { k: "TTFB", v: c.ttfb_ms ?? 0, cls: "bg-emerald-500" },
  ];
  const total = Math.max(c.duration_ms, parts.reduce((a, p) => a + p.v, 0), 1);
  return (
    <div className="mt-2 w-72 max-w-full">
      <div className="flex h-2 overflow-hidden rounded-full bg-surface-sunken">
        {parts.map((p) => (
          <span key={p.k} className={p.cls} style={{ width: `${(p.v / total) * 100}%` }} title={`${p.k} ${fmtMs(p.v)}`} />
        ))}
      </div>
      <div className="mt-1 flex gap-3 text-2xs text-ink-faint">
        {parts.map((p) => (
          <span key={p.k} className="inline-flex items-center gap-1"><span className={`h-1.5 w-1.5 rounded-full ${p.cls}`} />{p.k}</span>
        ))}
      </div>
    </div>
  );
}

function MonitorSections({
  orgID, projID, monitorID, history,
}: {
  orgID: string;
  projID: string;
  monitorID: string;
  history: ReturnType<typeof useApi<HistoryResp>>;
}) {
  const [win, setWin] = useState<WindowKey>("24h");
  const uptime = useApi<{ series: UptimeBucket[] | null; window: UptimeSummary }>(
    apiPath.uptime(orgID, projID, monitorID, `?window=${win}`),
    { pollMs: 60_000 },
  );
  const incidents = useApi<{ incidents: Incident[]; total: number }>(
    apiPath.incidents(orgID, projID, `?monitor_id=${monitorID}&limit=10`),
    { pollMs: 60_000 },
  );

  const summary = uptime.data?.window;
  const series = uptime.data?.series ?? [];
  const checks = history.data?.checks ?? [];
  const openIncidents = (incidents.data?.incidents ?? []).filter((i) => i.status === "open").length;

  const counts = useMemo(() => {
    const slow = slowThreshold(checks);
    const c = { up: 0, slow: 0, down: 0 };
    for (const ch of checks) c[classify(ch, slow) as "up" | "slow" | "down"]++;
    return c;
  }, [checks]);

  const durations = checks.filter((c) => c.status === "up").map((c) => c.duration_ms).sort((a, b) => a - b);
  const p95 = durations.length ? durations[Math.min(durations.length - 1, Math.floor(durations.length * 0.95))] : null;

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label={`Uptime (${win})`}
          value={summary?.no_data || summary?.uptime_pct == null ? "no data" : fmtUptime(summary.uptime_pct)}
          tone={summary?.uptime_pct == null || summary?.no_data ? "neutral" : summary.uptime_pct >= 99.9 ? "up" : summary.uptime_pct >= 95 ? "neutral" : "down"}
          hint={summary ? `${summary.checks_up}/${summary.checks_total} checks · downtime ${fmtDuration(summary.downtime_seconds)}` : undefined}
        />
        <StatCard label="Avg response" value={averageMs(series)} hint={p95 != null ? `p95 ${fmtMs(p95)} (recent)` : `window ${win}`} />
        <StatCard
          label="Last check"
          value={checks[0] ? <span className={checks[0].status === "up" ? "text-up" : "text-down"}>{fmtMs(checks[0].duration_ms)}</span> : "—"}
          hint={checks[0] ? `HTTP ${checks[0].http_status_code ?? "—"} · ${fmtRelative(checks[0].checked_at)}` : "waiting for first check"}
        />
        <StatCard label="Open incidents" value={openIncidents} tone={openIncidents > 0 ? "down" : "up"} hint={`${incidents.data?.total ?? 0} total`} />
      </div>

      <Card>
        <CardHeader
          title="Check activity"
          subtitle={checks.length ? `Every square is one check — last ${checks.length} shown` : "Every square is one check"}
          actions={
            <div className="hidden gap-3 text-2xs sm:flex">
              <span className="text-up">{counts.up} up</span>
              <span className="text-warn">{counts.slow} slow</span>
              <span className="text-down">{counts.down} down</span>
            </div>
          }
        />
        <CardBody className="space-y-3">
          {history.loading ? <Skeleton className="h-[105px] w-full" /> :
           history.error ? <p className="text-xs text-down">{history.error.message}</p> :
           <CheckGrid checks={checks} />}
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-2xs text-ink-faint">Older ← → Newer</span>
            <GridLegend />
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title="Response time"
          subtitle="Average per bucket; red dots mark buckets with failed checks"
          actions={
            <div className="flex rounded-md border border-border p-0.5">
              {WINDOWS.map((w) => (
                <button key={w.key} type="button" onClick={() => setWin(w.key)} aria-pressed={win === w.key}
                  className={`rounded px-2.5 py-1 text-2xs font-medium transition-colors ${
                    win === w.key ? "bg-accent text-accent-ink" : "text-ink-muted hover:text-ink"
                  }`}>
                  {w.label}
                </button>
              ))}
            </div>
          }
        />
        <CardBody className="space-y-4">
          {uptime.loading ? <Skeleton className="h-64 w-full" /> :
           uptime.error ? <p className="text-xs text-down">{uptime.error.message}</p> :
           <ResponseChart series={series} />}
          {series.length > 0 ? (
            <div>
              <p className="mb-1.5 text-2xs font-medium uppercase tracking-wide text-ink-faint">Availability ({win})</p>
              <AvailabilityBar series={series} />
            </div>
          ) : null}
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

function ResponseChart({ series }: { series: UptimeBucket[] }) {
  const data = series.map((b) => ({
    t: new Date(b.bucket_start).getTime(),
    ms: Math.round(b.avg_response_ms),
    failed: b.checks_total > 0 && b.checks_up < b.checks_total,
  }));

  if (data.length === 0) {
    return <div className="flex h-64 items-center justify-center text-xs text-ink-muted">No response-time data in this window yet.</div>;
  }

  return (
    <div className="h-64" role="img" aria-label="Response time chart">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id="rtFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="rgb(var(--accent))" stopOpacity={0.35} />
              <stop offset="100%" stopColor="rgb(var(--accent))" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" stroke="rgb(var(--border))" vertical={false} />
          <XAxis
            dataKey="t" type="number" scale="time" domain={["dataMin", "dataMax"]}
            tickFormatter={(t: number) => winLabel(new Date(t))}
            tick={{ fontSize: 10, fill: "rgb(var(--ink-faint))" }} stroke="rgb(var(--border))" minTickGap={40}
          />
          <YAxis
            tick={{ fontSize: 10, fill: "rgb(var(--ink-faint))" }} stroke="rgb(var(--border))" width={56}
            tickFormatter={(ms: number) => (ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`)}
          />
          <Tooltip
            labelFormatter={(t) => fmtDateTime(new Date(Number(t)).toISOString())}
            formatter={(value) => [fmtMs(Number(value)), "avg response"]}
            contentStyle={{
              background: "rgb(var(--surface-raised))", border: "1px solid rgb(var(--border))",
              borderRadius: 8, fontSize: 12, color: "rgb(var(--ink))",
            }}
          />
          <Area
            type="monotone" dataKey="ms" stroke="rgb(var(--accent))" strokeWidth={2} fill="url(#rtFill)"
            isAnimationActive={false}
            activeDot={{ r: 4 }}
            dot={(props: { cx?: number; cy?: number; index?: number; payload?: { failed: boolean } }) => {
              const { cx, cy, index, payload } = props;
              if (!payload?.failed || cx == null || cy == null) return <g key={`d${index}`} />;
              return <circle key={`d${index}`} cx={cx} cy={cy} r={4} fill="rgb(var(--down))" stroke="rgb(var(--surface-raised))" strokeWidth={1.5} />;
            }}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

function CheckHistory({ history }: { history: ReturnType<typeof useApi<HistoryResp>> }) {
  if (history.error) return <ErrorState message={history.error.message} onRetry={() => void history.refresh()} />;
  if (history.loading) return <TableSkeleton rows={6} />;
  const checks = (history.data?.checks ?? []).slice(0, 100);
  if (checks.length === 0) return <EmptyState title="No checks yet" message="Results appear here once the worker completes the first check." />;
  const slow = slowThreshold(history.data?.checks ?? []);
  const max = Math.max(...checks.map((c) => c.duration_ms), 1);
  return (
    <div className="max-h-96 overflow-auto">
      <table className="table-base">
        <thead className="sticky top-0 z-10 bg-surface-raised">
          <tr><th>Time</th><th>Status</th><th className="text-right">HTTP</th><th>Response</th><th className="text-right">Detail</th></tr>
        </thead>
        <tbody>
          {checks.map((c, i) => {
            const kind = classify(c, slow);
            return (
              <tr key={`${c.checked_at}-${i}`} className="row-link">
                <td className="whitespace-nowrap text-2xs text-ink-muted">{fmtDateTime(c.checked_at)}</td>
                <td><StateBadge state={c.status} /></td>
                <td className="text-right tabular-nums">{c.http_status_code ?? "—"}</td>
                <td className="min-w-[140px]">
                  <div className="flex items-center gap-2">
                    <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-surface-sunken">
                      <div className={`h-full rounded-full ${CELL_CLASS[kind].split(" ")[0]}`} style={{ width: `${Math.max(3, (c.duration_ms / max) * 100)}%` }} />
                    </div>
                    <span className="w-16 text-right text-xs tabular-nums">{fmtMs(c.duration_ms)}</span>
                  </div>
                </td>
                <td className="max-w-[220px] truncate text-right text-2xs text-ink-faint">
                  {c.error_class && c.error_class !== "none"
                    ? <span className="text-down">{`${ERROR_CLASS_LABEL[c.error_class] ?? c.error_class}${c.error_detail ? `: ${c.error_detail}` : ""}`}</span>
                    : fmtBytes(c.response_bytes)}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function IncidentsList({ incidents }: { incidents: ReturnType<typeof useApi<{ incidents: Incident[]; total: number }>> }) {
  if (incidents.error) return <ErrorState message={incidents.error.message} onRetry={() => void incidents.refresh()} />;
  if (incidents.loading) return <TableSkeleton rows={3} />;
  const list = incidents.data?.incidents ?? [];
  if (list.length === 0) {
    return (
      <div className="flex flex-col items-center px-5 py-12 text-center">
        <span className="mb-3 flex h-10 w-10 items-center justify-center rounded-full bg-up/10 text-up">✓</span>
        <p className="text-sm font-medium">No incidents</p>
        <p className="mt-1 text-xs text-ink-muted">No outages recorded for this monitor.</p>
      </div>
    );
  }
  return (
    <ul className="divide-y divide-border/60">
      {list.map((i) => (
        <li key={i.id} className="flex items-start gap-3 px-5 py-3">
          <span className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${i.status === "open" ? "bg-down" : "bg-up"}`} />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium">{fmtDateTime(i.started_at)}</p>
            <p className="text-2xs text-ink-faint">
              {i.status === "open" ? `Ongoing · started ${fmtRelative(i.started_at)}` : `Resolved in ${fmtDuration(i.resolved_duration_s)}`}
              {i.cause_class ? ` · ${ERROR_CLASS_LABEL[i.cause_class] ?? i.cause_class}` : ""}
            </p>
          </div>
          {i.status === "open" ? <Badge tone="down">open</Badge> : <Badge tone="neutral">resolved</Badge>}
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

function winLabel(d: Date): string {
  const now = Date.now() - d.getTime();
  if (now < 26 * 3600_000) return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function averageMs(series: UptimeBucket[]): string {
  const pts = series.filter((b) => b.checks_total > 0);
  if (pts.length === 0) return "—";
  const total = pts.reduce((acc, b) => acc + b.checks_total, 0);
  const avg = pts.reduce((acc, b) => acc + b.avg_response_ms * b.checks_total, 0) / total;
  return fmtMs(Math.round(avg));
}
