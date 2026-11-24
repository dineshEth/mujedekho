import type { ApiErrorBody } from "./types";

/** Client-side fetch to the Go API via the BFF proxy. Throws ApiError on
 * non-2xx so callers can render error states without leaking internals. */
export class ApiError extends Error {
  status: number;
  code?: string;
  details?: Record<string, unknown>;

  constructor(status: number, code: string | undefined, message: string, details?: Record<string, unknown>) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

export async function api<T>(path: string, init?: RequestInit & { query?: Record<string, string> }): Promise<T> {
  const url = new URL(`/api/proxy/${path.replace(/^\/+/, "")}`, window.location.origin);
  if (init?.query) {
    for (const [k, v] of Object.entries(init.query)) url.searchParams.set(k, v);
  }

  const res = await fetch(url.toString(), {
    method: init?.method ?? "GET",
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    body: init?.body ?? undefined,
    cache: "no-store",
  });

  if (!res.ok) {
    let body: ApiErrorBody = {};
    try {
      body = await res.json();
    } catch {
      // Non-JSON (proxy 401, html): keep empty envelope.
    }
    throw new ApiError(res.status, body.error?.code, body.error?.message ?? `Request failed (${res.status})`, body.error?.details);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const apiPath = {
  me: "auth/me",
  projects: (orgID: string) => `organizations/${orgID}/projects`,
  monitors: (orgID: string, projID: string) => `organizations/${orgID}/projects/${projID}/monitors`,
  monitor: (orgID: string, projID: string, id: string) => `${apiPath.monitors(orgID, projID)}/${id}`,
  history: (orgID: string, projID: string, id: string, q: string) =>
    `${apiPath.monitor(orgID, projID, id)}/history${q}`,
  uptime: (orgID: string, projID: string, id: string, q: string) =>
    `${apiPath.monitor(orgID, projID, id)}/uptime${q}`,
  monitorTest: (orgID: string, projID: string, id: string) => `${apiPath.monitor(orgID, projID, id)}/test`,
  monitorAction: (orgID: string, projID: string, id: string, action: "pause" | "resume") =>
    `${apiPath.monitor(orgID, projID, id)}/${action}`,
  incidents: (orgID: string, projID: string, q = "") => `organizations/${orgID}/projects/${projID}/incidents${q}`,
  incident: (orgID: string, projID: string, id: string) =>
    `organizations/${orgID}/projects/${projID}/incidents/${id}`,
};
