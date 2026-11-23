# Database Design

> Phase 0 deliverable. PostgreSQL 16+. All timestamps `timestamptz` stored/served in UTC.
> All primary keys UUID v4 (`gen_random_uuid()`).

## 1. ERD

```mermaid
erDiagram
    users ||--o{ memberships : "joins"
    organizations ||--o{ memberships : "has"
    organizations ||--o{ projects : "owns"
    projects ||--o{ monitors : "contains"
    projects ||--o{ notification_channels : "has"
    projects ||--o{ alert_policies : "has"
    projects ||--o{ status_pages : "publishes"
    monitors ||--o{ monitoring_checks : "produces"
    monitors ||--o{ incidents : "opens"
    monitors }o--o{ alert_policies : "bound via monitor_alert_policies"
    alert_policies ||--o{ alert_policy_channels : "routes to"
    notification_channels ||--o{ alert_policy_channels : ""
    incidents ||--o{ incident_events : "timeline"
    incidents ||--o{ notification_deliveries : "triggers"

    users {
        uuid id PK
        text email UK "citext, unique"
        text password_hash "argon2id"
        text full_name
        timestamptz created_at
        timestamptz updated_at
    }
    organizations {
        uuid id PK
        text name
        text slug UK
        timestamptz created_at
    }
    memberships {
        uuid id PK
        uuid org_id FK
        uuid user_id FK
        text role "owner|admin|editor|viewer"
        timestamptz created_at
        "UNIQUE(org_id, user_id)"
    }
    projects {
        uuid id PK
        uuid org_id FK
        text name
        timestamptz created_at
        "UNIQUE(org_id, name)"
    }
    monitors {
        uuid id PK
        uuid project_id FK
        uuid org_id "denormalized for index locality"
        text name
        text monitor_type "http|keyword|ssl|dns|redirect"
        text url
        jsonb request_config "method, headers, body, expected_status, ..."
        jsonb check_config "keyword, ssl_expiry_warn_days, dns_record, allowed_redirects"
        int interval_seconds "30..3600, default 60"
        int timeout_ms "1000..30000, default 10000"
        text state "up|down|paused"
        int failure_threshold "default 3"
        uuid created_by FK users
        timestamptz created_at
        timestamptz updated_at
    }
    monitoring_checks {
        uuid monitor_id
        timestamptz checked_at
        text status "up|down|timeout"
        int http_status_code
        int duration_ms
        int dns_ms, connect_ms, tls_ms, ttfb_ms
        text error_class "dns_error|tls_error|connect_error|timeout|http_error|assertion_failed|internal_error|none"
        text error_detail "sanitized"
        int response_bytes
        text region
        jsonb detail "ssl cert info, dns answers, redirect chain"
        "PARTITION BY RANGE (checked_at)"
    }
    monitoring_check_rollups_1m {
        uuid monitor_id
        timestamptz bucket_start
        int checks_total
        int checks_up
        bigint duration_ms_sum
        int duration_ms_p50, p90, p95, p99
    }
    incidents {
        uuid id PK
        uuid monitor_id FK
        timestamptz started_at
        timestamptz resolved_at
        int resolved_duration_s
        text cause_class
        text status "open|resolved"
    }
    incident_events {
        uuid id PK
        uuid incident_id FK
        text event_type "opened|recovered|notified|notification_failed|annotated"
        jsonb payload
        timestamptz created_at
    }
    notification_channels {
        uuid id PK
        uuid project_id FK
        text type "email|webhook"
        jsonb config "emails[] | url + secret_ref"
        boolean enabled
    }
    alert_policies {
        uuid id PK
        uuid project_id FK
        text name
        text trigger "down|recovered|ssl_expiry|response_time"
        jsonb conditions "thresholds, renotify_after_min"
        boolean enabled
    }
    monitor_alert_policies {
        uuid monitor_id FK
        uuid policy_id FK
        "PK(monitor_id, policy_id)"
    }
    alert_policy_channels {
        uuid policy_id FK
        uuid channel_id FK
        "PK(policy_id, channel_id)"
    }
    notification_deliveries {
        uuid id PK
        uuid incident_id FK
        uuid channel_id FK
        uuid policy_id FK
        text kind "down|recovered|renotify"
        text status "pending|sent|failed|suppressed"
        text error "sanitized"
        int response_ms
        int attempt
        timestamptz created_at
    }
    status_pages {
        uuid id PK
        uuid project_id FK
        text slug UK
        text headline
        boolean published
    }
    status_page_monitors {
        uuid status_page_id FK
        uuid monitor_id FK
        int display_order
        "PK(status_page_id, monitor_id)"
    }
    refresh_tokens {
        uuid id PK
        uuid user_id FK
        text token_hash
        timestamptz expires_at
        timestamptz revoked_at
    }
    audit_logs {
        bigint id PK
        uuid org_id
        uuid user_id
        text action
        text resource_type
        uuid resource_id
        jsonb diff "redacted"
        inet ip
        timestamptz created_at
    }
    monitor_jobs {
        uuid monitor_id PK
        timestamptz next_run_at
        timestamptz scheduled_at
        text status "pending|leased"
        timestamptz lease_expires_at
        int attempts
    }
```

