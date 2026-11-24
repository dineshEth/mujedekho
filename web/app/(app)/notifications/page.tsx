"use client";

import { Card, CardHeader, CardBody } from "@/components/ui/card";

/** Notifications arrive in Phase 6 (email + webhook dispatcher). This page is
 * honest scaffolding: it documents status instead of mocking channels. */
export default function NotificationsPage() {
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-lg font-semibold">Notification settings</h1>
      <p className="mb-6 text-xs text-ink-muted">
        Alert channels, policies and delivery history for incidents.
      </p>

      <Card>
        <CardHeader
          title="Notification engine — shipping in Phase 6"
          subtitle="The incident engine and its notification hook are already wired; delivery channels land next."
        />
        <CardBody className="space-y-4 text-sm text-ink-muted">
          <p>
            When Phase 6 lands, this page will let you configure:
          </p>
          <ul className="list-inside list-disc space-y-1.5 text-xs">
            <li><span className="font-medium text-ink">Email</span> — SMTP delivery per channel, verified address.</li>
            <li><span className="font-medium text-ink">Webhooks</span> — signed payloads (HMAC) with retry and backoff; SSRF-validated targets.</li>
            <li><span className="font-medium text-ink">Alert policies</span> — which incidents trigger which channels, recovery alerts, deduplication.</li>
            <li><span className="font-medium text-ink">Delivery history</span> — per-recipient outcomes with timestamps.</li>
          </ul>
          <p className="text-2xs text-ink-faint">
            See the <a href="https://github.com/your-org/site24x7" className="text-accent hover:underline">implementation tracker</a> (docs/TRACKER.md) for phase status.
          </p>
        </CardBody>
      </Card>
    </div>
  );
}
