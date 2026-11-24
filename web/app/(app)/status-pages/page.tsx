"use client";

import { Card, CardHeader, CardBody } from "@/components/ui/card";

/** Public status pages ship in Phase 9 with the caching layer (server-rendered
 * HTML by the Go API at /status/{slug}). Honest placeholder. */
export default function StatusPagesPage() {
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-lg font-semibold">Status pages</h1>
      <p className="mb-6 text-xs text-ink-muted">
        Publish a public page with selected monitors, uptime and incident history.
      </p>

      <Card>
        <CardHeader
          title="Status pages — shipping in Phase 9"
          subtitle="Delivered together with the deployment stack for cacheability and session isolation."
        />
        <CardBody className="space-y-4 text-sm text-ink-muted">
          <p>The final phase adds:</p>
          <ul className="list-inside list-disc space-y-1.5 text-xs">
            <li><span className="font-medium text-ink">Public pages</span> at <code className="font-mono text-2xs">/status/&#123;slug&#125;</code> — no auth required, no session cookies.</li>
            <li><span className="font-medium text-ink">Monitor selection</span> and reorderable sections per page.</li>
            <li><span className="font-medium text-ink">Uptime banners</span> (90-day bar) and incident history feeds.</li>
            <li><span className="font-medium text-ink">CNAME support</span> for custom domains and CDN caching.</li>
          </ul>
        </CardBody>
      </Card>
    </div>
  );
}
