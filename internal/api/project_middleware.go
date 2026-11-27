package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"site24x7/internal/store"
)

// projectScope is the tenant-validated (orgID, projectID) pair attached to
// request context by RequireProject.
type projectScope struct {
	OrgID     uuid.UUID
	ProjectID uuid.UUID
}

const projectScopeKey ctxKey = "projectScope"

// ProjectScopeFrom returns the validated scope; ok=false when absent.
func ProjectScopeFrom(ctx context.Context) (projectScope, bool) {
	ps, ok := ctx.Value(projectScopeKey).(projectScope)
	return ps, ok
}

// RequireProject resolves {projectID} under {orgID} and enforces tenant
// isolation at the URL level: projects of another org 404, never 403.
func RequireProject(all *store.All, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			orgID, ok := parseUUID(r.PathValue("orgID"))
			if !ok {
				WriteError(w, log, http.StatusBadRequest, "bad_request", "Invalid organization id.", nil)
				return
			}
			projID, ok := parseUUID(r.PathValue("projectID"))
			if !ok {
				WriteError(w, log, http.StatusBadRequest, "bad_request", "Invalid project id.", nil)
				return
			}
			if _, err := all.Projects.Get(r.Context(), orgID, projID); err != nil {
				if !WriteStoreError(w, log, err) {
					WriteInternal(w, log, err, "resolve project")
				}
				return
			}
			scope := projectScope{OrgID: orgID, ProjectID: projID}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), projectScopeKey, scope)))
		})
	}
}
