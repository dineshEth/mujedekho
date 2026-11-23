package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"site24x7/internal/domain"
)

// Scope is the mandatory tenancy scope for every monitor- and check-scoped
// operation. Handlers build it from the authenticated membership; repositories
// refuse zero values.
type Scope struct {
	OrgID     uuid.UUID
	ProjectID uuid.UUID
}

// Validate asserts both halves are present.
func (s Scope) Validate() error {
	if s.OrgID == uuid.Nil || s.ProjectID == uuid.Nil {
		return fmt.Errorf("store: incomplete scope")
	}
	return nil
}

// Monitors is the tenant-scoped monitor repository.
type Monitors struct{ pool *pgxpool.Pool }

func NewMonitors(pool *pgxpool.Pool) *Monitors { return &Monitors{pool: pool} }

// ListFilter narrows listing results.
type ListFilter struct {
	State  string // "", "up", "down", "paused"
	Type   string
	Search string
	Limit  int
	Offset int
}

// Create inserts the monitor and arms its schedule (monitor_jobs row) in one
// transaction. New monitors start paused; the caller resumes explicitly.
func (m *Monitors) Create(ctx context.Context, scope Scope, mon *domain.Monitor) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if scope.OrgID != mon.OrgID || scope.ProjectID != mon.ProjectID {
		return fmt.Errorf("store: monitor identifiers disagree with scope")
	}
	reqJSON, err := json.Marshal(mon.RequestConfig)
	if err != nil {
		return err
	}
	chkJSON, err := json.Marshal(mon.CheckConfig)
	if err != nil {
		return err
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // rollback after commit is a no-op

	err = tx.QueryRow(ctx, `
		INSERT INTO monitors (org_id, project_id, name, monitor_type, url,
			request_config, check_config, interval_seconds, timeout_ms,
			failure_threshold, state, created_by)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8,$9,$10,$11,$12)
		RETURNING id, created_at, updated_at`,
		mon.OrgID, mon.ProjectID, mon.Name, string(mon.Type), mon.URL,
		string(reqJSON), string(chkJSON), mon.IntervalSeconds, mon.TimeoutMs,
		mon.FailureThreshold, string(mon.State), mon.CreatedBy,
	).Scan(&mon.ID, &mon.CreatedAt, &mon.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: create monitor: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO monitor_jobs (monitor_id, next_run_at, status)
		VALUES ($1, now(), 'pending')`, mon.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const monitorColumns = `id, org_id, project_id, name, monitor_type, url,
	request_config, check_config, interval_seconds, timeout_ms,
	failure_threshold, state, deleted, created_by, created_at, updated_at,
	consecutive_failures`

func scanMonitor(row pgx.Row) (*domain.Monitor, error) {
	var mon domain.Monitor
	var typ, state string
	var reqJSON, chkJSON []byte
	err := row.Scan(&mon.ID, &mon.OrgID, &mon.ProjectID, &mon.Name, &typ, &mon.URL,
		&reqJSON, &chkJSON, &mon.IntervalSeconds, &mon.TimeoutMs,
		&mon.FailureThreshold, &state, &mon.Deleted, &mon.CreatedBy,
		&mon.CreatedAt, &mon.UpdatedAt, &mon.ConsecutiveFailures)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan monitor: %w", err)
	}
	mon.Type = domain.MonitorType(typ)
	mon.State = domain.MonitorState(state)
	if err := json.Unmarshal(reqJSON, &mon.RequestConfig); err != nil {
		return nil, fmt.Errorf("store: request_config: %w", err)
	}
	if err := json.Unmarshal(chkJSON, &mon.CheckConfig); err != nil {
		return nil, fmt.Errorf("store: check_config: %w", err)
	}
	return &mon, nil
}

// Get fetches one monitor inside an org. The org filter is mandatory.
func (m *Monitors) Get(ctx context.Context, orgID, monitorID uuid.UUID) (*domain.Monitor, error) {
	row := m.pool.QueryRow(ctx, `
		SELECT `+monitorColumns+` FROM monitors
		WHERE id = $1 AND org_id = $2 AND deleted = false`, monitorID, orgID)
	return scanMonitor(row)
}

// List returns scoped monitors plus the total count for the filter.
func (m *Monitors) List(ctx context.Context, scope Scope, f ListFilter) ([]domain.Monitor, int, error) {
	if err := scope.Validate(); err != nil {
		return nil, 0, err
	}
	where := []string{"project_id = $1", "org_id = $2", "deleted = false"}
	args := []any{scope.ProjectID, scope.OrgID}
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	if f.State != "" {
		where = append(where, "state = "+arg(f.State))
	}
	if f.Type != "" {
		where = append(where, "monitor_type = "+arg(f.Type))
	}
	if f.Search != "" {
		where = append(where, "name ILIKE "+arg("%"+strings.ReplaceAll(f.Search, "%", "")+"%"))
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	w := strings.Join(where, " AND ")

	var total int
	if err := m.pool.QueryRow(ctx,
		`SELECT count(*) FROM monitors WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `SELECT ` + monitorColumns + ` FROM monitors WHERE ` + w +
		fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, f.Limit, f.Offset)
	rows, err := m.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.Monitor
	for rows.Next() {
		mon, err := scanMonitor(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *mon)
	}
	return out, total, rows.Err()
}

