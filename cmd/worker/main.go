// cmd/worker runs the scheduler + worker pool: it claims due monitors from the
// durable Postgres job queue, executes safe checks, and hands results to the
// incident engine. Multiple worker replicas are safe: claims use
// FOR UPDATE SKIP LOCKED and leases.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"site24x7/internal/config"
	"site24x7/internal/incident"
	"site24x7/internal/observability"
	"site24x7/internal/safety"
	"site24x7/internal/scheduler"
	"site24x7/internal/store"
	"site24x7/internal/uptime"
	"site24x7/internal/worker"
)

func main() {
	config.LoadDotEnv()
	if err := run(); err != nil {
		observability.NewLogger("error").Error("worker: fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := observability.NewLogger(level(cfg.Env))

	if cfg.DatabaseURL == "" {
		return errors.New("worker: DATABASE_URL is required")
	}
	if cfg.Region != "" {
		worker.RegionLabel = cfg.Region
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	start := time.Now()
	all, err := store.NewAll(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer all.Close()
	log.Info("worker: database ready", "elapsed_ms", time.Since(start).Milliseconds())

	validator := safety.NewValidator(cfg.DNSResolvers, 5*time.Second)

	// Scheduler and workers are two logical components of one process here;
	// they can be split into separate binaries without code changes because
	// all coordination lives in Postgres.
	pool := worker.NewPool(
		cfg.WorkerConcurrency,
		cfg.WorkerConcurrency*4, // queue depth: bounded backlog with backpressure
		all.Jobs,
		worker.NewHTTPProbe(validator),
		incident.NewEngine(all.Checks, all.Monitors, all.Incidents, log),
		worker.DefaultBackoff,
		log,
	)
	pool.Start(context.WithoutCancel(ctx))

	sched := scheduler.New(all.Jobs, pool, cfg.WorkerConcurrency*2, leaseSeconds, log)
	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		sched.Run(ctx, cfg.SchedulerTickPeriod)
	}()

	// Rollups + retention: keeps uptime queries cheap and enforces
	// CHECK_RETENTION_DAYS on raw check history.
	maintDone := make(chan struct{})
	go func() {
		defer close(maintDone)
		defer observability.CatchPanic(log)
		uptime.New(all.Pool).RunMaintenance(ctx,
			time.Duration(cfg.CheckRetentionDays)*24*time.Hour, 30*time.Second, log)
	}()

	log.Info("worker: running",
		"concurrency", cfg.WorkerConcurrency,
		"tick", cfg.SchedulerTickPeriod.String(),
		"region", cfg.Region,
	)

	<-ctx.Done()
	log.Info("worker: shutdown requested")

	// Order matters: stop claiming first, then drain in-flight checks, then
	// close the pool. Leased-but-unfinished jobs are recovered by lease expiry.
	// stop() also cancels ctx, which makes the scheduler return.

	select {
	case <-schedDone:
	case <-time.After(6 * time.Second):
		log.Warn("worker: scheduler stop timed out")
	}
	pool.Stop()
	<-maintDone
	log.Info("worker: stopped cleanly")
	return nil
}

func level(env string) string {
	if env == "development" {
		return "debug"
	}
	return "info"
}

const leaseSeconds = 45 // >= 3x the network timeout budget of a single check
