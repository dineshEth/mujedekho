UPDATE monitors SET interval_seconds = GREATEST(30, LEAST(3600, interval_seconds));
ALTER TABLE monitors DROP CONSTRAINT IF EXISTS monitors_interval_seconds_check;
ALTER TABLE monitors ADD CONSTRAINT monitors_interval_seconds_check
    CHECK (interval_seconds BETWEEN 30 AND 3600);
