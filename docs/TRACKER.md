# Implementation Tracker

> Live status of `docs/IMPLEMENTATION_PLAN.md`. Updated as work lands.
> Verification gate per phase: `make test` (build + vet + unit tests) green;
> DB-gated integration suites run under `make test-integration` when
> PostgreSQL is available.

## Overall status

| Phase | Scope | Status |
|-------|-------|--------|
| 0 | Requirements, architecture, ERD, threat model, API spec, plan | Done |
| 1 | Safe HTTP checker (SSRF-safe, no DB) | Done — 11 tests green |
| 2 | Persistence, monitor management, tenant isolation | Done |
| 3 | Scheduler and workers (durable queue, leases, recovery) | Done |
| 4 | Uptime calculations and incident engine | Done |
| 5 | REST API and authentication | Done (implemented subset; no refresh-token rotation yet) |
| 6 | Notifications (email/webhook, dedup, retry, history) | Not started |
| 7 | Frontend dashboard (Next.js) | Not started |
| 8 | SSL / DNS / keyword / redirect monitors | Not started |
| 9 | Status pages, deployment, load testing, prod checklist | Not started |

**Verification snapshot (latest `make test` run):** all packages pass —
`internal/{api,auth,checker/http,incident,safety,scheduler,store,uptime,worker}`.
DB-gated suites (store, scheduler, incident, uptime, api) skip unless
`TEST_DATABASE_URL` is set; the full integration run is pending a Postgres
instance (Docker was unavailable in the build environment).

## Implemented components

### Security
- `internal/safety` — exhaustive IP blocklists (v4/v6, loopback, private,
  link-local, multicast, reserved, cloud metadata), strict URL validation,
  validating resolver (all DNS answers must be public), per-dial
  re-validation inside the `Control` callback (DNS-rebinding/TOCTOU defense)
  with timing decomposition, bounded headers/body/time.
- API edge: SSRF validation of monitor URLs at create/update; private,
  loopback and metadata URLs rejected with `validation_failed` (tested).
- Auth: argon2id password hashing (PHC), HS256 JWT access tokens, Bearer
  middleware, RBAC middleware (viewer < editor < admin < owner), strict JSON
  body decoding, per-IP rate limiting (tighter on auth routes), security
  headers, panic recovery, one error envelope; internal errors never leak to
  clients.
- Tenant isolation: org membership checks and project-scope middleware;
  cross-tenant reads 404 (tested).

### Monitoring engine
- Durable scheduling in `monitor_jobs`: atomic claims via
  `FOR UPDATE SKIP LOCKED`, lease expiry reclamation, stale-completion
  pinning on `scheduled_at`, missed-schedule clamping (single make-up run,
  no backfill storm). Covered by DB-gated scheduler tests.
- Worker pool: bounded concurrency, blocking submit (backpressure),
  single retry with backoff for transient classes (timeout/dns/connect),
  drain-on-stop, panic guards; region stamped on every check.
- Safe HTTP checker: method/headers/body config, status-range rules,
  redirect chain capture with validation, bounded body reading, error
  classification (dns/connect/tls/timeout/http/redirect/assertion/...),
  timing decomposition (DNS, connect, TLS, TTFB, total).

### Data layer
- Migrations `0001` (full schema), `0002` (failure counter),
  `0003` (1-minute rollups + maintenance log); embedded down migrations and
  runner (`cmd/migrate`).
- Repositories: users, orgs (+memberships), projects, monitors, checks
  (idempotent insert, cursor paging, latest), jobs, incidents (open/resolve
  with row locks, events, scoped lists).
- Rollups and uptime: idempotent 1-minute aggregation job, prune,
  windowed summaries (24h/7d/30d/90d/custom) and chart series.
  Semantics: missing data = `no_data`, never counted as uptime/downtime.

### API (implemented — see `internal/api/openapi.yaml`, served at
`/api/v1/openapi.yaml`)
- `POST /auth/register`, `POST /auth/login`, `GET /auth/me`
- `GET|POST /organizations`, `GET /organizations/{orgID}`
- `GET|POST /organizations/{orgID}/projects`
- Monitors: `GET|POST .../monitors`, `GET|PUT|DELETE .../monitors/{id}`,
  `POST .../pause`, `POST .../resume`, `POST .../test`,
  `GET .../history`, `GET .../uptime`
- Incidents: `GET .../incidents`, `GET .../incidents/{incidentID}` (timeline)
- Health: `/healthz`, `/readyz`

Deliberate divergences from the Phase 0 sketch are documented in
[API_SPEC.md](API_SPEC.md) (register creates no auto-org; limit/offset +
cursor pagination instead of page/page_size; manual test is audit-only).

## Not implemented yet (by plan order)

1. **Phase 6 — notifications**: email/webhook channels, alert-policy
   binding, dispatcher with retry/backoff, delivery history, dedup
   (Postgres partial unique index authoritative; Redis `SET NX` optimization
   optional). The incident engine's `Notifier` hook is in place and unused.
2. **Phase 7 — frontend**: Next.js app (blocked locally on a working Node
   toolchain; GLIBCXX mismatch in the build environment).
3. **Phase 8 — SSL/DNS/keyword/redirect monitor types** (checker package
   extensions + API validation + UI detail cards).
4. **Phase 9 — status pages, Docker/Caddy deployment files, k6 load tests,
   production-readiness checklist** (including the missing
   `deploy/docker-compose.yml` that `make test-integration` references).

## Known gaps / debts

- Refresh-token rotation endpoints (`/auth/refresh`, logout) are not wired;
  access tokens are short-lived only. Tracked under Phase 5 tail work /
  Phase 6 (they belong with the auth hardening pass).
- Dashboard overview endpoint (aggregate counters) lands with Phase 5
  follow-up or Phase 7, whichever the frontend consumes first.
- `make test-integration` references `deploy/docker-compose.yml`, which does
  not exist yet (Phase 9 deliverable); with Docker down the Makefile path
  also depends on an exported `TEST_DATABASE_URL`.
- Audit-log writes are not yet wired into handlers (schema + index exist).
- Redis client is not integrated anywhere yet — by design until a
  measurable value case exists (dedup in Phase 6 is the first candidate).
- Production readiness is explicitly not claimed: the Phase 9 checklist
  (backup/restore drill, security suite under load, retention job run,
  monitoring of the monitors) has not been executed.
