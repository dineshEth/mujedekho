/** Types mirrored from internal/api/openapi.yaml. */

export interface User {
  id: string;
  email: string;
  full_name: string;
  created_at: string;
  updated_at?: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  created_at?: string;
}

export interface Project {
  id: string;
  org_id: string;
  name: string;
}

export type MonitorState = "up" | "down" | "paused";
export type MonitorType = "http" | "keyword" | "ssl" | "dns" | "redirect";
export type CheckStatus = "up" | "down" | "timeout";

export interface RequestConfig {
  method?: string;
  headers?: Record<string, string>;
  body?: string;
  expected_status_min?: number;
  expected_status_max?: number;
  follow_redirects?: boolean;
}

export interface CheckConfig {
  keyword?: string;
  keyword_condition?: "contains" | "not_contains";
  ssl_expiry_warn_days?: number;
  dns_record_type?: string;
  allowed_redirect_count?: number;
  allowed_redirect_target?: string;
  response_time_threshold_ms?: number;
}

export interface Monitor {
  id: string;
  org_id: string;
  project_id: string;
  name: string;
  monitor_type: MonitorType;
  url: string;
  request_config?: RequestConfig;
  check_config?: CheckConfig;
  interval_seconds: number;
  timeout_ms: number;
  failure_threshold: number;
  state: MonitorState;
  consecutive_failures?: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CheckResult {
  monitor_id?: string;
  checked_at: string;
  status: CheckStatus;
  http_status_code?: number;
  duration_ms: number;
  dns_ms?: number;
  connect_ms?: number;
  tls_ms?: number;
  ttfb_ms?: number;
  error_class?: string;
  error_detail?: string;
  response_bytes?: number;
  region?: string;
}

export interface Incident {
  id: string;
  monitor_id: string;
  status: "open" | "resolved";
  started_at: string;
  resolved_at?: string | null;
  resolved_duration_s?: number | null;
  cause_class?: string;
}

export interface IncidentEvent {
  id: string;
  incident_id: string;
  event_type: string;
  payload?: Record<string, unknown>;
  created_at: string;
}

export interface UptimeSummary {
  window_start: string;
  window_end: string;
  uptime_pct: number | null;
  checks_total: number;
  checks_up: number;
  downtime_seconds: number;
  no_data: boolean;
}

export interface UptimeBucket {
  bucket_start: string;
  checks_total: number;
  checks_up: number;
  avg_response_ms: number;
}

export interface ApiErrorBody {
  error?: { code?: string; message?: string; details?: Record<string, unknown> };
}
