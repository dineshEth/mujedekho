# API Specification

> Phase 0 deliverable. Versioned under `/api/v1`. JSON only, `Content-Type: application/json`.
> Auth: `Authorization: Bearer <JWT>`. Served as an OpenAPI 3.1 document at `/api/v1/openapi.yaml`
> (generated from route definitions so it cannot drift).
>
> Status: **Phase 5 implemented subset live as of the API build.** The canonical machine-readable
> contract for what is implemented now is `internal/api/openapi.yaml` (embedded in the server
> binary and served at `/api/v1/openapi.yaml`). Deliberate divergences from the Phase 0 sketch,
> decided during implementation:
>
> - Auth/register returns access token + user and does **not** auto-create an organization;
>   org creation is explicit (`POST /api/v1/organizations`) so invitation flows stay possible.
> - Pagination uses `limit`/`offset` for lists and RFC3339Nano cursors for check history
>   (`cursor`), instead of `page`/`page_size`; cursor paging avoids drift when rows move pages.
> - Monitor URLs are validated at the edge by the SSRF validator — customers never receive
>   driver-level errors, only `validation_failed` with a field detail.
> - `POST .../monitors/{id}/test` performs a synchronous check and persists it as an audit row,
>   but does not touch incident state or failure counters.

## 1. Conventions

- **Error envelope** (consistent everywhere):
  ```json
  { "error": { "code": "validation_failed", "message": "human readable", "details": [ { "field": "url", "message": "must be http(s) URL" } ], "request_id": "01J..." } }
  ```
  Codes: `validation_failed` (400), `unauthorized` (401), `forbidden` (403), `not_found` (404),
  `conflict` (409), `rate_limited` (429, + `Retry-After`), `internal` (500, no internals leaked).
- **Pagination**: `?page=1&page_size=50` (max 100). Response wrapper:
  ```json
  { "data": [...], "page": 1, "page_size": 50, "total": 143 }
  ```
- **Validation**: declarative via request structs, checked before handler logic; unknown fields rejected (`json:DisallowUnknownFields`).
- **Idempotency-sensitive ops** (create monitor, test) accept optional `Idempotency-Key` header.

## 2. Auth

| Method | Path | Notes |
|---|---|---|
| POST | `/api/v1/auth/register` | email, password (min 12 chars, checked against breach list length heuristics), full name. Returns tokens + creates first org (owner) |
| POST | `/api/v1/auth/login` | Rate limited. Returns `{access_token, refresh_token, expires_in}` |
| POST | `/api/v1/auth/refresh` | Refresh token rotation: old token revoked atomically, reuse of a revoked token revokes the whole family |
| POST | `/api/v1/auth/logout` | Revokes refresh token |
| GET | `/api/v1/auth/me` | Current user + org memberships |

Access token TTL 15 min; refresh TTL 30 days, single-use, hashed at rest.

## 3. Organizations, members, projects

| Method | Path | Roles |
|---|---|---|
| GET/POST | `/api/v1/organizations` | any auth / any |
| GET/PATCH | `/api/v1/organizations/{org_id}` | member / admin+ |
| GET/POST | `/api/v1/organizations/{org_id}/members` | admin+ / owner |
| DELETE | `/api/v1/organizations/{org_id}/members/{user_id}` | owner |
| GET/POST | `/api/v1/organizations/{org_id}/projects` | viewer (read) / editor+ (write, but editors can only manage monitors in projects they access) |
| GET/PATCH/DELETE | `/api/v1/organizations/{org_id}/projects/{project_id}` | admin+ |

Role matrix: `viewer` read-only; `editor` monitor/channel/policy CRUD; `admin` project + member
management minus removal of owners; `owner` everything including member removal and org deletion.
Enforced in middleware; handlers assume an authorized, org-scoped context.

