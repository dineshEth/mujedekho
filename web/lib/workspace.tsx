"use client";

import React, { createContext, useCallback, useContext, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { apiPath } from "@/lib/api";
import type { Organization, Project, User } from "@/lib/types";

interface MeResponse {
  user: User;
  organizations: Organization[];
}

interface WorkspaceValue {
  user: User | null;
  orgs: Organization[];
  org: Organization | null;
  project: Project | null;
  projects: Project[];
  setOrg: (id: string) => void;
  setProject: (id: string) => void;
  reload: () => Promise<void>;
  loading: boolean;
  error: string | null;
}

const Ctx = createContext<WorkspaceValue | null>(null);

const LS_ORG = "s24x7.org";
const LS_PROJ = (org: string) => `s24x7.project.${org}`;

export function WorkspaceProvider({ children }: { children: React.ReactNode }) {
  const [me, setMe] = useState<MeResponse | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [orgId, setOrgIdState] = useState<string | null>(null);
  const [projectId, setProjectIdState] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadMe = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const raw = await api<MeResponse>(apiPath.me);
      const data: MeResponse = { ...raw, organizations: raw.organizations ?? [] };
      setMe(data);
      const stored = window.localStorage.getItem(LS_ORG);
      const valid = data.organizations.find((o) => o.id === stored) ?? data.organizations[0] ?? null;
      setOrgIdState(valid?.id ?? null);
      if (valid) {
        const list = await fetchProjects(valid.id);
        const sp = window.localStorage.getItem(LS_PROJ(valid.id));
        const p = list.find((x) => x.id === sp) ?? list[0] ?? null;
        setProjects(list);
        setProjectIdState(p?.id ?? null);
      } else {
        setProjects([]);
        setProjectIdState(null);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load workspace");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadMe();
  }, [loadMe]);

  const setOrg = useCallback(
    (id: string) => {
      setOrgIdState(id);
      window.localStorage.setItem(LS_ORG, id);
      setProjects([]);
      setProjectIdState(null);
      void (async () => {
        const list = await fetchProjects(id).catch(() => []);
        setProjects(list);
        const sp = window.localStorage.getItem(LS_PROJ(id));
        const p = list.find((x) => x.id === sp) ?? list[0] ?? null;
        setProjectIdState(p?.id ?? null);
      })();
    },
    [],
  );

  const setProject = useCallback((id: string) => {
    setProjectIdState(id);
    if (orgId) window.localStorage.setItem(LS_PROJ(orgId), id);
  }, [orgId]);

  // Sync the active project's record from the loaded list (freshness).
  const org = me?.organizations.find((o) => o.id === orgId) ?? null;
  const project = projects.find((p) => p.id === projectId) ?? null;

  const value: WorkspaceValue = {
    user: me?.user ?? null,
    orgs: me?.organizations ?? [],
    org,
    project,
    projects,
    setOrg,
    setProject,
    reload: loadMe,
    loading,
    error,
  };

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

async function fetchProjects(orgID: string): Promise<Project[]> {
  const res = await api<{ projects: Project[] }>(apiPath.projects(orgID));
  return res.projects ?? [];
}

export function useWorkspace(): WorkspaceValue {
  const v = useContext(Ctx);
  if (!v) throw new Error("useWorkspace must be used inside WorkspaceProvider");
  return v;
}
