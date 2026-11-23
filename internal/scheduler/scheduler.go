// Package scheduler runs the loop that claims due monitors from the durable
// job queue and dispatches them to the worker pool. It holds no scheduling
// state in memory: PostgreSQL is the source of truth, so restarts are
// crash-safe by construction.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"site24x7/internal/observability"
	"site24x7/internal/store"
)

// Submitter hands claimed jobs to the worker pool. Blocking Submit provides
// backpressure: when checks outpace capacity, claims stall rather than pile up
// in unbounded queues.
type Submitter interface {
	Submit(job store.Job) bool
}

// Scheduler drives DueBatch on a tick and dispatches results.
type Scheduler struct {
	jobs      *store.Jobs
	workers   Submitter
	batchSize int
	leaseSecs int
	log       *slog.Logger
}

func New(jobs *store.Jobs, workers Submitter, batchSize, leaseSecs int, log *slog.Logger) *Scheduler {
	if log == nil {
		log = observability.Nop()
	}
	return &Scheduler{jobs: jobs, workers: workers, batchSize: batchSize, leaseSecs: leaseSecs, log: log}
}

// Run claims and dispatches until ctx is cancelled. Tick cadence is owned by
// the caller.
func (s *Scheduler) Run(ctx context.Context, tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.tickOnce(ctx)
	}
}

func (s *Scheduler) tickOnce(ctx context.Context) {
	defer observability.CatchPanic(s.log)

	// Reclaim leases from dead workers first, so their jobs re-queue in this
	// cycle instead of waiting a full extra interval.
	lost, err := s.jobs.ReclaimExpired(ctx, time.Now())
	if err != nil {
		s.log.Error("scheduler: reclaim expired leases", "err", err)
	}
	for _, id := range lost {
		s.log.Warn("scheduler: reclaimed expired job lease", "monitor_id", id)
	}

	jobs, err := s.jobs.DueBatch(ctx, s.batchSize, s.leaseSecs)
	if err != nil {
		s.log.Error("scheduler: claim batch", "err", err)
		return
	}
	for _, job := range jobs {
		if !s.workers.Submit(job) {
			s.log.Warn("scheduler: worker pool shut down mid-claim", "monitor_id", job.Monitor.ID)
			return
		}
	}
}
