-- 1-minute uptime aggregates. Populated by the rollup job; the API reads
-- uptime from here for 24h/7d/30d/90d windows so dashboards never aggregate
-- over tens of millions of raw rows.
CREATE TABLE monitoring_check_rollups_1m (
    monitor_id      uuid        NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    bucket_start    timestamptz NOT NULL, -- UTC-truncated to the minute
    checks_total    int         NOT NULL CHECK (checks_total > 0),
    checks_up       int         NOT NULL CHECK (checks_up >= 0 AND checks_up <= checks_total),
    duration_ms_sum bigint      NOT NULL DEFAULT 0,
    duration_ms_p50 int         NOT NULL DEFAULT 0,
    duration_ms_p90 int         NOT NULL DEFAULT 0,
    duration_ms_p95 int         NOT NULL DEFAULT 0,
    duration_ms_p99 int         NOT NULL DEFAULT 0,
    PRIMARY KEY (monitor_id, bucket_start)
);

CREATE INDEX idx_rollups_bucket ON monitoring_check_rollups_1m (bucket_start);

-- Retention scaffolding bookkeeping (partition cutover + prune jobs record runs).
CREATE TABLE maintenance_log (
    id          bigserial   PRIMARY KEY,
    job_name    text        NOT NULL,
    target      text        NOT NULL,
    finished_at timestamptz NOT NULL DEFAULT now(),
    detail      jsonb       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_maintenance_log ON maintenance_log (job_name, finished_at DESC);
