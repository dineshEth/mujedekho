package incident_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"site24x7/internal/domain"
	"site24x7/internal/incident"
	"site24x7/internal/store"
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

type recordingNotifier struct {
	mu     sync.Mutex
	events []recEvent
}

type recEvent struct {
	incidentID uuid.UUID
	kind       domain.IncidentEventType
}

func (r *recordingNotifier) NotifyIncident(_ context.Context, inc *domain.Incident, _ *domain.Monitor, kind domain.IncidentEventType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recEvent{inc.ID, kind})
}

func (r *recordingNotifier) get() []recEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recEvent(nil), r.events...)
}

func fixtureMonitor(t *testing.T, all *store.All, tag string) (*store.All, *domain.Monitor, store.Scope) {
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
	scope := store.Scope{OrgID: org.ID, ProjectID: proj.ID}
	mon := &domain.Monitor{
		OrgID: org.ID, ProjectID: proj.ID, Name: "mon-" + tag,
		Type: domain.MonitorTypeHTTP, URL: "https://example.com/",
		IntervalSeconds: 60, TimeoutMs: 5000, FailureThreshold: 3,
		State: domain.MonitorStateUp, CreatedBy: user.ID,
	}
	if err := all.Monitors.Create(ctx, scope, mon); err != nil {
		t.Fatalf("monitor fixture: %v", err)
	}
	return all, mon, scope
}

func downResult(monID uuid.UUID) *domain.CheckResult {
	return &domain.CheckResult{
		MonitorID:  monID,
		CheckedAt:  time.Now().UTC(),
		Status:     domain.CheckStatusDown,
		ErrorClass: domain.CheckErrorHTTP,
		DurationMs: 123,
	}
}

func upResult(monID uuid.UUID) *domain.CheckResult {
	return &domain.CheckResult{
		MonitorID:      monID,
		CheckedAt:      time.Now().UTC(),
		Status:         domain.CheckStatusUp,
		HTTPStatusCode: 200,
		DurationMs:     42,
	}
}

// TestIncidentLifecycle: below-threshold failures open nothing; the threshold
// run opens exactly one incident; recovery resolves it; a relapse opens a new
// one.
func TestIncidentLifecycle(t *testing.T) {
	all, mon, _ := fixtureMonitor(t, testDB(t), "inc")
	ctx := context.Background()
	eng := incident.NewEngine(all.Checks, all.Monitors, all.Incidents, nil)
	notifier := &recordingNotifier{}
	eng.Notifier = notifier

	now := time.Now().UTC().Truncate(time.Millisecond).UTC()

	for i := 1; i <= 2; i++ {
		res := downResult(mon.ID)
		res.CheckedAt = now.Add(time.Duration(i) * time.Second)
		if err := eng.Process(ctx, mon, *res, res.CheckedAt); err != nil {
			t.Fatalf("process failure %d: %v", i, err)
		}
	}
	if got, err := all.Incidents.OpenForMonitor(ctx, mon.ID); err == nil {
		t.Fatalf("incident opened below threshold: %s", got.ID)
	}

	res3 := downResult(mon.ID)
	res3.CheckedAt = now.Add(3 * time.Second)
	if err := eng.Process(ctx, mon, *res3, res3.CheckedAt); err != nil {
		t.Fatalf("process threshold failure: %v", err)
	}
	inc1, err := all.Incidents.OpenForMonitor(ctx, mon.ID)
	if err != nil {
		t.Fatalf("expected open incident at threshold: %v", err)
	}
	if inc1.Status != domain.IncidentOpen {
		t.Fatalf("incident status = %q", inc1.Status)
	}

	// 4th failure: still the same incident, no duplicate.
	res4 := downResult(mon.ID)
	res4.CheckedAt = now.Add(4 * time.Second)
	if err := eng.Process(ctx, mon, *res4, res4.CheckedAt); err != nil {
		t.Fatalf("process 4th failure: %v", err)
	}
	inc1b, err := all.Incidents.OpenForMonitor(ctx, mon.ID)
	if err != nil || inc1b.ID != inc1.ID {
		t.Fatalf("incident duplicated: %s vs %s (err=%v)", inc1b.ID, inc1.ID, err)
	}

	// Recovery.
	resUp := upResult(mon.ID)
	resUp.CheckedAt = now.Add(5 * time.Second)
	if err := eng.Process(ctx, mon, *resUp, resUp.CheckedAt); err != nil {
		t.Fatalf("process recovery: %v", err)
	}
	if _, err := all.Incidents.OpenForMonitor(ctx, mon.ID); err == nil {
		t.Fatal("incident still open after recovery")
	}
	resolved, err := all.Incidents.Get(ctx, mon.OrgID, inc1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != domain.IncidentResolved || resolved.ResolvedAt == nil {
		t.Fatalf("incident not resolved properly: %+v", resolved)
	}
	if resolved.ResolvedSeconds == nil || *resolved.ResolvedSeconds < 2 {
		t.Fatalf("resolved duration suspect: %v", resolved.ResolvedSeconds)
	}

	// Relapse: threshold run opens a new incident.
	for i := 6; i <= 8; i++ {
		res := downResult(mon.ID)
		res.CheckedAt = now.Add(time.Duration(i) * time.Second)
		if err := eng.Process(ctx, mon, *res, res.CheckedAt); err != nil {
			t.Fatalf("process relapse %d: %v", i, err)
		}
	}
	inc2, err := all.Incidents.OpenForMonitor(ctx, mon.ID)
	if err != nil {
		t.Fatalf("relapse incident missing: %v", err)
	}
	if inc2.ID == inc1.ID {
		t.Fatal("relapse reused old incident")
	}

	// Notification timeline: one opened for inc1, one recovery, one opened for inc2.
	evs := notifier.get()
	if len(evs) != 3 {
		t.Fatalf("notifier events = %d (%v), want 3", len(evs), evs)
	}
	if evs[0].kind != domain.IncidentEventOpened || evs[0].incidentID != inc1.ID {
		t.Errorf("event 0 wrong: %+v", evs[0])
	}
	if evs[1].kind != domain.IncidentEventRecovery {
		t.Errorf("event 1 wrong: %+v", evs[1])
	}
	if evs[2].kind != domain.IncidentEventOpened || evs[2].incidentID != inc2.ID {
		t.Errorf("event 2 wrong: %+v", evs[2])
	}

	// Every processed check is persisted.
	page, err := all.Checks.List(ctx, mon.ID, now, now.Add(10*time.Second), nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 8 {
		t.Fatalf("persisted %d checks, want 8", len(page.Items))
	}
}
