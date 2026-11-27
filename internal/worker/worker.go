// Package worker executes claimed checks with bounded concurrency, applies
// the retry policy, and hands results to the result processor.
package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"site24x7/internal/domain"
	"site24x7/internal/observability"
	"site24x7/internal/store"
)

// RegionLabel stamps every check; configuration sets it at startup by
// overwriting this var (single-region deployments use "default").
var RegionLabel = "default"

// Probe executes one monitor check and returns a domain result. Implemented
// by the checker adapters.
type Probe interface {
	Probe(ctx context.Context, mon *domain.Monitor) domain.CheckResult
}

// Processor consumes completed results (persistence, incident engine,
// notification triggers).
type Processor interface {
	Process(ctx context.Context, mon *domain.Monitor, result domain.CheckResult, scheduledAt time.Time) error
}

// BackoffProvider chooses the retry delay for a transient failure. Returning
// 0 disables the retry.
type BackoffProvider func(attempt int) time.Duration

// DefaultBackoff retries transient outcomes once after one second. A single
// lost packet or brief CPU stall should not page anyone.
func DefaultBackoff(attempt int) time.Duration {
	if attempt != 0 {
		return 0
	}
	return time.Second
}

// Pool is a fixed-size worker pool. Submit blocks while the queue is full,
// giving the scheduler natural backpressure; Stop drains in-flight work.
type Pool struct {
	maxConcurrency int
	jobs           chan store.Job
	jobsStore      *store.Jobs
	probe          Probe
	process        Processor
	backoff        BackoffProvider
	log            *slog.Logger
	wg             sync.WaitGroup
	closed         chan struct{}
	stopOnce       sync.Once
}

func NewPool(maxConcurrency, queueDepth int, jobsStore *store.Jobs, probe Probe, process Processor, backoff BackoffProvider, log *slog.Logger) *Pool {
	if log == nil {
		log = observability.Nop()
	}
	return &Pool{
		maxConcurrency: maxConcurrency,
		jobs:           make(chan store.Job, queueDepth),
		jobsStore:      jobsStore,
		probe:          probe,
		process:        process,
		backoff:        backoff,
		log:            log,
		closed:         make(chan struct{}),
	}
}

// Start launches the workers; Submit then becomes usable.
func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.maxConcurrency; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.loop(ctx)
		}()
	}
}

// Submit enqueues a job, blocking until space frees up. It returns false only
// after Stop was called.
func (p *Pool) Submit(job store.Job) bool {
	// Fast reject when stopped (select would otherwise happily choose a
	// buffered send over the closed channel).
	select {
	case <-p.closed:
		return false
	default:
	}
	select {
	case p.jobs <- job:
		return true
	case <-p.closed:
		return false
	}
}

// Stop signals no more submissions and waits for queued and in-flight checks
// to finish. Any job still queued when all workers retire is left leased and
// is recovered automatically by lease expiry on the next scheduler cycle.
func (p *Pool) Stop() {
	p.stopOnce.Do(func() { close(p.closed) })
	p.wg.Wait()
}

// loop is the worker goroutine.
func (p *Pool) loop(ctx context.Context) {
	defer observability.CatchPanic(p.log)
	for {
		select {
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			p.safeRun(ctx, job)
		case <-p.closed:
			p.drain(ctx)
			return
		}
	}
}

// drain consumes anything already in the queue before retiring.
func (p *Pool) drain(ctx context.Context) {
	for {
		select {
		case job := <-p.jobs:
			p.safeRun(ctx, job)
		default:
			return
		}
	}
}

func (p *Pool) safeRun(ctx context.Context, job store.Job) {
	defer observability.CatchPanic(p.log)
	p.run(ctx, job)
}

func (p *Pool) run(ctx context.Context, job store.Job) {
	mon := job.Monitor
	timeout := time.Duration(mon.TimeoutMs) * time.Millisecond

	wctx, cancel := context.WithTimeout(ctx, timeout*3)
	defer cancel()

	result := probeOnce(p.probe, wctx, mon)

	// Single retry with backoff for transient outcomes; the retry replaces
	// the failed result and never violates the no-overlap rule, because the
	// monitor's job stays leased until after processing completes.
	if retryable(result) && p.backoff != nil {
		if d := p.backoff(0); d > 0 {
			select {
			case <-time.After(d):
				result = probeOnce(p.probe, wctx, mon)
			case <-wctx.Done():
			}
		}
	}

	pctx, pcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer pcancel()
	if err := p.process.Process(pctx, mon, result, job.ScheduledAt); err != nil {
		p.log.Error("worker: process result", "err", err, "monitor_id", mon.ID)
	}

	if mon.State == domain.MonitorStatePaused {
		// Paused while we ran: leave next_run_at as pause left it.
		return
	}
	ok, err := p.jobsStore.Complete(pctx, mon.ID, job.ScheduledAt, mon.IntervalSeconds)
	if err != nil {
		p.log.Error("worker: reschedule job", "err", err, "monitor_id", mon.ID)
	} else if !ok {
		// Lease expired and another scheduler re-claimed: our completion is
		// intentionally ignored. The log line makes the race observable.
		p.log.Warn("worker: stale job completion ignored (lease reassigned)",
			"monitor_id", mon.ID, "scheduled_at", job.ScheduledAt)
	}
}

func probeOnce(probe Probe, ctx context.Context, mon *domain.Monitor) domain.CheckResult {
	result := probe.Probe(ctx, mon)
	result.MonitorID = mon.ID
	result.CheckedAt = time.Now().UTC()
	if result.Region == "" {
		result.Region = RegionLabel
	}
	return result
}

func retryable(r domain.CheckResult) bool {
	switch r.ErrorClass {
	case domain.CheckErrorTimeout,
		domain.CheckErrorDNS,
		domain.CheckErrorConnect:
		return r.Status == domain.CheckStatusDown || r.Status == domain.CheckStatusTimeout
	default:
		return false
	}
}
