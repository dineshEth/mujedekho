"use client";

import { useState, type FormEvent } from "react";
import { Card, CardHeader, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label, Input, Select } from "@/components/ui/input";
import { ApiError } from "@/lib/api";
import type { Monitor } from "@/lib/types";

export type MonitorPayload = Record<string, unknown>;

const INTERVAL_PRESETS = [
  { s: 30, label: "30s" },
  { s: 60, label: "1m" },
  { s: 300, label: "5m" },
  { s: 900, label: "15m" },
  { s: 3600, label: "1h" },
];

/**
 * Shared create/edit form. In edit mode it is seeded from the monitor and the
 * submit callback issues a PUT; in create mode it POSTs and arms the monitor.
 */
export function MonitorForm({
  initial,
  submitLabel,
  submittingLabel,
  onSubmit,
  onCancel,
  compact,
}: {
  initial?: Monitor;
  submitLabel: string;
  submittingLabel: string;
  onSubmit: (payload: MonitorPayload) => Promise<void>;
  onCancel?: () => void;
  compact?: boolean;
}) {
  const rc = initial?.request_config;
  const [name, setName] = useState(initial?.name ?? "");
  const [url, setUrl] = useState(initial?.url ?? "https://");
  const [intervalSeconds, setIntervalSeconds] = useState(initial?.interval_seconds ?? 60);
  const [timeoutMs, setTimeoutMs] = useState(initial?.timeout_ms ?? 10000);
  const [failureThreshold, setFailureThreshold] = useState(initial?.failure_threshold ?? 3);
  const [method, setMethod] = useState(rc?.method || "GET");
  const [followRedirects, setFollowRedirects] = useState(rc?.follow_redirects !== false);
  const [expectedMin, setExpectedMin] = useState(rc?.expected_status_min || 200);
  const [expectedMax, setExpectedMax] = useState(rc?.expected_status_max || 299);
  const [headers, setHeaders] = useState(
    Object.entries(rc?.headers ?? {}).map(([k, v]) => `${k}: ${v}`).join("\n"),
  );
  const [body, setBody] = useState(rc?.body ?? "");
  const [advanced, setAdvanced] = useState(!!initial);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handle(e: FormEvent) {
    e.preventDefault();
    const parsedHeaders = parseHeaders(headers);
    if (typeof parsedHeaders === "string") {
      setError(parsedHeaders);
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await onSubmit({
        name: name.trim(),
        url: url.trim(),
        monitor_type: initial?.monitor_type ?? "http",
        interval_seconds: intervalSeconds,
        timeout_ms: timeoutMs,
        failure_threshold: failureThreshold,
        request_config: {
          method,
          follow_redirects: followRedirects,
          expected_status_min: clamp(expectedMin, 100, 599),
          expected_status_max: clamp(expectedMax, 100, 599),
          ...(Object.keys(parsedHeaders).length ? { headers: parsedHeaders } : {}),
          ...(method === "POST" && body ? { body } : {}),
        },
      });
    } catch (err) {
      if (err instanceof ApiError && err.details && Object.keys(err.details).length > 0) {
        setError(Object.entries(err.details).map(([k, v]) => `${k.replace(/_/g, " ")}: ${String(v)}`).join(" · "));
      } else {
        setError(err instanceof Error ? err.message : "Request failed");
      }
    } finally {
      setSubmitting(false);
    }
  }

  const invalid = !name.trim() || url.trim().length <= "https://".length;
  const Wrap = compact ? Plain : Card;

  return (
    <form onSubmit={handle} noValidate className="space-y-4">
      <Wrap>
        {compact ? null : <CardHeader title="Basic" subtitle="What to check and how often" />}
        <CardBody className={`space-y-4 ${compact ? "!px-0 !py-0" : ""}`}>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <Label htmlFor="m-name">Name</Label>
              <Input id="m-name" required maxLength={100} placeholder="Marketing site" value={name}
                onChange={(e) => setName(e.target.value)} />
            </div>
            <div>
              <Label htmlFor="m-url" hint="public http(s) only">URL</Label>
              <Input id="m-url" required type="url" placeholder="https://example.com/" value={url}
                onChange={(e) => setUrl(e.target.value)} className="font-mono text-xs" />
            </div>
          </div>

          <div>
            <Label htmlFor="m-interval" hint="seconds (5–86400)">Check interval</Label>
            <div className="flex flex-wrap items-center gap-2">
              <div className="flex rounded-md border border-border p-0.5">
                {INTERVAL_PRESETS.map((p) => (
                  <button key={p.s} type="button" onClick={() => setIntervalSeconds(p.s)} aria-pressed={intervalSeconds === p.s}
                    className={`rounded px-2.5 py-1 text-2xs font-medium transition-colors ${
                      intervalSeconds === p.s ? "bg-accent text-accent-ink" : "text-ink-muted hover:text-ink"
                    }`}>
                    {p.label}
                  </button>
                ))}
              </div>
              <Input id="m-interval" type="number" min={5} max={86400} value={intervalSeconds}
                onChange={(e) => setIntervalSeconds(Number(e.target.value))} className="!w-28" />
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <Label htmlFor="m-timeout" hint="ms (1000–30000)">Timeout</Label>
              <Input id="m-timeout" type="number" min={1000} max={30000} step={500} value={timeoutMs}
                onChange={(e) => setTimeoutMs(Number(e.target.value))} />
            </div>
            <div>
              <Label htmlFor="m-threshold" hint="failures before incident (1–20)">Confirm failures</Label>
              <Input id="m-threshold" type="number" min={1} max={20} value={failureThreshold}
                onChange={(e) => setFailureThreshold(Number(e.target.value))} />
            </div>
          </div>
        </CardBody>
      </Wrap>

      <button type="button" onClick={() => setAdvanced((v) => !v)} aria-expanded={advanced}
        className="text-xs font-medium text-accent hover:underline">
        {advanced ? "Hide" : "Show"} request options
      </button>

      {advanced ? (
        <Wrap>
          {compact ? null : <CardHeader title="Request options" subtitle="Method, headers and what counts as success" />}
          <CardBody className={`space-y-4 ${compact ? "!px-0 !py-0" : ""}`}>
            <div className="grid gap-4 sm:grid-cols-4">
              <div>
                <Label htmlFor="m-method">Method</Label>
                <Select id="m-method" className="w-full" value={method} onValueChange={setMethod}
                  options={["GET", "HEAD", "POST"].map((m) => ({ value: m, label: m }))} />
              </div>
              <div>
                <Label htmlFor="m-emin">Status min</Label>
                <Input id="m-emin" type="number" min={100} max={599} value={expectedMin}
                  onChange={(e) => setExpectedMin(Number(e.target.value))} />
              </div>
              <div>
                <Label htmlFor="m-emax">Status max</Label>
                <Input id="m-emax" type="number" min={100} max={599} value={expectedMax}
                  onChange={(e) => setExpectedMax(Number(e.target.value))} />
              </div>
              <label className="flex items-center gap-2 pt-6 text-xs">
                <input type="checkbox" checked={followRedirects} onChange={(e) => setFollowRedirects(e.target.checked)}
                  className="h-4 w-4 rounded border-border text-accent focus:ring-accent" />
                Follow redirects
              </label>
            </div>
            <div>
              <Label htmlFor="m-headers" hint="one per line, Name: value">Request headers</Label>
              <textarea id="m-headers" rows={3} value={headers} onChange={(e) => setHeaders(e.target.value)}
                placeholder={"Accept: application/json\nX-Health-Token: …"}
                className="block w-full rounded-md border border-border bg-surface px-3 py-2 font-mono text-xs text-ink placeholder:text-ink-faint focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/20" />
            </div>
            {method === "POST" ? (
              <div>
                <Label htmlFor="m-body">Request body</Label>
                <textarea id="m-body" rows={3} value={body} onChange={(e) => setBody(e.target.value)}
                  className="block w-full rounded-md border border-border bg-surface px-3 py-2 font-mono text-xs text-ink focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/20" />
              </div>
            ) : null}
          </CardBody>
        </Wrap>
      ) : null}

      {error ? <p role="alert" className="rounded-md bg-down/10 px-3 py-2 text-xs text-down">{error}</p> : null}

      <div className="flex items-center gap-2">
        <Button type="submit" disabled={submitting || invalid}>{submitting ? submittingLabel : submitLabel}</Button>
        {onCancel ? <Button variant="ghost" onClick={onCancel} disabled={submitting}>Cancel</Button> : null}
      </div>
    </form>
  );
}

function Plain({ children }: { children: React.ReactNode }) {
  return <div className="space-y-4">{children}</div>;
}

function parseHeaders(raw: string): Record<string, string> | string {
  const out: Record<string, string> = {};
  for (const [i, line] of raw.split("\n").entries()) {
    const t = line.trim();
    if (!t) continue;
    const idx = t.indexOf(":");
    if (idx <= 0) return `Header line ${i + 1} must look like "Name: value".`;
    out[t.slice(0, idx).trim()] = t.slice(idx + 1).trim();
  }
  return out;
}

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n));
}
