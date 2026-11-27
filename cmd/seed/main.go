// Command seed creates demo tenant data for local development. It is
// idempotent per run (fresh UUIDs each run) and only for APP_ENV=development.
package main

import (
	"context"
	"fmt"
	"os"

	"site24x7/internal/auth"
	"site24x7/internal/config"
	"site24x7/internal/domain"
	"site24x7/internal/store"
)

func main() {
	config.LoadDotEnv()
	if os.Getenv("APP_ENV") == "production" {
		fmt.Fprintln(os.Stderr, "seed: refusing to run with APP_ENV=production")
		os.Exit(2)
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://site24x7:site24x7@localhost:5432/site24x7?sslmode=disable"
	}
	ctx := context.Background()
	all, err := store.NewAll(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
	defer all.Close()

	seedPassword := os.Getenv("SEED_PASSWORD")
	if seedPassword == "" {
		fmt.Fprintln(os.Stderr, "seed: SEED_PASSWORD is required")
		os.Exit(2)
	}
	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}

	user := &domain.User{Email: os.Getenv("SEED_EMAIL"), PasswordHash: hash, FullName: "Demo Owner"}
	if user.Email == "" {
		user.Email = "demo@example.com"
	}
	if err := all.Users.Create(ctx, user); err != nil {
		fmt.Fprintln(os.Stderr, "seed: user (already seeded?)", err)
		os.Exit(1)
	}
	org := &domain.Organization{Name: "Demo Org", Slug: fmt.Sprintf("demo-%s", user.ID.String()[:8])}
	if err := all.Organizations.Create(ctx, org); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
	if err := all.Organizations.AddMembership(ctx, &domain.Membership{OrgID: org.ID, UserID: user.ID, Role: domain.RoleOwner}); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
	proj := &domain.Project{OrgID: org.ID, Name: "Production"}
	if err := all.Projects.Create(ctx, proj); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
	scope := store.Scope{OrgID: org.ID, ProjectID: proj.ID}
	for _, url := range []string{"https://example.com", "https://httpbin.org/get"} {
		mon := &domain.Monitor{
			OrgID: org.ID, ProjectID: proj.ID, Name: url,
			Type: domain.MonitorTypeHTTP, URL: url,
			IntervalSeconds: 60, TimeoutMs: 10000, FailureThreshold: 3,
			State: domain.MonitorStatePaused, CreatedBy: user.ID,
		}
		if err := all.Monitors.Create(ctx, scope, mon); err != nil {
			fmt.Fprintln(os.Stderr, "seed:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("seed: user %s, org %s, project %s created; monitors added (paused)\n",
		user.Email, org.Slug, proj.ID)
}
