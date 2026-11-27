"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Card, CardHeader, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label, Input, Select } from "@/components/ui/input";
import { ErrorState } from "@/components/ui/feedback";
import { api, apiPath, ApiError } from "@/lib/api";
import { useWorkspace } from "@/lib/workspace";
import type { Monitor } from "@/lib/types";

export default function NewMonitorPage() {
  const ws = useWorkspace();
  const router = useRouter();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  async function createAndArm(payload: Record<string, unknown>) {
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
    return monitor;
  }

  if (!ws.org || !ws.project) {
    return <ErrorState message="Create or select an organization and project before adding a monitor." />;
  }

  return (
    <div className="mx-auto max-w-2xl">
      <h1 className="mb-1 text-lg font-semibold">Add monitor</h1>
      <p className="mb-6 text-xs text-ink-muted">
        Checks begin as soon as the monitor is created. Private and loopback URLs are rejected by SSRF guards.
      </p>
      <MonitorForm
        key={ws.project.id}
        submitting={submitting}
        error={error}
        onFormError={setError}
        submittingSetter={setSubmitting}
        submit={createAndArm}
        onSuccess={(m) => router.push(`/monitors/${m.id}`)}
      />
    </div>
  );
}

interface MonitorFormProps {
  submitting: boolean;
  error: Error | null;
  submit: (payload: Record<string, unknown>) => Promise<Monitor>;
  onSuccess: (m: Monitor) => void;
  onFormError: (e: Error | null) => void;
  submittingSetter: (v: boolean) => void;
}

function MonitorForm({ submitting, error, submit, onSuccess, onFormError, submittingSetter }: MonitorFormProps) {
  const [name, setName] = useState("");
  const [url, setUrl] = useState("https://");
  const [intervalSeconds, setIntervalSeconds] = useState(60);
  const [timeoutMs, setTimeoutMs] = useState(10000);
  const [failureThreshold, setFailureThreshold] = useState(3);
  const [method, setMethod] = useState("GET");
  const [followRedirects, setFollowRedirects] = useState(true);
  const [expectedMin, setExpectedMin] = useState(200);
  const [expectedMax, setExpectedMax] = useState(299);
  const [advanced, setAdvanced] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    submittingSetter(true);
    onFormError(null);
    try {
      const monitor = await submit({
        name,
        url: url.trim(),
        monitor_type: "http",
        interval_seconds: intervalSeconds,
        timeout_ms: timeoutMs,
        failure_threshold: failureThreshold,
        request_config: {
          method,
          follow_redirects: followRedirects,
          expected_status_min: clamp(expectedMin, 100, 599),
          expected_status_max: clamp(expectedMax, 100, 599),
        },
      });
      onSuccess(monitor);
    } catch (err) {
      if (err instanceof ApiError && err.details && Object.keys(err.details).length > 0) {
        const fields = Object.entries(err.details).map(([k, v]) => `${k.replace(/_/g, " ")}: ${String(v)}`);
        onFormError(new Error(fields.join(" · ")));
      } else {
        onFormError(err instanceof Error ? err : new Error("Failed to create monitor"));
      }
      submittingSetter(false);
    }
  }

  const field = "w-full";

  return (
    <form onSubmit={onSubmit} noValidate>
      <Card>
        <CardHeader title="Basic" subtitle="What to check and how often" />
        <CardBody className="space-y-4">
          <div>
            <Label htmlFor="m-name">Name</Label>
            <Input id="m-name" required maxLength={80} placeholder="Marketing site" value={name}
              onChange={(e) => setName(e.target.value)} className={field} />
          </div>
          <div>
            <Label htmlFor="m-url" hint="public http(s) only">URL</Label>
            <Input id="m-url" required type="url" placeholder="https://example.com/" value={url}
              onChange={(e) => setUrl(e.target.value)} className={field} />
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <div>
              <Label htmlFor="m-interval" hint="seconds (5–86400)">Interval</Label>
              <Input id="m-interval" type="number" min={5} max={86400} value={intervalSeconds}
                onChange={(e) => setIntervalSeconds(Number(e.target.value))} className={field} />
            </div>
            <div>
              <Label htmlFor="m-timeout" hint="ms (1000–30000)">Timeout</Label>
              <Input id="m-timeout" type="number" min={1000} max={30000} value={timeoutMs}
                onChange={(e) => setTimeoutMs(Number(e.target.value))} className={field} />
            </div>
            <div>
              <Label htmlFor="m-threshold" hint="failures before incident (1–20)">Confirm failures</Label>
              <Input id="m-threshold" type="number" min={1} max={20} value={failureThreshold}
                onChange={(e) => setFailureThreshold(Number(e.target.value))} className={field} />
            </div>
          </div>
        </CardBody>
      </Card>

      <div className="mt-4">
        <button
          type="button"
          onClick={() => setAdvanced((v) => !v)}
          aria-expanded={advanced}
          className="text-xs font-medium text-accent hover:underline"
        >
          {advanced ? "Hide" : "Show"} advanced request options
        </button>
      </div>

      {advanced ? (
        <Card className="mt-3">
          <CardHeader title="Request options" />
          <CardBody className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <Label htmlFor="m-method">Method</Label>
                <Select id="m-method" className={`w-full ${field}`} value={method} onValueChange={setMethod}
                  options={["GET", "HEAD", "POST"].map((m) => ({ value: m, label: m }))} />
              </div>
              <label className="flex items-center gap-2 pt-6 text-sm">
                <input type="checkbox" checked={followRedirects}
                  onChange={(e) => setFollowRedirects(e.target.checked)}
                  className="h-4 w-4 rounded border-border text-accent focus:ring-accent" />
                Follow redirects
              </label>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <Label htmlFor="m-emin">Expected status min</Label>
                <Input id="m-emin" type="number" min={100} max={599} value={expectedMin}
                  onChange={(e) => setExpectedMin(Number(e.target.value))} className={field} />
              </div>
              <div>
                <Label htmlFor="m-emax">Expected status max</Label>
                <Input id="m-emax" type="number" min={100} max={599} value={expectedMax}
                  onChange={(e) => setExpectedMax(Number(e.target.value))} className={field} />
              </div>
            </div>
          </CardBody>
        </Card>
      ) : null}

      {error ? (
        <p role="alert" className="mt-4 rounded-md bg-down/10 px-3 py-2 text-xs text-down">
          {error.message}
        </p>
      ) : null}

      <div className="mt-6 flex items-center gap-3">
        <Button type="submit" disabled={submitting || !name || url.trim().length <= "https://".length}>
          {submitting ? "Creating…" : "Create and start monitoring"}
        </Button>
        <span className="text-2xs text-ink-faint">
          The target is validated at creation and on every check.
        </span>
      </div>
    </form>
  );
}

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n));
}
