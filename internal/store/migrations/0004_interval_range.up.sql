-- 0004: allow 5-second check intervals and align the upper bound with the
-- API (86400s). The 0001 range (30..3600) rejected values the API accepted.
ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_interval_seconds_check;
ALTER TABLE monitors ADD CONSTRAINT monitors_interval_seconds_check
    CHECK (interval_seconds BETWEEN 5 AND 86400);
