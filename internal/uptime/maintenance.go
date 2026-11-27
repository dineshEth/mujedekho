package uptime

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// RollupRetention is how long 1-minute rollups are kept. It must cover the
// longest preset uptime window (90d) even though raw checks expire sooner.
const RollupRetention = 90 * 24 * time.Hour

// PruneChecks deletes raw checks older than before.
func (s *Systems) PruneChecks(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM monitoring_checks WHERE checked_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("uptime: prune checks: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RunMaintenance keeps rollups current and enforces retention until ctx ends.
// On start it rolls up the whole raw-retention window once (catch-up after
// downtime), then refreshes the recent tail every `every`, and prunes hourly.
// Every step is idempotent, so several worker replicas may run it at once.
func (s *Systems) RunMaintenance(ctx context.Context, rawRetention, every time.Duration, log *slog.Logger) {
	now := time.Now().UTC()
	if _, err := s.RollupMins(ctx, now.Add(-rawRetention), now.Add(time.Minute)); err != nil && ctx.Err() == nil {
		log.Error("uptime: catch-up rollup failed", "err", err)
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	lastPrune := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		now := time.Now().UTC()
		// The tail overlaps previous passes so late-arriving checks (retries,
		// slow workers) land in their bucket; +1m includes the current minute.
		if _, err := s.RollupMins(ctx, now.Add(-10*time.Minute), now.Add(time.Minute)); err != nil && ctx.Err() == nil {
			log.Error("uptime: rollup failed", "err", err)
		}
		if now.Sub(lastPrune) >= time.Hour {
			lastPrune = now
			if n, err := s.PruneChecks(ctx, now.Add(-rawRetention)); err != nil {
				log.Error("uptime: prune checks failed", "err", err)
			} else if n > 0 {
				log.Info("uptime: pruned raw checks", "rows", n)
			}
			if n, err := s.PruneRollups(ctx, now.Add(-RollupRetention)); err != nil {
				log.Error("uptime: prune rollups failed", "err", err)
			} else if n > 0 {
				log.Info("uptime: pruned rollups", "rows", n)
			}
		}
	}
}
