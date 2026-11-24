"use client";

import { Card, CardHeader, CardBody, StatCard } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/feedback";
import { fmtDateTime } from "@/lib/format";
import { useWorkspace } from "@/lib/workspace";

export default function AccountSettingsPage() {
  const ws = useWorkspace();
  const user = ws.user;

  return (
    <div className="mx-auto max-w-3xl">
      <div className="mb-6">
        <h1 className="text-lg font-semibold">Account settings</h1>
        <p className="text-xs text-ink-muted">Your profile and session.</p>
      </div>

      {!user ? (
        <Skeleton className="h-40 w-full" />
      ) : (
        <>
          <div className="mb-6 grid gap-4 sm:grid-cols-2">
            <StatCard label="Organizations" value={ws.orgs.length} hint="you are a member of" />
            <StatCard label="Projects" value={ws.projects.length} hint={`in ${ws.org?.name ?? "current org"}`} />
          </div>

          <Card>
            <CardHeader title="Profile" />
            <CardBody className="space-y-4 text-sm">
              <Row label="Full name" value={user.full_name || "—"} />
              <Row label="Email" value={user.email} mono />
              <Row label="User ID" value={user.id} mono />
              <Row label="Member since" value={fmtDateTime(user.created_at)} />
              <Row label="Session" value="JWT access token in an httpOnly cookie (15-minute TTL by default)" />
            </CardBody>
          </Card>

          <Card className="mt-4">
            <CardHeader
              title="Email & password changes"
              subtitle="Will open up with the account API expansion (verification flow included)"
            />
            <CardBody className="text-xs text-ink-muted">
              Password changes and email updates require re-authentication and verification; they land with the
              account-management endpoints rather than as client-side tricks.
            </CardBody>
          </Card>
        </>
      )}
    </div>
  );
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border/40 pb-2">
      <span className="text-xs text-ink-muted">{label}</span>
      <span className={`text-xs font-medium ${mono ? "font-mono" : ""}`}>{value}</span>
    </div>
  );
}