// Update persists mutable fields of a scoped monitor.
func (m *Monitors) Update(ctx context.Context, scope Scope, mon *domain.Monitor) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	reqJSON, err := json.Marshal(mon.RequestConfig)
	if err != nil {
		return err
	}
	chkJSON, err := json.Marshal(mon.CheckConfig)
	if err != nil {
		return err
	}
	tag, err := m.pool.Exec(ctx, `
		UPDATE monitors SET name=$3, url=$4, request_config=$5::jsonb,
			check_config=$6::jsonb, interval_seconds=$7, timeout_ms=$8,
			failure_threshold=$9, updated_at=now()
		WHERE id=$1 AND org_id=$2 AND deleted=false`,
		mon.ID, scope.OrgID, mon.Name, mon.URL,
		string(reqJSON), string(chkJSON), mon.IntervalSeconds, mon.TimeoutMs,
		mon.FailureThreshold)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetState transitions monitor state and reschedules jobs on pause.
func (m *Monitors) SetState(ctx context.Context, scope Scope, monitorID uuid.UUID, state domain.MonitorState) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	tag, err := m.pool.Exec(ctx, `
		UPDATE monitors SET state=$3, updated_at=now()
		WHERE id=$1 AND org_id=$2 AND deleted=false`,
		monitorID, scope.OrgID, string(state))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if state == domain.MonitorStatePaused {
		_, err = m.pool.Exec(ctx, `
			UPDATE monitor_jobs SET status='pending', next_run_at='infinity',
				lease_expires_at=NULL WHERE monitor_id=$1`, monitorID)
	} else {
		_, err = m.pool.Exec(ctx, `
			UPDATE monitor_jobs SET status='pending', next_run_at=now()
			WHERE monitor_id=$1 AND next_run_at='infinity'`, monitorID)
	}
	return err
}

// RecordOutcome folds one check outcome into the failure counter and
// visibility state: successes reset the counter and flip state to up; failures
// increment it and flip state to down only once the threshold is reached.
// It returns the new consecutive-failure count.
func (m *Monitors) RecordOutcome(ctx context.Context, monitorID uuid.UUID, up bool, threshold int) (int, error) {
	var consec int
	err := m.pool.QueryRow(ctx, `
		UPDATE monitors SET
			consecutive_failures = CASE WHEN $2 THEN 0 ELSE consecutive_failures + 1 END,
			state = CASE
				WHEN $2 THEN 'up'::text
				WHEN consecutive_failures + 1 >= $3 THEN 'down'::text
				ELSE state
			END,
			updated_at = now()
		WHERE id = $1 AND deleted = false
		RETURNING consecutive_failures`,
		monitorID, up, threshold).Scan(&consec)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return consec, err
}

// SoftDelete pauses scheduling and hides the monitor from lists; history and
// incidents remain for forensics.
func (m *Monitors) SoftDelete(ctx context.Context, scope Scope, monitorID uuid.UUID) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	tag, err := m.pool.Exec(ctx, `
		UPDATE monitors SET deleted=true, state='paused', updated_at=now()
		WHERE id=$1 AND org_id=$2 AND deleted=false`, monitorID, scope.OrgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = m.pool.Exec(ctx, `
		UPDATE monitor_jobs SET status='pending', next_run_at='infinity',
			lease_expires_at=NULL WHERE monitor_id=$1`, monitorID)
	return err
}
