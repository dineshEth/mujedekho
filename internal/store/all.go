package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// All is the assembled store: pool plus every repository, wired once at
// startup and passed down as a single dependency.
type All struct {
	Pool          *pgxpool.Pool
	Users         *Users
	Organizations *Organizations
	Projects      *Projects
	Monitors      *Monitors
	Checks        *Checks
	Jobs          *Jobs
	Incidents     *Incidents
}

// NewAll opens the pool and applies pending migrations.
func NewAll(ctx context.Context, dsn string) (*All, error) {
	pool, err := Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if _, err := Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return &All{
		Pool:          pool,
		Users:         NewUsers(pool),
		Organizations: NewOrganizations(pool),
		Projects:      NewProjects(pool),
		Monitors:      NewMonitors(pool),
		Checks:        NewChecks(pool),
		Jobs:          NewJobs(pool),
		Incidents:     NewIncidents(pool),
	}, nil
}

// Close releases the pool.
func (a *All) Close() {
	if a.Pool != nil {
		a.Pool.Close()
	}
}

// Remigrate is a no-op when the schema is current; exposed for tests.
func (a *All) Remigrate(ctx context.Context) error {
	_, err := Migrate(ctx, a.Pool)
	return err
}
