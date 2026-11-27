package api

import (
	"net/http"
	"regexp"
	"strings"

	"log/slog"

	"site24x7/internal/domain"
	"site24x7/internal/store"
)

// OrgHandler serves /api/v1/organizations.
type OrgHandler struct {
	All *store.All
	Log *slog.Logger
}

type createOrgRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Create makes a new organization; the caller becomes its owner.
func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	id, ok := IdentityFrom(r.Context())
	if !ok {
		WriteError(w, h.Log, http.StatusUnauthorized, "unauthorized", "Authentication required.", nil)
		return
	}
	var req createOrgRequest
	if errs, ok := BodyUnknownFields(r, &req); !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Malformed JSON body.", errs)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	if req.Name == "" || len(req.Name) > 100 {
		WriteError(w, h.Log, http.StatusUnprocessableEntity, "validation_failed", "Invalid organization payload.", map[string]any{"name": "1..100 characters required"})
		return
	}
	if !slugRe.MatchString(req.Slug) {
		WriteError(w, h.Log, http.StatusUnprocessableEntity, "validation_failed", "Invalid organization payload.", map[string]any{"slug": "lowercase letters, digits and dashes (3..50)"})
		return
	}

	org := &domain.Organization{Name: req.Name, Slug: req.Slug}
	if err := h.All.Organizations.Create(r.Context(), org); err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "create organization")
		}
		return
	}
	if err := h.All.Organizations.AddMembership(r.Context(), &domain.Membership{
		OrgID: org.ID, UserID: id.UserID, Role: domain.RoleOwner,
	}); err != nil {
		// Org exists without membership: rare half-state; surface as 500 but
		// log loudly. A migration-time account of this is in ARCHITECTURE.md.
		WriteInternal(w, h.Log, err, "add owner membership")
		return
	}
	writeJSON(w, h.Log, http.StatusCreated, org)
}

// ListMine returns the caller's organizations.
func (h *OrgHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	id, ok := IdentityFrom(r.Context())
	if !ok {
		WriteError(w, h.Log, http.StatusUnauthorized, "unauthorized", "Authentication required.", nil)
		return
	}
	orgs, err := h.All.Organizations.ListForUser(r.Context(), id.UserID)
	if err != nil {
		WriteInternal(w, h.Log, err, "list organizations")
		return
	}
	if orgs == nil {
		orgs = []domain.Organization{}
	}
	writeJSON(w, h.Log, http.StatusOK, map[string]any{"organizations": orgs})
}

// Get returns one organization (membership enforced by middleware).
func (h *OrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseUUID(r.PathValue("orgID"))
	if !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Invalid organization id.", nil)
		return
	}
	org, err := h.All.Organizations.Get(r.Context(), orgID)
	if err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "get organization")
		}
		return
	}
	writeJSON(w, h.Log, http.StatusOK, org)
}

// ProjectHandler serves project subresources.
type ProjectHandler struct {
	All *store.All
	Log *slog.Logger
}

type createProjectRequest struct {
	Name string `json:"name"`
}

// Create adds a project under the org (editor+ enforced upstream).
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseUUID(r.PathValue("orgID"))
	if !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Invalid organization id.", nil)
		return
	}
	var req createProjectRequest
	if errs, ok := BodyUnknownFields(r, &req); !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Malformed JSON body.", errs)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		WriteError(w, h.Log, http.StatusUnprocessableEntity, "validation_failed", "Invalid project payload.", map[string]any{"name": "1..100 characters required"})
		return
	}
	proj := &domain.Project{OrgID: orgID, Name: req.Name}
	if err := h.All.Projects.Create(r.Context(), proj); err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "create project")
		}
		return
	}
	writeJSON(w, h.Log, http.StatusCreated, proj)
}

// List returns projects for the org.
func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := parseUUID(r.PathValue("orgID"))
	if !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Invalid organization id.", nil)
		return
	}
	projs, err := h.All.Projects.List(r.Context(), orgID)
	if err != nil {
		WriteInternal(w, h.Log, err, "list projects")
		return
	}
	if projs == nil {
		projs = []domain.Project{}
	}
	writeJSON(w, h.Log, http.StatusOK, map[string]any{"projects": projs})
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,49}$`)
