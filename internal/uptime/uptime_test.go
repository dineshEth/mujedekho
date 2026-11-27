package uptime_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"site24x7/internal/domain"
	"site24x7/internal/store"
	"site24x7/internal/uptime"
)

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

// TestRollupAndSummary: raw checks roll up into 1-minute buckets; uptime
// follows the up/down semantics; rollup is idempotent.
func TestRollupAndSummary(t *testing.T) {
	all := testDB(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]

	user := &domain.User{Email: "up-" + suffix + "@example.com", PasswordHash: "x", FullName: "F"}
	org := &domain.Organization{Name: "Org up", Slug: "up-" + suffix}
	proj := &domain.Project{Name: "p"}
	if err := all.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := all.Organizations.Create(ctx, org); err != nil {
		t.Fatal(err)
	}
	proj.OrgID = org.ID // org.ID only exists after Create
	if err := all.Projects.Create(ctx, proj); err != nil {
		t.Fatal(err)
	}
	scope := store.Scope{OrgID: org.ID, ProjectID: proj.ID}
	mon := &domain.Monitor{
		OrgID: org.ID, ProjectID: proj.ID, Name: "mon", Type: domain.MonitorTypeHTTP,
		URL: "https://example.com/", IntervalSeconds: 60, TimeoutMs: 5000,
		FailureThreshold: 3, State: domain.MonitorStateUp, CreatedBy: user.ID,
	}
	if err := all.Monitors.Create(ctx, scope, mon); err != nil {
		t.Fatal(err)
	}

	base := time.Now().UTC().Truncate(time.Minute).Add(-10 * time.Minute)
	// 8 up checks (4 minutes), 2 down checks (1 minute).
	insert := func(at time.Time, up bool, ms int64) {
		r := &domain.CheckResult{
			MonitorID: mon.ID, CheckedAt: at, DurationMs: ms, Region: "test",
		}
		if up {
			r.Status = domain.CheckStatusUp
			r.HTTPStatusCode = 200
		} else {
			r.Status = domain.CheckStatusDown
			r.ErrorClass = domain.CheckErrorHTTP
			r.HTTPStatusCode = 500
		}
		if err := all.Checks.Insert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 8; i++ {
		insert(base.Add(time.Duration(i)*30*time.Second), true, int64(100+i))
	}
	for i := 8; i < 10; i++ {
		insert(base.Add(time.Duration(i)*30*time.Second), false, int64(400+i))
	}

	up := uptime.New(all.Pool)
	window := [2]time.Time{base, base.Add(5 * time.Minute)}

	n, err := up.RollupMins(ctx, window[0], window[1])
	if err != nil {
		t.Fatal(err)
	}
	if n < 3 {
		t.Fatalf("rolled up %d buckets, want >= 3", n)
	}

	// Idempotent: a second pass must not change totals.
	if _, err := up.RollupMins(ctx, window[0], window[1]); err != nil {
		t.Fatal(err)
	}

	sum, err := up.Summary(ctx, mon.ID.String(), window[0], window[1])
	if err != nil {
		t.Fatal(err)
	}
	if sum.NoData {
		t.Fatal("summary reports no_data for populated window")
	}
	if sum.ChecksTotal != 10 {
		t.Fatalf("checks_total = %d, want 10", sum.ChecksTotal)
	}
	if sum.ChecksUp != 8 {
		t.Fatalf("checks_up = %d, want 8", sum.ChecksUp)
	}
	// 8 up checks 30s apart span 4 minutes; 2 down checks fill a 5th.
	// 5 observed minutes: 4 up, 1 down -> 80%.
	if sum.UptimePct == nil || *sum.UptimePct != 80.0 {
		t.Fatalf("uptime = %v, want 80.0", deref(sum.UptimePct))
	}
	if sum.DowntimeSec != 60 {
		t.Fatalf("downtime = %ds, want 60", sum.DowntimeSec)
	}

	// Empty window is no_data, never 100%.
	empty, err := up.Summary(ctx, mon.ID.String(), base.Add(-48*time.Hour), base.Add(-47*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !empty.NoData || empty.UptimePct != nil {
		t.Fatalf("empty window should be no_data with nil uptime: %+v", empty)
	}

	// Series aggregates into chart points.
	pts, err := up.Series(ctx, mon.ID.String(), window[0], window[1], 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) == 0 {
		t.Fatal("series empty")
	}
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
