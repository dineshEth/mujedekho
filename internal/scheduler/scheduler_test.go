package scheduler_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"site24x7/internal/domain"
	"site24x7/internal/scheduler"
	"site24x7/internal/store"

	"github.com/google/uuid"
)

// testDB skips on environments without a database.
func testDB(t *testing.T) *store.All {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	all, err := store.NewAll(ctx, dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(all.Close)
	return all
}

// collectorSubmitter records submitted jobs and can block to simulate a full
// worker pool (backpressure).
type collectorSubmitter struct {
	mu   sync.Mutex
	jobs []store.Job
}

func (c *collectorSubmitter) Submit(job store.Job) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jobs = append(c.jobs, job)
	return true
}

func (c *collectorSubmitter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.jobs)
}

func (c *collectorSubmitter) monitorIDs() map[string]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make(map[string]bool, len(c.jobs))
	for _, j := range c.jobs {
		ids[j.Monitor.ID.String()] = true
	}
	return ids
}

// fixture builds a fresh user/org/project/scope and returns them.
func fixture(t *testing.T, all *store.All, tag string) (store.Scope, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()[:8]

	user := &domain.User{Email: tag + "-" + suffix + "@example.com", PasswordHash: "x", FullName: "Fixture"}
	if err := all.Users.Create(ctx, user); err != nil {
		t.Fatalf("user fixture: %v", err)
	}
	org := &domain.Organization{Name: "Org " + tag, Slug: tag + "-" + suffix}
	if err := all.Organizations.Create(ctx, org); err != nil {
		t.Fatalf("org fixture: %v", err)
	}
	proj := &domain.Project{OrgID: org.ID, Name: "proj-" + tag}
	if err := all.Projects.Create(ctx, proj); err != nil {
		t.Fatalf("project fixture: %v", err)
	}
	return store.Scope{OrgID: org.ID, ProjectID: proj.ID}, user.ID
}

func newMonitor(t *testing.T, all *store.All, scope store.Scope, createdBy uuid.UUID, name string, interval int) *domain.Monitor {
	t.Helper()
	mon := &domain.Monitor{
		OrgID:            scope.OrgID,
		ProjectID:        scope.ProjectID,
		Name:             name,
		Type:             domain.MonitorTypeHTTP,
		URL:              "https://example.com/",
		IntervalSeconds:  interval,
		TimeoutMs:        5000,
		FailureThreshold: 2,
		State:            domain.MonitorStateUp,
		CreatedBy:        createdBy,
	}
	if err := all.Monitors.Create(context.Background(), scope, mon); err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	return mon
}

