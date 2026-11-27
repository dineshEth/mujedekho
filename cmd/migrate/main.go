// Command migrate applies pending PostgreSQL migrations and exits.
package main

import (
	"context"
	"fmt"
	"os"

	"site24x7/internal/config"
	"site24x7/internal/store"
)

func main() {
	config.LoadDotEnv()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "migrate: DATABASE_URL is required")
		os.Exit(2)
	}
	ctx := context.Background()
	pool, err := store.Open(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	defer pool.Close()
	applied, err := store.Migrate(ctx, pool)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	fmt.Printf("migrate: %d pending migration(s) applied\n", applied)
}
