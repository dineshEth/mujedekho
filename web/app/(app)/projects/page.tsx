"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { Card, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EmptyState, Skeleton } from "@/components/ui/feedback";
import { NewProjectDialog } from "@/components/projects/new-project-dialog";
import { apiPath } from "@/lib/api";
import { useApi } from "@/lib/use-api";
import { useWorkspace } from "@/lib/workspace";
import type { Monitor, Project } from "@/lib/types";

export default function ProjectsPage() {
  const ws = useWorkspace();
  const [creating, setCreating] = useState(false);

  if (ws.loading) return null;
  if (!ws.org) return <EmptyState title="No organization" message="Create an organization first." />;

  return (
    <div>
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-lg font-semibold">Projects</h1>
          <p className="text-xs text-ink-muted">
            {ws.projects.length} project{ws.projects.length === 1 ? "" : "s"} in <span className="font-medium text-ink">{ws.org.name}</span>.
            Use projects to separate environments, teams or customers.
          </p>
        </div>
        <Button size="sm" onClick={() => setCreating(true)}>+ New project</Button>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {ws.projects.map((p) => <ProjectCard key={p.id} p={p} active={p.id === ws.project?.id} />)}
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="flex min-h-[168px] flex-col items-center justify-center rounded-lg border-2 border-dashed border-border text-ink-muted transition-colors hover:border-accent hover:text-accent"
        >
          <span className="text-2xl leading-none">+</span>
          <span className="mt-2 text-sm font-medium">Add project</span>
        </button>
      </div>

      <NewProjectDialog open={creating} onClose={() => setCreating(false)} />
    </div>
  );
}

function ProjectCard({ p, active }: { p: Project; active: boolean }) {
  const ws = useWorkspace();
  const router = useRouter();
  const monitors = useApi<{ monitors: Monitor[]; total: number }>(
    ws.org ? `${apiPath.monitors(ws.org.id, p.id)}?limit=200` : null,
    { pollMs: 60_000 },
  );
  const list = monitors.data?.monitors ?? [];
  const up = list.filter((m) => m.state === "up").length;
  const down = list.filter((m) => m.state === "down").length;
  const paused = list.filter((m) => m.state === "paused").length;
  const total = list.length || 1;

  function open(path: string) {
    ws.setProject(p.id);
    router.push(path);
  }

  return (
    <Card className={active ? "ring-2 ring-accent/60" : ""}>
      <CardBody className="flex h-full flex-col">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold">{p.name}</p>
            <p className="truncate font-mono text-2xs text-ink-faint">{p.id}</p>
          </div>
          {active ? <Badge tone="up">current</Badge> : down > 0 ? <Badge tone="down">{down} down</Badge> : null}
        </div>

        {monitors.loading ? (
          <Skeleton className="mt-4 h-10" />
        ) : (
          <>
            <div className="mt-4 flex items-baseline gap-4 text-xs">
              <span><span className="text-xl font-semibold tabular-nums">{list.length}</span> <span className="text-ink-muted">monitors</span></span>
              <span className="text-up">{up} up</span>
              <span className={down ? "text-down" : "text-ink-faint"}>{down} down</span>
              <span className="text-ink-faint">{paused} paused</span>
            </div>
            <div className="mt-2 flex h-1.5 overflow-hidden rounded-full bg-surface-sunken">
              {list.length ? (
                <>
                  <span className="bg-up" style={{ width: `${(up / total) * 100}%` }} />
                  <span className="bg-down" style={{ width: `${(down / total) * 100}%` }} />
                  <span className="bg-paused" style={{ width: `${(paused / total) * 100}%` }} />
                </>
              ) : null}
            </div>
          </>
        )}

        <div className="mt-auto flex gap-2 pt-4">
          <Button size="sm" variant={active ? "secondary" : "primary"} onClick={() => open("/monitors")}>
            {active ? "View monitors" : "Switch to project"}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => open("/monitors/new")}>Add monitor</Button>
        </div>
      </CardBody>
    </Card>
  );
}
