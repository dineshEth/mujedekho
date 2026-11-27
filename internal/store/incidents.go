package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"site24x7/internal/domain"
)

// Incidents owns the incident lifecycle.
type Incidents struct{ pool *pgxpool.Pool }

func NewIncidents(pool *pgxpool.Pool) *Incidents { return &Incidents{pool: pool} }

// Open starts a new incident with its `opened` event. It reports whether a
// new incident was created (false when an open incident already exists, which
// is the authoritative multi-writer dedup gate).
func (i *Incidents) Open(ctx context.Context, monitorID uuid.UUID, startedAt time.Time, causeClass string) (uuid.UUID, bool, error) {
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // legal after Commit

	var existing uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT id FROM incidents WHERE monitor_id = $1 AND status = 'open' FOR UPDATE`,
		monitorID).Scan(&existing)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// proceed to open
	case err != nil:
		return uuid.Nil, false, err
	default:
		return existing, false, nil
	}

	var id uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO incidents (monitor_id, started_at, cause_class, status)
		VALUES ($1, $2, $3, 'open') RETURNING id`, monitorID, startedAt, causeClass).Scan(&id); err != nil {
		return uuid.Nil, false, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO incident_events (incident_id, event_type, payload)
		VALUES ($1, 'opened', jsonb_build_object('cause_class', $2::text))`, id, causeClass); err != nil {
		return uuid.Nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

// Resolve closes an open incident with the recovery event. It reports whether
// this call actually did the closing (one closer wins).
func (i *Incidents) Resolve(ctx context.Context, incidentID uuid.UUID, recoveredAt time.Time) (bool, error) {
	tag, err := i.pool.Exec(ctx, `
		UPDATE incidents SET
			status = 'resolved',
			resolved_at = $2,
			resolved_duration_s = GREATEST(0, EXTRACT(EPOCH FROM ($2 - started_at))::int)
		WHERE id = $1 AND status = 'open'`, incidentID, recoveredAt)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	_, err = i.pool.Exec(ctx, `
		INSERT INTO incident_events (incident_id, event_type, payload)
		VALUES ($1, 'recovered', jsonb_build_object('recovered_at', $2::timestamptz))`,
		incidentID, recoveredAt)
	return true, err
}

// OpenForMonitor returns the open incident or nil.
func (i *Incidents) OpenForMonitor(ctx context.Context, monitorID uuid.UUID) (*domain.Incident, error) {
	row := i.pool.QueryRow(ctx, `
		SELECT id, monitor_id, started_at, resolved_at, resolved_duration_s,
			cause_class, status
		FROM incidents WHERE monitor_id = $1 AND status = 'open'`, monitorID)
	return i.scanOne(row)
}

func (i *Incidents) scanOne(row pgx.Row) (*domain.Incident, error) {
	var inc domain.Incident
	var status string
	err := row.Scan(&inc.ID, &inc.MonitorID, &inc.StartedAt, &inc.ResolvedAt,
		&inc.ResolvedSeconds, &inc.CauseClass, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	inc.Status = domain.IncidentStatus(status)
	return &inc, nil
}

// IncidentFilter narrows incident listings.
type IncidentFilter struct {
	Status    string // "", "open", "resolved"
	MonitorID *uuid.UUID
	Limit     int
	Offset    int
}

// List returns incidents visible to an organization (optionally narrowed to
// one project and one monitor), newest first, plus the total count.
func (i *Incidents) List(ctx context.Context, orgID uuid.UUID, projectID *uuid.UUID, f IncidentFilter) ([]domain.Incident, int, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	where := []string{"m.org_id = $1"}
	args := []any{orgID}
	argNum := len(args)
	add := func(v any) string {
		args = append(args, v)
		argNum++
		return fmt.Sprintf("$%d", argNum)
	}
	if projectID != nil {
		where = append(where, "m.project_id = "+add(*projectID))
	}
	if f.Status != "" {
		where = append(where, "inc.status = "+add(f.Status))
	}
	if f.MonitorID != nil {
		where = append(where, "inc.monitor_id = "+add(*f.MonitorID))
	}
	w := strings.Join(where, " AND ")

	var total int
	if err := i.pool.QueryRow(ctx, `
		SELECT count(*) FROM incidents inc
		JOIN monitors m ON m.id = inc.monitor_id
		WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `
		SELECT inc.id, inc.monitor_id, inc.started_at, inc.resolved_at,
			inc.resolved_duration_s, inc.cause_class, inc.status
		FROM incidents inc
		JOIN monitors m ON m.id = inc.monitor_id
		WHERE ` + w + fmt.Sprintf(` ORDER BY inc.started_at DESC LIMIT %d OFFSET %d`, f.Limit, f.Offset)
	rows, err := i.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.Incident
	for rows.Next() {
		var inc domain.Incident
		var status string
		if err := rows.Scan(&inc.ID, &inc.MonitorID, &inc.StartedAt, &inc.ResolvedAt,
			&inc.ResolvedSeconds, &inc.CauseClass, &status); err != nil {
			return nil, 0, err
		}
		inc.Status = domain.IncidentStatus(status)
		out = append(out, inc)
	}
	return out, total, rows.Err()
}

// Events returns the ordered timeline for an incident.
func (i *Incidents) Events(ctx context.Context, incidentID uuid.UUID) ([]domain.IncidentEvent, error) {
	rows, err := i.pool.Query(ctx, `
		SELECT id, incident_id, event_type, payload, created_at
		FROM incident_events WHERE incident_id = $1 ORDER BY created_at`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.IncidentEvent
	for rows.Next() {
		var ev domain.IncidentEvent
		var typ string
		if err := rows.Scan(&ev.ID, &ev.IncidentID, &typ, &ev.Payload, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.EventType = domain.IncidentEventType(typ)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// Get fetches one incident that must belong to a monitor of the org.
func (i *Incidents) Get(ctx context.Context, orgID, incidentID uuid.UUID) (*domain.Incident, error) {
	row := i.pool.QueryRow(ctx, `
		SELECT inc.id, inc.monitor_id, inc.started_at, inc.resolved_at,
			inc.resolved_duration_s, inc.cause_class, inc.status
		FROM incidents inc
		JOIN monitors m ON m.id = inc.monitor_id
		WHERE inc.id = $1 AND m.org_id = $2`, incidentID, orgID)
	return i.scanOne(row)
}

// Annotate appends a manual event.
func (i *Incidents) Annotate(ctx context.Context, incidentID uuid.UUID, note string) error {
	_, err := i.pool.Exec(ctx, `
		INSERT INTO incident_events (incident_id, event_type, payload)
		VALUES ($1, 'annotated', jsonb_build_object('note', $2::text))`, incidentID, note)
	return err
}
