"use client";

import { useState, type FormEvent } from "react";
import { Card, CardHeader, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label, Input } from "@/components/ui/input";
import { api, apiPath } from "@/lib/api";
import { useWorkspace } from "@/lib/workspace";
import type { Organization, Project } from "@/lib/types";

/** First-run setup: a new account has no organization (the API creates the
 * user only), and an organization may have no project yet. Without both, no
 * monitor can be created, so the dashboard renders this instead. */
export function Onboarding() {
  const ws = useWorkspace();
  const needsOrg = !ws.org;

  const [orgName, setOrgName] = useState("");
  const [projectName, setProjectName] = useState("Production");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      let orgID = ws.org?.id;
      if (needsOrg) {
        const org = await api<Organization>(apiPath.organizations, {
          method: "POST",
          body: JSON.stringify({ name: orgName.trim(), slug: slugify(orgName) }),
        });
        orgID = org.id;
        window.localStorage.setItem("s24x7.org", org.id);
      }
      await api<Project>(apiPath.projects(orgID!), {
        method: "POST",
        body: JSON.stringify({ name: projectName.trim() }),
      });
      await ws.reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Setup failed");
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-lg">
      <Card>
        <CardHeader
          title={needsOrg ? "Set up your workspace" : "Create a project"}
          subtitle={
            needsOrg
              ? "Monitors live in a project inside an organization. Create both to get started."
              : `${ws.org?.name} has no projects yet. Create one to start adding monitors.`
          }
        />
        <CardBody>
          <form onSubmit={onSubmit} className="space-y-4" noValidate>
            {needsOrg ? (
              <div>
                <Label htmlFor="ob-org" hint="your company or team">Organization name</Label>
                <Input id="ob-org" required maxLength={100} placeholder="Acme Inc." value={orgName}
                  onChange={(e) => setOrgName(e.target.value)} />
              </div>
            ) : null}
            <div>
              <Label htmlFor="ob-proj" hint="e.g. Production, Staging">Project name</Label>
              <Input id="ob-proj" required maxLength={100} value={projectName}
                onChange={(e) => setProjectName(e.target.value)} />
            </div>
            {error ? (
              <p role="alert" className="rounded-md bg-down/10 px-3 py-2 text-xs text-down">{error}</p>
            ) : null}
            <Button
              type="submit"
              disabled={submitting || !projectName.trim() || (needsOrg && slugify(orgName).length < 3)}
            >
              {submitting ? "Creating…" : "Continue"}
            </Button>
          </form>
        </CardBody>
      </Card>
    </div>
  );
}

/** Slug rules mirror the API: ^[a-z0-9][a-z0-9-]{2,49}$. A short random
 * suffix keeps slugs unique across tenants with the same display name. */
function slugify(name: string): string {
  const base = name
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40);
  if (!base) return "";
  return `${base}-${Math.random().toString(36).slice(2, 7)}`;
}
