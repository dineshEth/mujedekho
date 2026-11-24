"use client";

import Link from "next/link";
import { Card, CardHeader, CardBody } from "@/components/ui/card";
import { useWorkspace } from "@/lib/workspace";

export default function ProjectSettingsPage() {
  const ws = useWorkspace();

  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-6">
        <h1 className="text-lg font-semibold">Project settings</h1>
        <p className="text-xs text-ink-muted">
          <Link href="/settings" className="text-accent hover:underline">Settings</Link> / Project
        </p>
      </div>

      <Card>
        <CardHeader title="Project" subtitle="Server-rendered member management lands with the RBAC API expansion" />
        <CardBody className="space-y-4 text-sm">
          <ConfigRow label="Name" value={ws.project?.name ?? "—"} />
          <ConfigRow label="Project ID" value={ws.project?.id ?? "—"} mono />
          <ConfigRow label="Organization" value={ws.org?.name ?? "—"} />
          <ConfigRow label="Organization slug" value={ws.org?.slug ?? "—"} mono />
          <ConfigRow label="Your role" value="viewer / editor / admin — shown by the API in the members expansion" />
        </CardBody>
      </Card>

      <Card className="mt-4">
        <CardHeader title="Monitor defaults" subtitle="Applied to new monitors created in this project" />
        <CardBody className="grid gap-x-8 gap-y-3 text-sm sm:grid-cols-2">
          <ConfigRow label="Default interval" value="60 seconds" />
          <ConfigRow label="Default timeout" value="10,000 ms" />
          <ConfigRow label="Failure threshold" value="3 checks" />
          <ConfigRow label="Allowed URL schemes" value="http, https (public targets only)" />
        </CardBody>
      </Card>
    </div>
  );
}

function ConfigRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border/40 pb-2">
      <span className="text-xs text-ink-muted">{label}</span>
      <span className={`text-xs font-medium ${mono ? "font-mono" : ""}`}>{value}</span>
    </div>
  );
}
