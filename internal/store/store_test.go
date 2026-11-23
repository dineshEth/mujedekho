package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"site24x7/internal/domain"
	"site24x7/internal/store"
)

// testDB skips on environments without a database. `make test-integration`
// starts docker compose services and exports TEST_DATABASE_URL.
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

// Trio is a deterministic user/org/project fixture per test.
type Trio struct {
	User   *domain.User
	Org    *domain.Organization
	Proj   *domain.Project
	Scope  store.Scope
	OrgB   *domain.Organization
	ProjB  *domain.Project
	ScopeB store.Scope
}

func trio(t *testing.T, all *store.All, tag string) Trio {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()[:8]

	user := &domain.User{Email: tag + "-" + suffix + "@example.com", PasswordHash: "test-hash", FullName: "Fixture"}
	if err := all.Users.Create(ctx, user); err != nil {
		t.Fatalf("user fixture: %v", err)
	}
	orgA := &domain.Organization{Name: "Org " + tag, Slug: tag + "-" + suffix}
	if err := all.Organizations.Create(ctx, orgA); err != nil {
		t.Fatalf("org fixture: %v", err)
	}
	projA := &domain.Project{OrgID: orgA.ID, Name: "proj-" + tag}
	if err := all.Projects.Create(ctx, projA); err != nil {
		t.Fatalf("project fixture: %v", err)
	}
	if err := all.Organizations.AddMembership(ctx, &domain.Membership{OrgID: orgA.ID, UserID: user.ID, Role: domain.RoleOwner}); err != nil {
		t.Fatalf("membership fixture: %v", err)
	}

	orgB := &domain.Organization{Name: "Org " + tag + "-b", Slug: tag + "-b-" + suffix}
	if err := all.Organizations.Create(ctx, orgB); err != nil {
		t.Fatalf("org b fixture: %v", err)
	}
	projB := &domain.Project{OrgID: orgB.ID, Name: "proj-" + tag + "-b"}
	if err := all.Projects.Create(ctx, projB); err != nil {
		t.Fatalf("project b fixture: %v", err)
	}

	return Trio{
		User: user, Org: orgA, Proj: projA,
		Scope: store.Scope{OrgID: orgA.ID, ProjectID: projA.ID},
		OrgB:  orgB, ProjB: projB,
		ScopeB: store.Scope{OrgID: orgB.ID, ProjectID: projB.ID},
	}
}

// TestTenantIsolation is the load-bearing security test: no scoped lookup may
// cross the organization or project boundary.
func TestTenantIsolation(t *testing.T) {
	all := testDB(t)
	ctx := context.Background()
	f := trio(t, all, "iso-a")

	mon := &domain.Monitor{
		OrgID: f.Org.ID, ProjectID: f.Proj.ID, Name: "A-site",
		Type: domain.MonitorTypeHTTP, URL: "https://a.example.com",
		IntervalSeconds: 60, TimeoutMs: 10000, FailureThreshold: 3,
		State: domain.MonitorStatePaused, CreatedBy: f.User.ID,
	}
	if err := all.Monitors.Create(ctx, f.Scope, mon); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Cross-scope reads must not find A's monitor.
	if _, err := all.Monitors.Get(ctx, f.OrgB.ID, mon.ID); err != store.ErrNotFound {
		t.Fatalf("get with wrong org is not ErrNotFound: %v", err)
	}
	mons, _, err := all.Monitors.List(ctx, f.ScopeB, store.ListFilter{})
	if err != nil {
		t.Fatalf("list b: %v", err)
	}
	if len(mons) != 0 {
		t.Fatalf("project B listing leaked %d monitors", len(mons))
	}

	// Updates must not touch other tenants.
	if err := all.Monitors.SetState(ctx, f.ScopeB, mon.ID, domain.MonitorStateUp); err != store.ErrNotFound {
		t.Fatalf("cross-tenant SetState must be ErrNotFound, got %v", err)
	}
	got, err := all.Monitors.Get(ctx, f.Org.ID, mon.ID)
	if err != nil {
		t.Fatalf("get back: %v", err)
	}
	if got.State != domain.MonitorStatePaused {
		t.Fatalf("cross-tenant SetState leaked: state=%s", got.State)
	}

	// Unscoped creation with inconsistent identifiers must be refused.
	bad := *mon
	bad.OrgID = f.OrgB.ID
	if err := all.Monitors.Create(ctx, f.Scope, &bad); err == nil {
		t.Fatal("mismatched org_id in monitor payload must be refused")
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	all := testDB(t)
	if err := all.Remigrate(context.Background()); err != nil {
		t.Fatalf("remigrate: %v", err)
	}
}

func TestMonitorCRUDAndChecks(t *testing.T) {
	all := testDB(t)
	ctx := context.Background()
	f := trio(t, all, "crud")

	mon := &domain.Monitor{
		OrgID: f.Org.ID, ProjectID: f.Proj.ID, Name: "prod",
		Type: domain.MonitorTypeHTTP, URL: "https://prod.example.com",
		IntervalSeconds: 60, TimeoutMs: 10000, FailureThreshold: 3,
		State: domain.MonitorStatePaused, CreatedBy: f.User.ID,
	}
	if err := all.Monitors.Create(ctx, f.Scope, mon); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := all.Monitors.Get(ctx, f.Org.ID, mon.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.URL != mon.URL || got.Type != domain.MonitorTypeHTTP {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}

	// Duplicate insert for same (monitor, checked_at) is a no-op.
	now := time.Now().UTC().Truncate(time.Millisecond)
	for i := 0; i < 2; i++ {
		c := domain.CheckResult{
			MonitorID: mon.ID, CheckedAt: now, Status: domain.CheckStatusUp,
			DurationMs: 120, ErrorClass: domain.CheckErrorNone, Region: "default",
		}
		if err := all.Checks.Insert(ctx, &c); err != nil {
			t.Fatalf("insert check: %v", err)
		}
	}
	page, err := all.Checks.List(ctx, mon.ID, now.Add(-time.Hour), now.Add(time.Hour), nil, 10)
	if err != nil {
		t.Fatalf("list checks: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("idempotency window failed: %d rows for duplicate insert", len(page.Items))
	}

	// Pause/unpause drives the schedule rows.
	if err := all.Monitors.SetState(ctx, f.Scope, mon.ID, domain.MonitorStatePaused); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if err := all.Monitors.SetState(ctx, f.Scope, mon.ID, domain.MonitorStateUp); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := all.Monitors.SoftDelete(ctx, f.Scope, mon.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := all.Monitors.Get(ctx, f.Org.ID, mon.ID); err != store.ErrNotFound {
		t.Fatalf("get after soft delete must be ErrNotFound, got %v", err)
	}
}

// TestUniqueConstraints exercises store-level conflict mapping.
func TestUniqueConstraints(t *testing.T) {
	all := testDB(t)
	ctx := context.Background()
	f := trio(t, all, "unique")

	dup := &domain.User{Email: f.User.Email, PasswordHash: "x", FullName: "Dup"}
	if err := all.Users.Create(ctx, dup); err == nil {
		t.Fatal("duplicate email must fail")
	}
	dupOrg := &domain.Organization{Name: "dup", Slug: f.Org.Slug}
	if err := all.Organizations.Create(ctx, dupOrg); err == nil {
		t.Fatal("duplicate slug must fail")
	}
	p2 := &domain.Project{OrgID: f.Org.ID, Name: f.Proj.Name}
	if err := all.Projects.Create(ctx, p2); err == nil {
		t.Fatal("duplicate project name in org must fail")
	}
}
