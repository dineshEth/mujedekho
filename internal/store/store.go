// Package store owns PostgreSQL access: pooling, migrations, and the scoped
// repositories. Every monitor/check-read path requires an org or project scope
// so tenant isolation is enforced at the data layer, not only in handlers.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by getters when the scoped lookup matched nothing.
var ErrNotFound = errors.New("store: not found")

// ErrConflict is returned on unique-constraint violations.
var ErrConflict = errors.New("store: conflict")

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open creates a connection pool.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return pool, nil
}

// Migrate applies pending migrations in order inside transactions.
// It returns the number of applied migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return 0, fmt.Errorf("store: migrations table: %w", err)
	}

	entries, err := fs.Glob(migrationsFS, "migrations/*.up.sql")
	if err != nil {
		return 0, fmt.Errorf("store: glob migrations: %w", err)
	}
	sort.Strings(entries)

	for _, path := range entries {
		version := versionOf(path)
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists); err != nil {
			return 0, err
		}
		if exists {
			continue
		}
		sqlBytes, err := migrationsFS.ReadFile(path)
		if err != nil {
			return 0, fmt.Errorf("store: read %s: %w", path, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return 0, fmt.Errorf("store: apply %s: %w", path, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return 0, err
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
	}
	return len(entries), nil
}

func versionOf(path string) string {
	base := path[strings.LastIndex(path, "/")+1:]
	if i := strings.Index(base, "."); i > 0 {
		return base[:i]
	}
	return base
}