## 2. Index strategy (high-traffic paths)

| Query | Index |
|---|---|
| Last N checks for a monitor (history API, status badge) | `monitoring_checks (monitor_id, checked_at DESC)` — this is also the partition key index |
| Uptime aggregate over range | Same index; range scan with partial aggregation; 24h/7d answered preferentially from `monitoring_check_rollups_1m` |
| Scheduler due-claim | `monitor_jobs (next_run_at) WHERE status = 'pending'` |
| Incident list per project | `incidents (monitor_id, started_at DESC)` + join through monitors on `project_id` |
| Delivery retry sweep | `notification_deliveries (status, next_retry_at) WHERE status='pending'` |
| Rollup upsert | PK `(monitor_id, bucket_start)` |
| Audit lookup | `audit_logs (org_id, created_at DESC)` |

Only range-partitioned table: `monitoring_checks`.

## 3. Partitioning and retention — the plan and the trigger point

We will **not** start with partitioned tables in Phase 1. An unpartitioned table with the composite
index above serves tens of millions of rows fine on modest hardware. The design below is
pre-agreed so the migration is a planned cutover, not an emergency.

- **Trigger:** ≥ 20M rows in `monitoring_checks` or raw checks exceeding 50 GB, whichever first
  (10k monitors × 1 check/min = 14.4M rows/day at full scale, so this arrives at ~1.4 days in the
  worst case — which is why the partition DDL is written now).
- **Scheme:** `PARTITION BY RANGE (checked_at)`, daily partitions, created ahead by a scheduler job
  (7 days future), named `monitoring_checks_YYYYMMDD`.
- **Retention:** raw checks 30 days (env `CHECK_RETENTION_DAYS`); the rollup job detaches and drops
  partitions whole — `DROP TABLE` is O(1) versus gigabyte-scale `DELETE`s. Rollups: hourly table
  pruned to 90 days; monthly rollups to 13 months.
- **Cutover:** new table with same name created partitioned in a migration, data copied per-day
  window under advisory lock, swap under `RENAME`. Brief write pause accepted, read replicas
  unaffected.
- **Downtime semantics caveat, encoded anywhere uptime is computed:** missing ranges because data
  was dropped by retention are reported as `no_data`, never as availability 100%.

## 4. DDL sketches (canonical files land in `internal/store/migrations/`)

```sql
CREATE TABLE monitoring_checks (
    monitor_id       uuid        NOT NULL REFERENCES monitors(id),
    checked_at       timestamptz NOT NULL,
    status           text        NOT NULL CHECK (status IN ('up','down','timeout')),
    http_status_code int         CHECK (http_status_code BETWEEN 100 AND 599),
    duration_ms      int         NOT NULL CHECK (duration_ms >= 0),
    dns_ms           int, connect_ms int, tls_ms int, ttfb_ms int,
    error_class      text        NOT NULL DEFAULT 'none',
    error_detail     text,
    response_bytes   int,
    region           text        NOT NULL DEFAULT 'default',
    detail           jsonb,
    PRIMARY KEY (monitor_id, checked_at)
);

CREATE TABLE monitor_jobs (
    monitor_id       uuid PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
    next_run_at      timestamptz NOT NULL,
    scheduled_at     timestamptz,
    status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','leased')),
    lease_expires_at timestamptz,
    attempts         int  NOT NULL DEFAULT 0
);
CREATE INDEX idx_monitor_jobs_due ON monitor_jobs (next_run_at) WHERE status = 'pending';

CREATE TYPE notification_kind  AS ENUM ('down','recovered','renotify');
CREATE TYPE delivery_status    AS ENUM ('pending','sent','failed','suppressed');
```

Note: `monitoring_checks` deliberately has no `id` PK — the composite `(monitor_id, checked_at)`
key matches every query pattern and keeps index write amplification low.

## 5. Tenancy enforcement in the schema

`monitors.org_id` is denormalized on purpose: every read path filters `WHERE m.project_id = $1 AND
m.org_id = $2`, and repository functions take a `(orgID, projectID)` scope struct. There is no
query in the API codebase that selects monitors without an org filter; a repository-level test
asserts this on every monitor query function.
