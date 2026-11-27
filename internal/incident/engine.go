// Package incident implements the incident engine: it turns raw check results
// into persisted check rows, maintains the consecutive-failure counter, and
// opens/resolves incidents at the configured confirmation thresholds.
//
// The Postgres-side row locking in store.Incidents is authoritative against
// duplicate incidents; this engine is idempotent anyway (workers may retry,
// manual checks may race with scheduled ones).
package incident

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"site24x7/internal/domain"
	"site24x7/internal/observability"
	"site24x7/internal/store"
)

// Notifier receives incident lifecycle notifications. Phase 6 (notifications)
// provides the real dispatcher; until then a nil Notifier is valid and skipped.
type Notifier interface {
	NotifyIncident(ctx context.Context, inc *domain.Incident, mon *domain.Monitor, kind domain.IncidentEventType)
}

// Engine implements the worker.Processor contract.
type Engine struct {
	Checks    *store.Checks
	Monitors  *store.Monitors
	Incidents *store.Incidents
	Log       *slog.Logger
	Notifier  Notifier // optional
	now       func() time.Time
}

func NewEngine(checks *store.Checks, monitors *store.Monitors, incidents *store.Incidents, log *slog.Logger) *Engine {
	if log == nil {
		log = observability.NewLogger("info")
	}
	return &Engine{
		Checks:    checks,
		Monitors:  monitors,
		Incidents: incidents,
		Log:       log,
		now:       time.Now,
	}
}

// Process is called once per (monitor, scheduled run) after the probe executed.
// Persisting the check first, then evaluating state, keeps the incident
// timeline reconstructable even if the process crashes mid-way: the check row
// is the source of truth and incident decisions can be replayed from it.
func (e *Engine) Process(ctx context.Context, mon *domain.Monitor, res domain.CheckResult, scheduledAt time.Time) error {
	if mon == nil {
		return errors.New("incident: nil monitor or result")
	}
	log := e.Log.With("monitor_id", mon.ID, "check_status", res.Status)

	check := normalizeCheck(mon, &res, e.now)
	if err := e.Checks.Insert(ctx, check); err != nil {
		return fmt.Errorf("incident: persist check: %w", err)
	}

	up := res.Status == domain.CheckStatusUp
	consec, err := e.Monitors.RecordOutcome(ctx, mon.ID, up, mon.FailureThreshold)
	if err != nil {
		// The check row exists; state bookkeeping lagging is worse than
		// losing the result entirely. Log loudly and continue.
		log.Error("record outcome failed", "error", err)
		return fmt.Errorf("incident: record outcome: %w", err)
	}
	mon.ConsecutiveFailures = consec

	if up {
		return e.maybeResolve(ctx, mon, check.CheckedAt, log)
	}
	return e.maybeOpen(ctx, mon, &res, consec, log)
}

// maybeOpen creates an incident once the failure confirmation threshold is
// exactly reached. Later consecutive failures keep the existing open incident
// (Postgres row lock enforces exactly-one-open).
func (e *Engine) maybeOpen(ctx context.Context, mon *domain.Monitor, res *domain.CheckResult, consec int, log *slog.Logger) error {
	if consec != mon.FailureThreshold {
		return nil
	}
	startedAt := res.CheckedAt
	if startedAt.IsZero() {
		startedAt = e.now().UTC()
	}
	incID, created, err := e.Incidents.Open(ctx, mon.ID, startedAt, string(res.ErrorClass))
	if err != nil {
		return fmt.Errorf("incident: open: %w", err)
	}
	if !created {
		return nil
	}
	log.Info("incident opened", "incident_id", incID)
	inc, err := e.Incidents.Get(ctx, mon.OrgID, incID)
	if err != nil {
		// Never lose the notification on a read glitch: synthesize the
		// incident from what we know.
		inc = &domain.Incident{ID: incID, MonitorID: mon.ID, Status: domain.IncidentOpen, StartedAt: startedAt, CauseClass: string(res.ErrorClass)}
	}
	e.notify(ctx, inc, mon, domain.IncidentEventOpened)
	return nil
}

// maybeResolve closes the open incident on the first successful check. If a
// flapping failure follows, the next threshold run opens a fresh incident.
func (e *Engine) maybeResolve(ctx context.Context, mon *domain.Monitor, recoveredAt time.Time, log *slog.Logger) error {
	if mon.ConsecutiveFailures != 0 {
		return nil
	}
	inc, err := e.Incidents.OpenForMonitor(ctx, mon.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil // already resolved (or never opened)
	}
	if err != nil {
		return fmt.Errorf("incident: lookup open incident: %w", err)
	}
	// The recovering check's timestamp is the authoritative recovery time;
	// wall-clock now() would skew durations for delayed or replayed results.
	ok, err := e.Incidents.Resolve(ctx, inc.ID, recoveredAt)
	if err != nil {
		return fmt.Errorf("incident: resolve: %w", err)
	}
	if !ok {
		return nil
	}
	log.Info("incident resolved", "incident_id", inc.ID)
	// Re-fetch so the notifier sees resolved_at / duration.
	if fresh, err := e.Incidents.Get(ctx, mon.OrgID, inc.ID); err == nil {
		inc = fresh
	}
	e.notify(ctx, inc, mon, domain.IncidentEventRecovery)
	return nil
}

func (e *Engine) notify(ctx context.Context, inc *domain.Incident, mon *domain.Monitor, kind domain.IncidentEventType) {
	if e.Notifier == nil {
		return
	}
	// Fresh, bounded context: a slow notification must not delay other checks
	// and must not inherit an already-cancelling check context.
	nctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	defer observability.CatchPanic(e.Log)
	e.Notifier.NotifyIncident(nctx, inc, mon, kind)
}

func normalizeCheck(mon *domain.Monitor, res *domain.CheckResult, now func() time.Time) *domain.CheckResult {
	c := *res
	c.MonitorID = mon.ID
	if c.CheckedAt.IsZero() {
		c.CheckedAt = now().UTC()
	}
	if c.Region == "" {
		c.Region = "default"
	}
	return &c
}
