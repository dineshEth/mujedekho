// Package config parses and validates runtime configuration from environment
// variables. No credentials are ever defaulted; JWTSecret must be provided in
// non-development environments.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config carries every runtime setting. Paths from env only.
type Config struct {
	Env                   string        // development | production
	HTTPAddr              string        // listen address for the API
	DatabaseURL           string        // postgres DSN
	RedisAddr             string        // host:port, optional
	JWTSecret             string        // access token signing key
	AccessTTL             time.Duration // access token lifetime
	RefreshTTL            time.Duration // refresh token lifetime
	DNSResolvers          []string      // optional pinned resolvers for checks
	WorkerConcurrency     int           // max concurrent checks
	SchedulerTickPeriod   time.Duration // how often the scheduler scans for due jobs
	CheckRetentionDays    int           // raw retention window
	Region                string        // region label stamped on checks
	MaxMonitorsPerProject int           // plan guard
}

// Load reads configuration. It fails fast on unparsable numbers and on a
// missing JWT secret outside development.
func Load() (Config, error) {
	c := Config{
		Env:                   getenv("APP_ENV", "development"),
		HTTPAddr:              getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:           getenv("DATABASE_URL", ""),
		RedisAddr:             getenv("REDIS_ADDR", ""),
		JWTSecret:             os.Getenv("JWT_SECRET"),
		AccessTTL:             15 * time.Minute,
		RefreshTTL:            30 * 24 * time.Hour,
		WorkerConcurrency:     8,
		SchedulerTickPeriod:   time.Second,
		CheckRetentionDays:    30,
		Region:                getenv("REGION", "default"),
		MaxMonitorsPerProject: 5000,
	}
	if v, err := duration("ACCESS_TTL", getenv("ACCESS_TTL", "")); err == nil && v != 0 {
		c.AccessTTL = v
	}
	if v, err := duration("REFRESH_TTL", getenv("REFRESH_TTL", "")); err == nil && v != 0 {
		c.RefreshTTL = v
	}
	if v, err := atoi("WORKER_CONCURRENCY", os.Getenv("WORKER_CONCURRENCY"), c.WorkerConcurrency); err != nil {
		return c, err
	} else if v > 0 {
		c.WorkerConcurrency = v
	}
	if v, err := atoi("SCHEDULER_TICK_MS", os.Getenv("SCHEDULER_TICK_MS"), 1000); err != nil {
		return c, err
	} else if v > 0 {
		c.SchedulerTickPeriod = time.Duration(v) * time.Millisecond
	}
	if v, err := atoi("CHECK_RETENTION_DAYS", os.Getenv("CHECK_RETENTION_DAYS"), c.CheckRetentionDays); err != nil {
		return c, err
	} else if v > 0 {
		c.CheckRetentionDays = v
	}
	if v, err := atoi("MAX_MONITORS_PER_PROJECT", os.Getenv("MAX_MONITORS_PER_PROJECT"), c.MaxMonitorsPerProject); err != nil {
		return c, err
	} else if v > 0 {
		c.MaxMonitorsPerProject = v
	}
	if raw := os.Getenv("DNS_RESOLVERS"); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			if s = strings.TrimSpace(s); s != "" {
				c.DNSResolvers = append(c.DNSResolvers, s)
			}
		}
	}
	if c.Env != "development" && c.JWTSecret == "" {
		return c, fmt.Errorf("config: JWT_SECRET is required when APP_ENV != development")
	}
	if c.Env != "development" && c.JWTSecret != "" && len(c.JWTSecret) < 32 {
		return c, fmt.Errorf("config: JWT_SECRET must be at least 32 bytes")
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func atoi(name, raw string, def int) (int, error) {
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer, got %q", name, raw)
	}
	return v, nil
}

func duration(name, raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be a duration, got %q", name, raw)
	}
	return v, nil
}
