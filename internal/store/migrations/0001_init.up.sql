-- 0001: full initial schema for website monitoring.
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         citext NOT NULL UNIQUE,
    password_hash text   NOT NULL,
    full_name     text   NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE organizations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text   NOT NULL,
    slug       citext NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     uuid   NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id    uuid   NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       text   NOT NULL CHECK (role IN ('owner','admin','editor','viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, user_id)
);
CREATE INDEX idx_memberships_user ON memberships (user_id);

CREATE TABLE projects (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     uuid   NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name       text   NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, name)
);

CREATE TABLE monitors (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            uuid   NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id        uuid   NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name              text   NOT NULL,
    monitor_type      text   NOT NULL DEFAULT 'http' CHECK (monitor_type IN ('http','keyword','ssl','dns','redirect')),
    url               text   NOT NULL,
    request_config    jsonb  NOT NULL DEFAULT '{}'::jsonb,
    check_config      jsonb  NOT NULL DEFAULT '{}'::jsonb,
    interval_seconds  int    NOT NULL DEFAULT 60 CHECK (interval_seconds BETWEEN 30 AND 3600),
    timeout_ms        int    NOT NULL DEFAULT 10000 CHECK (timeout_ms BETWEEN 1000 AND 30000),
    failure_threshold int    NOT NULL DEFAULT 3 CHECK (failure_threshold >= 1),
    state             text   NOT NULL DEFAULT 'paused' CHECK (state IN ('up','down','paused')),
    deleted           boolean NOT NULL DEFAULT false,
    created_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_monitors_listing ON monitors (project_id, state) WHERE deleted = false;
CREATE INDEX idx_monitors_org ON monitors (org_id, project_id) WHERE deleted = false;

-- Durable scheduling queue: one pending-or-leased job per monitor.
CREATE TABLE monitor_jobs (
    monitor_id       uuid PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
    next_run_at      timestamptz NOT NULL,
    scheduled_at     timestamptz,
    status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','leased')),
    lease_expires_at timestamptz,
    attempts         int  NOT NULL DEFAULT 0
);
CREATE INDEX idx_monitor_jobs_due ON monitor_jobs (next_run_at) WHERE status = 'pending';

CREATE TABLE monitoring_checks (
    monitor_id       uuid        NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    checked_at       timestamptz NOT NULL,
    status           text        NOT NULL CHECK (status IN ('up','down','timeout')),
    http_status_code int         CHECK (http_status_code BETWEEN 100 AND 599),
    duration_ms      int         NOT NULL CHECK (duration_ms >= 0),
    dns_ms           int         CHECK (dns_ms >= 0),
    connect_ms       int         CHECK (connect_ms >= 0),
    tls_ms           int         CHECK (tls_ms >= 0),
    ttfb_ms          int         CHECK (ttfb_ms >= 0),
    error_class      text        NOT NULL DEFAULT 'none',
    error_detail     text,
    response_bytes   int         CHECK (response_bytes >= 0),
    region           text        NOT NULL DEFAULT 'default',
    detail           jsonb,
    PRIMARY KEY (monitor_id, checked_at)
);

CREATE TABLE incidents (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    monitor_id          uuid   NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    started_at          timestamptz NOT NULL,
    resolved_at         timestamptz,
    resolved_duration_s int,
    cause_class         text,
    status              text NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
    CHECK ((status = 'resolved') = (resolved_at IS NOT NULL))
);
CREATE INDEX idx_incidents_monitor ON incidents (monitor_id, started_at DESC);
CREATE INDEX idx_incidents_open ON incidents (status) WHERE status = 'open';

CREATE TABLE incident_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id uuid   NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    event_type  text   NOT NULL CHECK (event_type IN ('opened','recovered','notified','notification_failed','annotated')),
    payload     jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_incident_events ON incident_events (incident_id, created_at);

CREATE TABLE notification_channels (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid   NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    type       text   NOT NULL CHECK (type IN ('email','webhook')),
    config     jsonb  NOT NULL,
    enabled    boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_channels_project ON notification_channels (project_id);

CREATE TABLE alert_policies (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  uuid   NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        text   NOT NULL,
    trigger     text   NOT NULL CHECK (trigger IN ('down','recovered','ssl_expiry','response_time')),
    conditions  jsonb  NOT NULL DEFAULT '{}'::jsonb,
    enabled     boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_policies_project ON alert_policies (project_id);

CREATE TABLE monitor_alert_policies (
    monitor_id uuid NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    policy_id  uuid NOT NULL REFERENCES alert_policies(id) ON DELETE CASCADE,
    PRIMARY KEY (monitor_id, policy_id)
);

CREATE TABLE alert_policy_channels (
    policy_id  uuid NOT NULL REFERENCES alert_policies(id) ON DELETE CASCADE,
    channel_id uuid NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    PRIMARY KEY (policy_id, channel_id)
);

CREATE TABLE notification_deliveries (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id   uuid   NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    channel_id    uuid   NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    policy_id     uuid REFERENCES alert_policies(id) ON DELETE SET NULL,
    kind          text   NOT NULL CHECK (kind IN ('down','recovered','renotify')),
    status        text   NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed','suppressed')),
    next_retry_at timestamptz,
    attempt       int    NOT NULL DEFAULT 0,
    error         text,
    response_ms   int,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Authoritative dedup constraint: one 'down' notification per open incident
-- and channel is enforced here; Redis dedup is an optimization layer.
CREATE UNIQUE INDEX idx_deliveries_dedup ON notification_deliveries (incident_id, channel_id, kind)
    WHERE status <> 'suppressed' AND kind IN ('down','recovered');

CREATE INDEX idx_deliveries_pending ON notification_deliveries (next_retry_at) WHERE status = 'pending';

CREATE TABLE status_pages (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid   NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    slug       citext NOT NULL UNIQUE,
    headline   text   NOT NULL DEFAULT '',
    published  boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE status_page_monitors (
    status_page_id uuid NOT NULL REFERENCES status_pages(id) ON DELETE CASCADE,
    monitor_id     uuid NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    display_order  int  NOT NULL DEFAULT 0,
    PRIMARY KEY (status_page_id, monitor_id)
);

CREATE TABLE refresh_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid   NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text   NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens (user_id);

CREATE TABLE audit_logs (
    id            bigserial PRIMARY KEY,
    org_id        uuid,
    user_id       uuid,
    action        text        NOT NULL,
    resource_type text        NOT NULL,
    resource_id   uuid,
    diff          jsonb,
    ip            inet,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_org ON audit_logs (org_id, created_at DESC);
