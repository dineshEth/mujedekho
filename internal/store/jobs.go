package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"site24x7/internal/domain"
)

// Job is one claimed scheduling unit.
type Job struct {
	Monitor     *domain.Monitor
	ScheduledAt time.Time
}

// Jobs owns the durable scheduling queue. The claim protocol is:
//
//  1. DueBatch atomically flips due pending rows to 'leased' with a lease
//     expiry (FOR UPDATE SKIP LOCKED prevents two schedulers from claiming
//     the same row, which is the duplicate-job defense; the partial unique
//     pending index is the schema-level second line).
//  2. Workers run the check inside the lease window (lease = 3x check
//     timeout). A crashed worker never releases; ReclaimExpired returns the
//     job to the queue and records the timeout so the gap is visible.
//  3. Complete returns the job to pending with next_run =
//     max(scheduled_at + interval, now), clamping missed schedules to a
//     single make-up run instead of a backfill storm. The WHERE clause pins
//     scheduled_at: if the lease expired and another scheduler re-claimed
//     meanwhile, this completion is ignored.
type Jobs struct{ pool *pgxpool.Pool }

func NewJobs(pool *pgxpool.Pool) *Jobs { return &Jobs{pool: pool} }

// DueBatch claims up to limit due monitors and returns the matching monitor
// rows. Lease duration is passed in seconds.
func (j *Jobs) DueBatch(ctx context.Context, limit int, leaseSeconds int) ([]Job, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("store: DueBatch limit must be positive")
	}
	tx, err := j.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // legal after Commit

	rows, err := tx.Query(ctx, `
		UPDATE monitor_jobs j SET
			status = 'leased',
			lease_expires_at = now() + make_interval(secs => $2),
			scheduled_at = j.next_run_at,
			attempts = attempts + 1
		WHERE j.monitor_id IN (
			SELECT j2.monitor_id FROM monitor_jobs j2
			JOIN monitors m ON m.id = j2.monitor_id
			WHERE j2.status = 'pending'
			  AND j2.next_run_at <= now()
			  AND m.deleted = false
			  AND m.state <> 'paused'
			ORDER BY j2.next_run_at
			LIMIT $1
			FOR UPDATE OF j2 SKIP LOCKED
		)
		RETURNING j.monitor_id, j.scheduled_at`,
		limit, leaseSeconds)
	if err != nil {
		return nil, fmt.Errorf("store: claim jobs: %w", err)
	}
	type claimRow struct {
		id          uuid.UUID
		scheduledAt time.Time
	}
	var claims []claimRow
	for rows.Next() {
		var c claimRow
		if err := rows.Scan(&c.id, &c.scheduledAt); err != nil {
			rows.Close()
			return nil, err
		}
		claims = append(claims, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(claims) == 0 {
		return nil, tx.Commit(ctx)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	out := make([]Job, 0, len(claims))
	for _, c := range claims {
		mon, err := monitorByID(j.pool, ctx, c.id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// Monitor disappeared between claim and fetch; release.
				_, _ = j.pool.Exec(ctx, `
					UPDATE monitor_jobs SET status='pending', next_run_at=now()
					WHERE monitor_id=$1 AND status='leased'`, c.id)
				continue
			}
			return nil, err
		}
		out = append(out, Job{Monitor: mon, ScheduledAt: c.scheduledAt})
	}
	return out, nil
}

// Complete finalizes a successful run and schedules the next occurrence. It
// reports whether this completion landed (false means the lease expired and
// the job was reassigned on the way).
func (j *Jobs) Complete(ctx context.Context, monitorID uuid.UUID, scheduledAt time.Time, intervalSeconds int) (bool, error) {
	tag, err := j.pool.Exec(ctx, `
		UPDATE monitor_jobs SET
			status = 'pending',
			attempts = 0,
			next_run_at = GREATEST(scheduled_at + make_interval(secs => $2), now()),
			lease_expires_at = NULL
		WHERE monitor_id = $1 AND status = 'leased' AND scheduled_at = $3`,
		monitorID, intervalSeconds, scheduledAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ReclaimExpired returns overdue leases to pending and reports the affected
// monitors, so the caller can record visible timeout outcomes.
func (j *Jobs) ReclaimExpired(ctx context.Context, before time.Time) ([]uuid.UUID, error) {
	rows, err := j.pool.Query(ctx, `
		UPDATE monitor_jobs SET status = 'pending'
		WHERE status = 'leased' AND lease_expires_at <= $1
		RETURNING monitor_id, next_run_at`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		var next time.Time
		if err := rows.Scan(&id, &next); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DispatchNow ensures the monitor is scheduled for immediate run and marks it
// paused/removed scheduling when it is not-running. Used by pause/resume and
// admin tools.
func (j *Jobs) DispatchNow(ctx context.Context, monitorID uuid.UUID) error {
	_, err := j.pool.Exec(ctx, `
		UPDATE monitor_jobs SET status='pending', next_run_at=now()
		WHERE monitor_id=$1`, monitorID)
	return err
}

// Monitor fetch by id, unscoped (engine side), non-deleted only.
func monitorByID(pool *pgxpool.Pool, ctx context.Context, id uuid.UUID) (*domain.Monitor, error) {
	row := pool.QueryRow(ctx, `
		SELECT id, org_id, project_id, name, monitor_type, url,
			request_config, check_config, interval_seconds, timeout_ms,
			failure_threshold, state, deleted, created_by, created_at, updated_at,
			consecutive_failures
		FROM monitors WHERE id = $1 AND deleted = false`, id)
	mon, err := scanMonitor(row)
	if err != nil {
		return nil, err
	}
	return mon, nil
}
