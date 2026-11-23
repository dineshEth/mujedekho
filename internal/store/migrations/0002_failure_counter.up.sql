-- 0002: consecutive-failure counter for the incident engine.
ALTER TABLE monitors ADD COLUMN consecutive_failures int NOT NULL DEFAULT 0;