// TestSchedulerDispatchesDueMonitors: one tick claims due monitors exactly
// once and dispatches them to the submitter.
func TestSchedulerDispatchesDueMonitors(t *testing.T) {
	all := testDB(t)
	scope, userID := fixture(t, all, "sched")

	due1 := newMonitor(t, all, scope, userID, "due-a", 60)
	due2 := newMonitor(t, all, scope, userID, "due-b", 60)
	future := newMonitor(t, all, scope, userID, "future", 3600)

	// Make 'future' not due yet.
	if _, err := all.Pool.Exec(context.Background(),
		`UPDATE monitor_jobs SET next_run_at = now() + interval '30 minutes' WHERE monitor_id = $1`,
		future.ID); err != nil {
		t.Fatal(err)
	}

	sub := &collectorSubmitter{}
	s := scheduler.New(all.Jobs, sub, 10, 60, nil)

	ctx2, cancel := context.WithCancel(context.Background())
	go s.Run(ctx2, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	cancel()

	ids := sub.monitorIDs()
	if !ids[due1.ID.String()] || !ids[due2.ID.String()] {
		t.Fatalf("due monitors not dispatched: got %v", ids)
	}
	if ids[future.ID.String()] {
		t.Fatal("future monitor was dispatched")
	}
	// The test database is shared with other suites, so only monitors from
	// this test's own project are meaningful here.
	mine := 0
	for id := range ids {
		var proj string
		if err := all.Pool.QueryRow(context.Background(),
			`SELECT project_id::text FROM monitors WHERE id = $1`, id).Scan(&proj); err != nil {
			t.Fatal(err)
		}
		if proj == scope.ProjectID.String() {
			mine++
		}
	}
	if mine != 2 {
		t.Fatalf("expected exactly 2 monitors from this project dispatched, got %d (%v)", mine, ids)
	}

	// Claimed jobs are leased, not pending: an immediate second scheduler must
	// not double-claim within the lease window.
	sub2 := &collectorSubmitter{}
	s2 := scheduler.New(all.Jobs, sub2, 10, 60, nil)
	ctx3, cancel3 := context.WithCancel(context.Background())
	go s2.Run(ctx3, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	cancel3()
	for _, j := range sub2.jobs {
		if j.Monitor.ID == due1.ID || j.Monitor.ID == due2.ID {
			t.Fatalf("monitor %s double-claimed while leased", j.Monitor.ID)
		}
	}
}

// TestSchedulerReclaimsExpiredLeases: a job leased by a "dead" worker (short
// lease) is recovered and re-dispatched after the lease lapses.
func TestSchedulerReclaimsExpiredLeases(t *testing.T) {
	all := testDB(t)
	scope, userID := fixture(t, all, "lease")
	ctx := context.Background()
	mon := newMonitor(t, all, scope, userID, "lease-me", 60)

	// Simulate a crashed worker: claim, then never Complete, with a 1-second lease.
	lease := 1
	batch, err := all.Jobs.DueBatch(ctx, 10, lease)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 1 || batch[0].Monitor.ID != mon.ID {
		t.Fatalf("expected to claim exactly the fixture monitor, got %d jobs", len(batch))
	}

	// Before the lease expires: no reclaim, no re-claim.
	if lost, err := all.Jobs.ReclaimExpired(ctx, time.Now()); err != nil || len(lost) != 0 {
		t.Fatalf("premature reclaim: lost=%v err=%v", lost, err)
	}

	time.Sleep(1100 * time.Millisecond)

	lost, err := all.Jobs.ReclaimExpired(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(lost) != 1 || lost[0] != mon.ID {
		t.Fatalf("expected lease reclaim of %s, got %v", mon.ID, lost)
	}

	// After reclaim the job is pending again and claimable.
	batch2, err := all.Jobs.DueBatch(ctx, 10, 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch2) != 1 || batch2[0].Monitor.ID != mon.ID {
		t.Fatalf("expected re-claim after reclaim, got %d jobs", len(batch2))
	}
}

// TestCompleteClampsMissedSchedules: Complete pins on scheduled_at; a stale
// completion (lease reassigned meanwhile) is a no-op.
func TestCompletePinsScheduledAt(t *testing.T) {
	all := testDB(t)
	scope, userID := fixture(t, all, "clamp")
	ctx := context.Background()

	mon := newMonitor(t, all, scope, userID, "clamp-me", 60)

	batch, err := all.Jobs.DueBatch(ctx, 10, 60)
	if err != nil || len(batch) != 1 {
		t.Fatalf("claim: %d jobs err=%v", len(batch), err)
	}
	job := batch[0]

	// Simulate the lease expiring and another scheduler re-claiming the same
	// monitor with a NEW scheduled_at: rewind next_run_at and re-claim.
	if _, err := all.Pool.Exec(ctx,
		`UPDATE monitor_jobs SET status='pending', lease_expires_at = now() - interval '1s', attempts = 0
		 WHERE monitor_id = $1`, mon.ID); err != nil {
		t.Fatal(err)
	}
	var newScheduled time.Time
	if err := all.Pool.QueryRow(ctx, `
		UPDATE monitor_jobs SET status='leased', lease_expires_at = now() + interval '60s',
		       scheduled_at = now(), next_run_at = now()
		WHERE monitor_id = $1 RETURNING scheduled_at`, mon.ID).Scan(&newScheduled); err != nil {
		t.Fatal(err)
	}

	// The old worker now completes with its (outdated) scheduled_at. This must
	// be ignored — otherwise it would clobber the newer claim's schedule.
	ok, err := all.Jobs.Complete(ctx, mon.ID, job.ScheduledAt, mon.IntervalSeconds)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("stale completion reported ok")
	}

	// The current claim still completes cleanly and schedules forward.
	ok2, err := all.Jobs.Complete(ctx, mon.ID, newScheduled, mon.IntervalSeconds)
	if err != nil || !ok2 {
		t.Fatalf("current claim completion: ok=%v err=%v", ok2, err)
	}

	var nextRun time.Time
	var status string
	if err := all.Pool.QueryRow(ctx,
		`SELECT next_run_at, status FROM monitor_jobs WHERE monitor_id = $1`, mon.ID).Scan(&nextRun, &status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("job status = %q, want pending", status)
	}
	if !nextRun.After(newScheduled) {
		t.Fatalf("next_run %v not after scheduled %v (clamp failed)", nextRun, newScheduled)
	}
	if diff := time.Until(nextRun); diff < 55*time.Second || diff > 65*time.Second {
		t.Fatalf("next_run %v not ~60s ahead (got %v)", nextRun, diff)
	}
}
