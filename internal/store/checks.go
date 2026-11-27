package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"site24x7/internal/domain"
)

// Checks writes and reads monitoring history. Conflict handling implements the
// idempotency window: a duplicate write for the same (monitor, checked_at)
// from a recovered job or replayed result is a no-op.
type Checks struct{ pool *pgxpool.Pool }

func NewChecks(pool *pgxpool.Pool) *Checks { return &Checks{pool: pool} }

// Insert stores a check result. Duplicate (monitor, checked_at) writes are
// silently ignored. An unset Detail must arrive as SQL NULL — an empty string
// is not valid JSON for the ::jsonb cast.
func (c *Checks) Insert(ctx context.Context, r *domain.CheckResult) error {
	var detailParam any
	if len(r.Detail) > 0 {
		detailParam = []byte(r.Detail)
	}
	// Zero means "no HTTP status" (transport-level failure); the schema's
	// CHECK (http_status_code BETWEEN 100 AND 599) wants NULL, not 0.
	var httpStatus any
	if r.HTTPStatusCode >= 100 && r.HTTPStatusCode <= 599 {
		httpStatus = r.HTTPStatusCode
	}
	tag, err := c.pool.Exec(ctx, `
		INSERT INTO monitoring_checks (monitor_id, checked_at, status,
			http_status_code, duration_ms, dns_ms, connect_ms, tls_ms, ttfb_ms,
			error_class, error_detail, response_bytes, region, detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb)
		ON CONFLICT (monitor_id, checked_at) DO NOTHING`,
		r.MonitorID, r.CheckedAt, string(r.Status), httpStatus, r.DurationMs,
		r.DNSMs, r.ConnectMs, r.TLSMs, r.TTFBMs,
		string(r.ErrorClass), r.ErrorDetail, r.ResponseBytes, r.Region, detailParam)
	if err != nil {
		return fmt.Errorf("store: insert check: %w", err)
	}
	_ = tag
	return nil
}

// CheckPage is a cursor-paged history of checks.
type CheckPage struct {
	Items      []domain.CheckResult
	NextCursor *time.Time
}

// List returns checks for the monitor in descending time. cursor pages from
// (exclusive) that timestamp.
func (c *Checks) List(ctx context.Context, monitorID uuid.UUID, from, to time.Time, cursor *time.Time, limit int) (*CheckPage, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	q := `SELECT monitor_id, checked_at, status, COALESCE(http_status_code, 0), duration_ms,
			COALESCE(dns_ms, 0), COALESCE(connect_ms, 0), COALESCE(tls_ms, 0),
			COALESCE(ttfb_ms, 0), error_class, COALESCE(error_detail, ''),
			COALESCE(response_bytes, 0), region, detail
		FROM monitoring_checks
		WHERE monitor_id = $1 AND checked_at >= $2 AND checked_at < $3`
	args := []any{monitorID, from, to}
	if cursor != nil {
		q += ` AND checked_at < $4`
		args = append(args, *cursor)
	}
	q += fmt.Sprintf(` ORDER BY checked_at DESC LIMIT %d`, limit+1)

	rows, err := c.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &CheckPage{Items: make([]domain.CheckResult, 0, limit)}
	for rows.Next() {
		var r domain.CheckResult
		var status, class string
		var detail []byte
		if err := rows.Scan(&r.MonitorID, &r.CheckedAt, &status, &r.HTTPStatusCode,
			&r.DurationMs, &r.DNSMs, &r.ConnectMs, &r.TLSMs, &r.TTFBMs,
			&class, &r.ErrorDetail, &r.ResponseBytes, &r.Region, &detail); err != nil {
			return nil, err
		}
		r.Status = domain.CheckStatus(status)
		r.ErrorClass = domain.CheckErrorClass(class)
		if len(detail) > 0 {
			r.Detail = detail
		}
		page.Items = append(page.Items, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[len(page.Items)-1].CheckedAt
		page.NextCursor = &last
	}
	return page, nil
}

// Latest returns the most recent check for the monitor, or ErrNotFound.
func (c *Checks) Latest(ctx context.Context, monitorID uuid.UUID) (*domain.CheckResult, error) {
	row := c.pool.QueryRow(ctx, `
		SELECT monitor_id, checked_at, status, COALESCE(http_status_code, 0), duration_ms,
			COALESCE(dns_ms, 0), COALESCE(connect_ms, 0), COALESCE(tls_ms, 0),
			COALESCE(ttfb_ms, 0), error_class, COALESCE(error_detail, ''),
			COALESCE(response_bytes, 0), region, detail
		FROM monitoring_checks WHERE monitor_id = $1
		ORDER BY checked_at DESC LIMIT 1`, monitorID)
	var r domain.CheckResult
	var status, class string
	var detail []byte
	err := row.Scan(&r.MonitorID, &r.CheckedAt, &status, &r.HTTPStatusCode,
		&r.DurationMs, &r.DNSMs, &r.ConnectMs, &r.TLSMs, &r.TTFBMs,
		&class, &r.ErrorDetail, &r.ResponseBytes, &r.Region, &detail)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.Status = domain.CheckStatus(status)
	r.ErrorClass = domain.CheckErrorClass(class)
	if len(detail) > 0 {
		r.Detail = detail
	}
	return &r, nil
}
