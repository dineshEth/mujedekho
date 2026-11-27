# site24x7 — Website Monitoring SaaS (Go)

A production-oriented, multi-tenant **website monitoring** platform inspired by
Site24x7's website monitoring capabilities. This project implements **website
monitoring only** — no server/cloud/APM/log/Kubernetes monitoring.

- Availability monitoring of http(s) endpoints with SSRF-safe check execution
- Durable scheduling (Postgres job queue, leases, crash-safe recovery)
- Uptime analytics (24h/7d/30d/90d/custom) with explicit no-data semantics
- Incident lifecycle (failure confirmation threshold → open → recovery → timeline)
- Versioned REST API under `/api/v1` with JWT auth, RBAC, tenant isolation

**Status: Phases 0–5 (backend) and Phase 7 (Next.js dashboard) are
implemented; `make test` and `web` build + typecheck are green. See
[docs/TRACKER.md](docs/TRACKER.md) for the live progress tracker. Not
production-ready yet — the Phase 9 checklist has not been run.**

## Stack

| Layer     | Choice |
|-----------|--------|
| Backend   | Go 1.26, `net/http` + chi, pgx/v5 |
| Database  | PostgreSQL (source of truth incl. job queue), SQL migrations |
| Auth      | Argon2id password hashing, JWT access tokens, role model |
| Frontend  | Next.js 15 (App Router), TypeScript, Tailwind CSS, Recharts |
| Infra     | Docker Compose for local dev (Phase 9) |

## Architecture

```
customer -> cmd/api (REST /api/v1)
                | writes
                v
          PostgreSQL (monitors, monitor_jobs, checks, incidents)
                ^ claims due jobs (FOR UPDATE SKIP LOCKED + leases)
                |
          cmd/worker: scheduler -> worker pool -> safe HTTP checker
                -> incident engine -> checks/monitor state/incidents
```

- **Scheduling is durable and recoverable**: jobs live in `monitor_jobs`;
  claims are atomic (`SKIP LOCKED`), leases expire so crashed workers'
  jobs are reclaimed, and completions are pinned on `scheduled_at` so a
  stale worker can never clobber a newer claim or backfill-storm missed runs.
- **SSRF defense** (`internal/safety`): exhaustive IP blocklists (loopback,
  private, link-local, multicast, reserved, cloud metadata, v6 mapped),
  validating DNS resolution (all answers must be public), per-dial
  re-validation in the `Control` callback (anti-DNS-rebinding, no TOCTOU),
  bounds on headers, body size, and total time.
- **Uptime semantics**: missing data is reported as `no_data` and never
  counted as uptime or downtime. 1-minute rollups power long windows.

## Quickstart (local development)

Prerequisites: Go 1.26+, Node 20+, PostgreSQL (or Docker/Podman for the
bundled compose file), optionally Redis.

```bash
make db-up                   # postgres + redis via deploy/docker-compose.yml
cp .env.example .env         # then set JWT_SECRET (32+ chars); .env is auto-loaded

go run ./cmd/migrate         # optional: api/worker also migrate on start (advisory-locked)
SEED_PASSWORD=dev-password-only go run ./cmd/seed   # optional dev seed (refuses production)

go run ./cmd/api             # REST API on :8080
go run ./cmd/worker          # scheduler + check workers + rollups/retention
cd web && npm install && npm run dev   # dashboard on http://localhost:3000
```

Register in the dashboard; the first visit walks you through creating an
organization and project, then add a monitor.

Required environment (see `.env.example` for the full list):

- `DATABASE_URL` — Postgres DSN
- `JWT_SECRET` — 32+ random bytes (mandatory outside `APP_ENV=development`)
- Optional: `WORKER_CONCURRENCY`, `SCHEDULER_TICK_MS`, `REGION`, `DNS_RESOLVERS`

Health endpoints: `GET /healthz`, `GET /readyz`.
OpenAPI 3.1 document: `GET /api/v1/openapi.yaml` (embedded in the server
binary; canonical source `internal/api/openapi.yaml`).

## Frontend (web/)

A Next.js 15 dashboard talks to the API above. The browser never sees the
JWT: login/register/lobby through Next route handlers that set an httpOnly
cookie, and a catch-all server-side proxy attaches the Bearer header.

