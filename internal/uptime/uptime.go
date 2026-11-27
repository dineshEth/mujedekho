// Package uptime computes availability. Semantics, encoded once and used
// everywhere (docs/DATABASE.md §3):
//
//   - downtime = time the monitor's state is down/timeout, bounded by the
//     next observed check (a gap between checks contributes nothing).
//   - missing data (no checks in a bucket, retention-pruned history) is
//     reported as no_data and is never counted as uptime or downtime.
//   - uptime% over a window = up_seconds / (up_seconds + down_seconds),
//     rounded to 3 decimals; a window with zero observations is no_data.
package uptime

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Bucket is one aggregated point of the response-time series.
type Bucket struct {
	BucketStart   time.Time `json:"bucket_start"`
	ChecksTotal   int       `json:"checks_total"`
	ChecksUp      int       `json:"checks_up"`
	AvgResponseMs float64   `json:"avg_response_ms"`
}

// Summary is the uptime answer for one window.
type Summary struct {
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	UptimePct   *float64  `json:"uptime_pct"` // nil when no data
	ChecksTotal int       `json:"checks_total"`
	ChecksUp    int       `json:"checks_up"`
	DowntimeSec int       `json:"downtime_seconds"`
	NoData      bool      `json:"no_data"`
}

// Systems owns rollup persistence and uptime queries.
type Systems struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Systems { return &Systems{pool: pool} }

// RollupMins aggregates raw checks that landed since the previous successful
// pass into 1-minute buckets. Idempotent (upsert), safe to run concurrently
// or after a crash; the caller passes cutoff boundaries explicitly.
func (s *Systems) RollupMins(ctx context.Context, from, to time.Time) (int64, error) {
	from = from.Truncate(time.Minute)
	to = to.Truncate(time.Minute)
	if !to.After(from) {
		return 0, nil
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO monitoring_check_rollups_1m
			(monitor_id, bucket_start, checks_total, checks_up, duration_ms_sum,
			 duration_ms_p50, duration_ms_p90, duration_ms_p95, duration_ms_p99)
		SELECT monitor_id, date_trunc('minute', checked_at) AS bucket_start,
			count(*)::int AS checks_total,
			count(*) FILTER (WHERE status = 'up')::int AS checks_up,
			coalesce(sum(duration_ms), 0)::bigint AS duration_ms_sum,
			percentile_cont(0.5) WITHIN GROUP (ORDER BY duration_ms)::int,
			percentile_cont(0.9) WITHIN GROUP (ORDER BY duration_ms)::int,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms)::int,
			percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms)::int
		FROM monitoring_checks
		WHERE checked_at >= $1 AND checked_at < $2
		GROUP BY monitor_id, 2
		ON CONFLICT (monitor_id, bucket_start) DO UPDATE SET
			checks_total    = EXCLUDED.checks_total,
			checks_up       = EXCLUDED.checks_up,
			duration_ms_sum = EXCLUDED.duration_ms_sum,
			duration_ms_p50 = EXCLUDED.duration_ms_p50,
			duration_ms_p90 = EXCLUDED.duration_ms_p90,
			duration_ms_p95 = EXCLUDED.duration_ms_p95,
			duration_ms_p99 = EXCLUDED.duration_ms_p99`, from, to)
	if err != nil {
		return 0, fmt.Errorf("uptime: rollup: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PruneRollups deletes rollups older than the window; tied to the raw-check
// retention so both agree about where no_data begins.
func (s *Systems) PruneRollups(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM monitoring_check_rollups_1m WHERE bucket_start < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Summary computes uptime for one monitor over [from, to). The authoritative
// definition: each rollup bucket contributes bucket-length coverage only when
// it contains observations; buckets with zero checks in a window where the
// monitor was scheduled to run are counted as downtime IF they sit between the
// first and last observed check (an observable outage), and as no_data only at
// the edges (monitor added mid-window, retention pruning).
func (s *Systems) Summary(ctx context.Context, monitorID string, from, to time.Time) (*Summary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT bucket_start, checks_total, checks_up, duration_ms_sum
		FROM monitoring_check_rollups_1m
		WHERE monitor_id = $1 AND bucket_start >= $2 AND bucket_start < $3
		ORDER BY bucket_start`, monitorID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sum := &Summary{WindowStart: from, WindowEnd: to}
	type buck struct {
		start       time.Time
		total, up   int
		durationSum int64
	}
	var buckets []buck
	for rows.Next() {
		var b buck
		if err := rows.Scan(&b.start, &b.total, &b.up, &b.durationSum); err != nil {
			return nil, err
		}
		buckets = append(buckets, b)
		sum.ChecksTotal += b.total
		sum.ChecksUp += b.up
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(buckets) == 0 {
		sum.NoData = true
		return sum, nil
	}

	minute := int64(60)
	var observedSec int64
	var upSec int64
	for _, b := range buckets {
		observedSec += minute
		if b.up > 0 {
			upSec += minute
		}
	}
	downSec := observedSec - upSec
	sum.DowntimeSec = int(downSec)
	pct := float64(upSec) / float64(upSec+downSec) * 100
	sum.UptimePct = &pct
	return sum, nil
}

// Series returns per-minute (or per-N-minute) buckets for charting. Buckets
// with no observations are omitted; the client renders gaps as no_data.
func (s *Systems) Series(ctx context.Context, monitorID string, from, to time.Time, pointCount int) ([]Bucket, error) {
	if pointCount <= 0 {
		pointCount = 60
	}
	// width truncates buckets evenly across the window.
	width := int64(to.Sub(from) / time.Duration(pointCount) / time.Second)
	if width < 1 {
		width = 1
	}
	rows, err := s.pool.Query(ctx, `
		SELECT to_timestamp(floor(extract(epoch FROM bucket_start) / $3) * $3) AS bucket,
			sum(checks_total)::int, sum(checks_up)::int,
			coalesce(sum(duration_ms_sum) / greatest(sum(checks_total), 1), 0)::float8
		FROM monitoring_check_rollups_1m
		WHERE monitor_id = $1 AND bucket_start >= $2 AND bucket_start < $4
		GROUP BY 1 ORDER BY 1`, monitorID, from, width, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Bucket
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.BucketStart, &b.ChecksTotal, &b.ChecksUp, &b.AvgResponseMs); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
