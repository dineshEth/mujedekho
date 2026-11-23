package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"site24x7/internal/domain"
)

// Users stores accounts.
type Users struct{ pool *pgxpool.Pool }

func NewUsers(pool *pgxpool.Pool) *Users { return &Users{pool: pool} }

func (u *Users) Create(ctx context.Context, user *domain.User) error {
	return u.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`,
		user.Email, user.PasswordHash, user.FullName,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

func (u *Users) ByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := u.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, full_name, created_at, updated_at
		FROM users WHERE email = $1`, email)
	return scanUser(row)
}

func (u *Users) Get(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row := u.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, full_name, created_at, updated_at
		FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func scanUser(row pgx.Row) (*domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.FullName, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan user: %w", err)
	}
	return &user, nil
}

// Organizations stores tenants.
type Organizations struct{ pool *pgxpool.Pool }

func NewOrganizations(pool *pgxpool.Pool) *Organizations { return &Organizations{pool: pool} }

func (o *Organizations) Create(ctx context.Context, org *domain.Organization) error {
	return o.pool.QueryRow(ctx, `
		INSERT INTO organizations (name, slug) VALUES ($1, $2)
		RETURNING id, created_at`, org.Name, org.Slug,
	).Scan(&org.ID, &org.CreatedAt)
}

func (o *Organizations) Get(ctx context.Context, id uuid.UUID) (*domain.Organization, error) {
	row := o.pool.QueryRow(ctx,
		`SELECT id, name, slug, created_at FROM organizations WHERE id = $1`, id)
	var org domain.Organization
	if err := row.Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &org, nil
}

// ListForUser returns organizations the user is a member of.
func (o *Organizations) ListForUser(ctx context.Context, userID uuid.UUID) ([]domain.Organization, error) {
	rows, err := o.pool.Query(ctx, `
		SELECT o.id, o.name, o.slug, o.created_at
		FROM organizations o
		JOIN memberships m ON m.org_id = o.id
		WHERE m.user_id = $1
		ORDER BY o.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Organization
	for rows.Next() {
		var org domain.Organization
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, org)
	}
	return out, rows.Err()
}

// AddMembership inserts or replaces a membership role.
func (o *Organizations) AddMembership(ctx context.Context, m *domain.Membership) error {
	_, err := o.pool.Exec(ctx, `
		INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		m.OrgID, m.UserID, string(m.Role))
	return err
}

// Membership fetches the user's role in an organization, or ErrNotFound.
func (o *Organizations) Membership(ctx context.Context, orgID, userID uuid.UUID) (*domain.Membership, error) {
	row := o.pool.QueryRow(ctx, `
		SELECT org_id, user_id, role, created_at FROM memberships
		WHERE org_id = $1 AND user_id = $2`, orgID, userID)
	var m domain.Membership
	if err := row.Scan(&m.OrgID, &m.UserID, &m.Role, &m.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

// Projects stores monitor groupings.
type Projects struct{ pool *pgxpool.Pool }

func NewProjects(pool *pgxpool.Pool) *Projects { return &Projects{pool: pool} }

func (p *Projects) Create(ctx context.Context, proj *domain.Project) error {
	return p.pool.QueryRow(ctx, `
		INSERT INTO projects (org_id, name) VALUES ($1, $2)
		RETURNING id, created_at`, proj.OrgID, proj.Name,
	).Scan(&proj.ID, &proj.CreatedAt)
}

func (p *Projects) Get(ctx context.Context, orgID, id uuid.UUID) (*domain.Project, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, name, created_at FROM projects WHERE id = $1 AND org_id = $2`, id, orgID)
	var proj domain.Project
	if err := row.Scan(&proj.ID, &proj.OrgID, &proj.Name, &proj.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &proj, nil
}

func (p *Projects) List(ctx context.Context, orgID uuid.UUID) ([]domain.Project, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, org_id, name, created_at FROM projects
		WHERE org_id = $1 ORDER BY created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		var proj domain.Project
		if err := rows.Scan(&proj.ID, &proj.OrgID, &proj.Name, &proj.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, proj)
	}
	return out, rows.Err()
}
