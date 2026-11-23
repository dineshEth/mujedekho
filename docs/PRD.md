# PRD — Website Monitoring SaaS

> Phase 0 deliverable. Version 1.0. Scope is exclusively website (synthetic HTTP/SSL/DNS/keyword) monitoring.
> Server monitoring, APM, log management, cloud, and Kubernetes monitoring are explicitly out of scope.

## 1. Problem statement

Teams learn about website outages from their users, not from their tooling. This platform provides
continuous, externally observed monitoring of websites: availability, response time, TLS certificate
validity, DNS resolution, page-content assertions, and redirect behavior — with durable incident
history and reliable alerting.

## 2. Personas

| Persona | Needs |
|---|---|
| SRE / Ops engineer | Precise uptime and latency data, incident timelines, fast triage |
| Developer | Keyword/redirect assertions on their own app, API access |
| Agency / consultant | Multi-organization, multi-project management, client-facing status pages |
| Engineering manager | Dashboards, uptime percentages over 24h/7d/30d/90d, MTTR trends |

## 3. Tenancy model

- **User** — an authenticated account (email + password, argon2id hash).
- **Organization** — billing and isolation boundary. A user can belong to many organizations.
- **Membership** — a user's role in an organization (`owner`, `admin`, `editor`, `viewer`).
- **Project** — a grouping of monitors inside an organization (e.g. "Marketing sites", "Checkout").
  Monitors, notification channels, alert policies, and status pages belong to a project.
- **Authorization rule** — all data access is scoped by `project_id → organization_id → membership`.
  There is no cross-organization read path anywhere in the API.

## 4. Feature requirements

### 4.1 Monitors

| Requirement | Definition |
|---|---|
| FR-1 | Create HTTP/HTTPS monitors: URL (https preferred), method (GET/HEAD/POST), headers, optional body, timeout (default 10s, max 30s), check interval (default 60s, min 30s, max 3600s), failure threshold before DOWN, expected HTTP status class |
| FR-2 | Monitor states: `up`, `down`, `paused`. Pause/resume stops and restarts scheduling without losing history |
| FR-3 | Manual test: run one check on demand, return the full result synchronously |
| FR-4 | Monitor types (Phase 8): keyword (contains / not-contains, bounded body read), SSL (cert validity, issuer, expiry, warning window), DNS (record resolution, latency), redirect (chain, final URL, unexpected-target assertion) |
| FR-5 | Per-monitor named regions concept: results carry a `region` label (single-region deployment first, multi-region field-supported) |

### 4.2 Engine

| Requirement | Definition |
|---|---|
| FR-6 | Scheduler assigns each due monitor to a durable job queue; workers execute checks with bounded concurrency (default limit, env-configurable) |
| FR-7 | No overlapping checks for the same monitor; a skipped tick is missed, not queued up (missed schedule = run at next interval boundary) |
| FR-8 | Retries: network-level errors retried once with short backoff inside the worker; a retry never masks the "no overlap" rule |
| FR-9 | Results record: timestamp, monitor ID, status, HTTP status code, DNS/connect/TLS/TTFB/total timings, error classification, region, duration |

### 4.3 Analytics

| Requirement | Definition |
|---|---|
| FR-10 | Uptime over 24h / 7d / 30d / 90d and arbitrary custom ranges from checks |
| FR-11 | Uptime definition: a check is "up" only when result status is `up`. Downtime between the first failing check and the next succeeding check is counted. **Missing data** (monitor paused, worker outage, new monitor before first check) is excluded from the denominator and rendered as gaps, never as downtime or uptime |
| FR-12 | Response-time charts: p50/p90/p95/p99 and average, bucketed (1min for 24h view, coarser for longer ranges) |

### 4.4 Incidents

| Requirement | Definition |
|---|---|
| FR-13 | An incident opens after N consecutive failing checks (configurable per alert policy, default 3) |
| FR-14 | Recovery detected on the first succeeding check; incident duration = recovered_at − started_at |
| FR-15 | Incident events timeline: created, state transitions, notification sent/failed, manual annotations |
| FR-16 | Notification dedup: one "down" notification per (incident, channel) unless policy re-notifies at X minutes of ongoing downtime; one "recovery" per incident |

### 4.5 Notifications

| Requirement | Definition |
|---|---|
| FR-17 | Channels: email (SMTP) and webhook (JSON POST with signed payload) |
| FR-18 | Alert policies bind monitors to channels with conditions (down, recovered, ssl-expiry, response-time-threshold) |
| FR-19 | Delivery retries with exponential backoff (5 attempts), full delivery history with status, latency, and error message |

### 4.6 Status pages

| Requirement | Definition |
|---|---|
| FR-20 | Public, unauthenticated status pages at `/status/{slug}` with selected monitors, 90-day uptime bars, current state, and public incidents |
| FR-21 | Only explicitly selected monitors and only incidents on those monitors appear; no tenant metadata leaks |

### 4.7 Dashboard

| Requirement | Definition |
|---|---|
| FR-22 | Per-organization/project: total monitors, up, down, paused, overall uptime 24h, avg response time, recent incidents, response-time chart |

## 5. Non-functional requirements

| ID | Requirement | Target |
|---|---|---|
| NFR-1 | Check freshness | ≤ interval + 5s p95 scheduling delay |
| NFR-2 | Scale (initial) | 10k monitors, ≥ 100k checks/hour sustained |
| NFR-3 | API latency | p95 < 200ms for dashboard endpoints |
| NFR-4 | Durability | Scheduler state survives restart with zero lost monitors |
| NFR-5 | Security | SSRF-hardened egress; OWASP ASVS L2 for the control plane |
| NFR-6 | Observability | Structured JSON logs (slog), `/healthz` + `/readyz`, Prometheus metrics endpoint |
| NFR-7 | Retention | Raw checks 30 days (default), rollups 13 months, configurable |
| NFR-8 | Availability | Single-region deployment, graceful shutdown drains in-flight checks |

## 6. Explicitly out of scope

Server/cloud/K8s monitoring, APM, log management, RUM, alerting via SMS/Slack/PagerDuty (extensible
later via the channel interface), multi-region probe execution (schema supports it; engine ships
single-region first), billing/payment processing.

## 7. Success criteria for v1

A customer can sign up, create a project, add a monitor, receive an email within one interval of a
real outage, see the incident on the dashboard and public status page, and resolve it — with every
state change persisted and queryable through the API.