```bash
cd web
npm install
npm run dev                  # http://localhost:3000

# Point the BFF at the API (defaults to http://localhost:8080):
API_BASE_URL=http://localhost:8080 npm run dev
```

Pages: Login/Register, Dashboard, All Monitors, Add Monitor, Monitor
Details (uptime windows, response-time chart, check history, incidents,
pause/resume/manual test), Incidents, Notifications and Status Pages
(placeholder sections for their backend phases), Project/Account settings.
Tailwind with semantic design tokens, dark/light theme, and UP/DOWN/PAUSED
state language throughout.

Production build (verified green):

```bash
cd web && npm run build && npm start
```

## API walkthrough (curl)

```bash
# Register (returns an access token)
curl -s localhost:8080/api/v1/auth/register -d '{
  "email": "you@example.com", "password": "at-least-12-chars", "full_name": "You"}'

TOKEN=...   # access_token from the response

# Create organization (creator becomes owner) and a project
curl -s localhost:8080/api/v1/organizations -H "Authorization: Bearer $TOKEN" \
  -d '{"name": "Acme", "slug": "acme"}'
curl -s localhost:8080/api/v1/organizations/$ORG_ID/projects \
  -H "Authorization: Bearer $TOKEN" -d '{"name": "prod"}'

# Create + arm a monitor (private/loopback URLs are rejected at the edge)
BASE=/api/v1/organizations/$ORG_ID/projects/$PROJ_ID/monitors
curl -s localhost:8080$BASE -H "Authorization: Bearer $TOKEN" -d '{
  "name": "example", "url": "https://example.com/",
  "interval_seconds": 60, "timeout_ms": 10000, "failure_threshold": 3}'
curl -s localhost:8080$BASE/$MON_ID/resume -H "Authorization: Bearer $TOKEN"

# Operate it
curl -s "localhost:8080$BASE/$MON_ID/history?window_minutes=1440" -H "Authorization: Bearer $TOKEN"
curl -s "localhost:8080$BASE/$MON_ID/uptime?window=24h" -H "Authorization: Bearer $TOKEN"
curl -s localhost:8080$BASE/$MON_ID/test -X POST -H "Authorization: Bearer $TOKEN"
```

## Repository layout

```
├── cmd/
│   ├── api/          REST API server
│   ├── worker/       scheduler + worker pool + incident engine
│   ├── migrate/      SQL migration runner
│   └── seed/         development seed data
├── internal/
│   ├── api/          HTTP handlers, middleware, OpenAPI document
│   ├── auth/         argon2id passwords, JWT tokens
│   ├── checker/      safe HTTP checker (timings, error classification)
│   ├── config/       environment parsing and validation
│   ├── domain/       entities and pure business rules
│   ├── incident/     incident engine (threshold → open → recovery)
│   ├── observability/ structured slog logging, panic guards
│   ├── safety/       SSRF blocklists, validating resolver, pinned dialer
│   ├── scheduler/    durable job claiming loop
│   ├── store/        pgx repositories + SQL migrations
│   ├── uptime/       rollups, uptime summaries, series
│   └── worker/       bounded-concurrency pool, probe adapters
├── web/              Next.js dashboard (BFF auth proxy, Tailwind, Recharts)
├── docs/             design documents + progress tracker
└── Makefile          build / test / vet / integration gate
```

## Testing

```bash
make test                # build + vet + unit tests (no DB required)
make test-integration    # DB-gated suites; skips when postgres is unavailable
```

Suites that need PostgreSQL (store, scheduler, incident lifecycle, uptime,
API integration) skip automatically unless `TEST_DATABASE_URL` is set.
When Docker is available, `make test-integration` starts the services first.

## Design documents

- [docs/PRD.md](docs/PRD.md) — product requirements, features, non-functional targets
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — system architecture, diagrams, repository layout
- [docs/DATABASE.md](docs/DATABASE.md) — ERD, schema, indexing, partitioning/retention strategy
- [docs/API_SPEC.md](docs/API_SPEC.md) — REST API contract + implemented divergences
- [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) — STRIDE analysis, SSRF defense design
- [docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md) — phased delivery plan
- [docs/TRACKER.md](docs/TRACKER.md) — live implementation progress tracker