## 4. Monitors

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/organizations/{org_id}/projects/{project_id}/monitors` | filters: `state=up,down,paused`, `type`, `search`; sort `name|state|uptime_24h` |
| POST | `.../monitors` | see below |
| GET | `.../monitors/{monitor_id}` | includes `latest_check` summary and 24h uptime |
| PATCH | `.../monitors/{monitor_id}` | partial update; interval/timeout re-validated; schedule reset |
| DELETE | `.../monitors/{monitor_id}` | soft — pauses + flags deleted, history retained |
| POST | `.../monitors/{monitor_id}/pause` | `{ "paused": true/false }` or dedicated `/resume`; scheduler removes/re-adds job |
| POST | `.../monitors/{monitor_id}/test` | runs one check synchronously, 15s server-side cap; returns full result. Rate limited: 10/min/project |
| GET | `.../monitors/{monitor_id}/checks` | `?from&to&cursor`, cursor = `(checked_at, monotonic tiebreaker)` base64 |
| GET | `.../monitors/{monitor_id}/uptime` | `?period=24h|7d|30d|90d|custom&from&to` → per-period uptime %, buckets for chart, no_data windows flagged |
| GET | `.../monitors/{monitor_id}/incidents` | paged |

Create-monitor body:

```json
{
  "name": "Checkout API",
  "url": "https://shop.example.com/checkout",
  "monitor_type": "http",
  "interval_seconds": 60,
  "timeout_ms": 10000,
  "failure_threshold": 3,
  "request_config": {
    "method": "GET",
    "headers": { "X-Trace-From": "site24x7" },
    "expected_status_min": 200,
    "expected_status_max": 299,
    "follow_redirects": true
  },
  "check_config": {
    "keyword": "Order confirmed",
    "keyword_condition": "contains",
    "ssl_expiry_warn_days": 14,
    "record_type": "A",
    "allowed_redirect_count": 3,
    "allowed_redirect_target": "https://shop.example.com/cart"
  }
}
```

`monitor_type` selects which `check_config` fields apply; server validates the intersection
(`keyword` requires `keyword` + condition; `ssl` accepts any URL but asserts TLS; `dns` requires a
hostname — URL field may hold the hostname).

## 5. Uptime & dashboard

| Method | Path | Notes |
|---|---|---|
| GET | `.../projects/{project_id}/dashboard` | totals (up/down/paused), uptime_24h, avg_response_ms, recent_incidents[10], response_time series (60×1min buckets) |
| GET | `.../projects/{project_id}/uptime` | `?period` aggregated across all monitors |

Uptime response:

```json
{
  "period": "7d", "from": "2026-06-01T00:00:00Z", "to": "2026-06-08T00:00:00Z",
  "uptime_pct": 99.972,
  "total_checks": 10080, "up_checks": 10077,
  "no_data_pct": 0.0,
  "buckets": [ { "start": "...", "uptime_pct": 100.0, "avg_response_ms": 412, "no_data": false } ]
}
```

## 6. Incidents

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/organizations/{org_id}/incidents` | filters `status=open|resolved`, `monitor_id`, `from`, `to`; cross-project (org-wide) view |
| GET | `.../incidents/{incident_id}` | includes event timeline |
| POST | `.../incidents/{incident_id}/annotations` | `{ "note": "..." }` — editor+ |

## 7. Notification channels, alert policies, deliveries

| Method | Path |
|---|---|
| GET/POST | `.../projects/{project_id}/notification-channels` |
| GET/PATCH/DELETE | `.../notification-channels/{id}` |
| POST | `.../notification-channels/{id}/test` — fires test delivery, returns attempt result |
| GET/POST | `.../projects/{project_id}/alert-policies` |
| GET/PATCH/DELETE | `.../alert-policies/{id}` |
| PUT | `.../alert-policies/{id}/monitors` — `{ "monitor_ids": [...] }` (full replace) |
| GET | `.../projects/{project_id}/deliveries?incident_id=&status=` |

Channel config is stored server-side; `GET` never returns webhook signing secrets — only
`has_secret: true`. A new secret must be set wholesale via PATCH.

## 8. Status pages

| Method | Path | Auth |
|---|---|---|
| GET/POST | `.../projects/{project_id}/status-pages` | yes, editor+ |
| GET/PATCH/DELETE | `.../status-pages/{id}` | yes |
| PUT | `.../status-pages/{id}/monitors` | yes |
| GET | `/status/{slug}` | **public**, no auth; pinned to published pages only |
| GET | `/status/{slug}/incidents` | **public** |

Public page response intentionally omits org/project ids, monitor URLs are shown truncated to host,
and only `open`/`resolved` incidents in the last 90 days appear.

## 9. Health, metrics, docs

| Method | Path | Notes |
|---|---|---|
| GET | `/healthz` | liveness: process up |
| GET | `/readyz` | readiness: DB ping, Redis ping (degraded OK), migrations current |
| GET | `/metrics` | Prometheus format, internal network only |
| GET | `/api/v1/openapi.yaml` | OpenAPI 3.1 |

## 10. Rate limits (per access token + IP)

| Scope | Limit |
|---|---|
| `/auth/*` | 10/min/IP |
| General API | 600/min/token |
| Monitor test | 10/min/project |
| Webhook deliveries (outbound) | per-target 30/min, backoff on 429/5xx |

Implemented via Redis fixed-window; in-process fallback per instance (documented as degraded, not
exact).
