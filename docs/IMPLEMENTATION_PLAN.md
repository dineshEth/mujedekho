# Phased Implementation Plan

> Phase 0 deliverable. Each phase must compile, run, and pass its tests before the next begins.
> Adjusted minimally from the requested sequence to respect dependency order (the HTTP checker
> before persistence means its unit tests need no database).

## Phase 1 — Safe HTTP checker (pure Go, no DB)

- `internal/safety`: IP blocklists (v4+vi, all RFCs), URL validation, encoding normalization,
  DNS pinning resolver, dialer with `Control` callback re-validation.
- `internal/checker/http`: timing decomposition (DNS, connect, TLS, TTFB, total), status rules,
  error classification (`ErrorClass` enum), bounded body reading, safe redirect policy,
  configurable method/headers/timeout.
- Tests: table-driven blocklist, rebinding simulation, redirect validation, limits via `httptest`.
- Exit: `go test ./internal/safety ./internal/checker/...` green; checker demoable in `main`.

## Phase 2 — Persistence and monitor management

- Go module skeleton, pgx pool, migrations runner, full schema from `docs/DATABASE.md` (v1 files).
- `internal/domain` + `internal/store` repositories (monitors, orgs, projects, users).
- Tenant-scoped repo functions; isolation tests.
- Seed data for dev.
- Exit: migrations up/down clean, repo tests green against Postgres container.

## Phase 3 — Scheduler and workers

- `monitor_jobs` claiming with `FOR UPDATE SKIP LOCKED`, lease + expiry recovery, missed-schedule
  clamping, `_test.go` for duplicate-claim and crash-recovery using real Postgres.
- Worker pool with semaphore, per-check context, single retry with backoff, result writes.
- Integration with Phase 1 checker; end-to-end check of an `httptest` target persisted to DB.
- `cmd/worker`; graceful shutdown verified by test (SIGTERM during in-flight check).
- Exit: monitor added via seed is checked every interval; results visible in `monitoring_checks`.

## Phase 4 — Uptime calculations and incident engine

- Rollup job (1-minute aggregates), retention scaffolding (job + config, partition notes enforced).
- Uptime service: 24h/7d/30d/90d/custom, defined no-data semantics, bucketed series.
- Incident engine: threshold state machine, recovery detection, `incident_events`, dedup gates
  (Postgres-side uniqueness authoritative).
- Tests for boundary conditions: paused monitors, data gaps, threshold exactly reached, flapping.
- Exit: seeded week of data yields correct uptime percentages in unit tests.

## Phase 5 — REST API and authentication

- `chi` router, middleware chain, domain handlers for everything except notifications, OpenAPI 3.1
  generated from routes, error envelope, pagination, validation.
- Auth: register/login/refresh-rotation/logout, JWT middleware, RBAC middleware, audit logging.
- API tests: router-level with real DB (auth matrix 401/403 per role), rate-limit middleware tests.
- Exit: curl-driven walkthrough of monitor CRUD + history + uptime; `openapi.yaml` validates.

## Phase 6 — Notifications

- Channel registry (email SMTP, webhook with HMAC signing), alert policy binding, dispatcher with
  backoff retry, delivery history, dedup via Redis `SET NX` + Postgres constraint fallback.
- SSRF validation of webhook URLs at dispatch (reuses `internal/safety`).
- Tests: fake SMTP + `httptest` webhook target, backoff timing, dedup, signature verification.
- Exit: induced outage sends one down + one recovery notification per channel.

## Phase 7 — Frontend dashboard

- Next.js (App Router, TypeScript, Tailwind, shadcn/ui, Recharts), dark/light tokens, sidebar +
  topbar layout, org/project selector, command palette (global search).
- Pages: Login, Dashboard, All Monitors, Add Monitor (wizard), Monitor Details, Incidents,
  Notification Settings, Status Pages, Project Settings, Account Settings.
- States: skeletons, empty, error, UP/DOWN/PAUSED uptime bars; Recharts response-time charts.
- API client generated from openapi.yaml (types + fetch wrapper), auth via httpOnly cookie set by
  BFF route or bearer-in-memory (decided at implementation; lean: BFF route sets httpOnly cookie,
  avoids token-in-localStorage).
- Exit: all pages render against local API; basic Playwright smoke test for login + create monitor.

## Phase 8 — SSL, DNS, keyword, redirect monitors

- `checker/ssl` (direct TLS inspect via pinned dial to avoid rebinding), `checker/dns`,
  `checker/keyword` (bounded body), `checker/redirect` (chain capture).
- API validation per type; Monitor Details UI extensions (cert card, DNS answers, redirect chain).
- Alert triggers for `ssl_expiry` and `response_time` thresholds.
- Tests per checker incl. malformed certificates (self-signed test certs).
- Exit: one monitor of each type verified end-to-end.

## Phase 9 — Status pages, testing, deployment, performance

- Public status pages (Go-rendered or consumed by Next.js; decision: server-render HTML from Go at
  `/status/{slug}` for cache-ability and isolation from the app session).
- Load tests: `k6` against check pipeline (10k monitor simulation, kill worker mid-run, assert
  recovery), API load profile, Postgres index verification under load.
- Deploy: multi-stage Dockerfiles, docker-compose prod profile, Caddy config, `.env.example`,
  README.
- Production-readiness checklist: backups/restore drill, retention job run, security suite green,
  Grafana/Prometheus scrape config, cost notes. Explicitly: production readiness is *claimed only
  after* this checklist is executed.

## Verification per phase (always)

`make lint && make test && make test-integration` — integration suite uses `docker compose up -d
postgres redis test` automatically; CI runs the same. No phase advances with red tests or TODOs in
core paths.
