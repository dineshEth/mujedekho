/** Formatting helpers shared by views. */

export function fmtDateTime(iso: string | undefined | null): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return d.toLocaleString(undefined, {
    year: "numeric", month: "short", day: "2-digit",
    hour: "2-digit", minute: "2-digit",
  });
}

export function fmtRelative(iso: string | undefined | null): string {
  if (!iso) return "—";
  const diffMs = Date.now() - new Date(iso).getTime();
  const s = Math.max(0, Math.round(diffMs / 1000));
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

export function fmtDuration(seconds: number | null | undefined): string {
  if (seconds == null) return "—";
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m < 60) return s ? `${m}m ${s}s` : `${m}m`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

export function fmtMs(ms: number | undefined | null): string {
  if (ms == null) return "—";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
}

export function fmtUptime(pct: number | null | undefined): string {
  if (pct == null) return "no data";
  return `${pct.toFixed(3)}%`;
}

export function fmtBytes(n: number | undefined | null): string {
  if (n == null) return "—";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} kB`;
  return `${(n / 1024 / 1024).toFixed(2)} MB`;
}

export const ERROR_CLASS_LABEL: Record<string, string> = {
  none: "None",
  dns_error: "DNS failure",
  connect_error: "Connection refused",
  tls_error: "TLS failure",
  timeout: "Timeout",
  http_error: "HTTP error",
  assertion_failed: "Assertion failed",
  redirect_error: "Redirect error",
  response_too_large: "Response too large",
  invalid_response: "Invalid response",
  internal_error: "Internal error",
  blocked_target: "Target blocked (SSRF guard)",
};

export function fmtInterval(s: number): string {
  if (s < 60) return `${s}s`;
  if (s < 3600) return s % 60 ? `${Math.floor(s / 60)}m ${s % 60}s` : `${s / 60}m`;
  return s % 3600 ? `${(s / 3600).toFixed(1)}h` : `${s / 3600}h`;
}
