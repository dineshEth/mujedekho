package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"site24x7/internal/auth"
	"site24x7/internal/domain"
	"site24x7/internal/store"
)

// Identity is the authenticated principal attached to request context.
type Identity struct {
	UserID uuid.UUID
	Email  string
}

type ctxKey string

const (
	identityKey   ctxKey = "identity"
	membershipKey ctxKey = "membership"
)

// WithIdentity stores the identity in context.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFrom returns the authenticated principal; ok=false when absent.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	return id, ok
}

// RequireAuth verifies the Bearer access token and attaches Identity.
func RequireAuth(tokens *auth.Tokens, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(raw, prefix) {
				WriteError(w, log, http.StatusUnauthorized, "unauthorized", "Missing bearer token.", nil)
				return
			}
			claims, err := tokens.VerifyAccess(strings.TrimPrefix(raw, prefix))
			if err != nil {
				WriteError(w, log, http.StatusUnauthorized, "unauthorized", "Invalid or expired token.", nil)
				return
			}
			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				WriteError(w, log, http.StatusUnauthorized, "unauthorized", "Invalid token subject.", nil)
				return
			}
			ctx := WithIdentity(r.Context(), Identity{UserID: userID, Email: claims.Email})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole resolves the caller's membership for the org in the URL and
// enforces a minimum role. Viewer < editor < admin < owner.
func RequireRole(all *store.All, min domain.Role, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := IdentityFrom(r.Context())
			if !ok {
				WriteError(w, log, http.StatusUnauthorized, "unauthorized", "Authentication required.", nil)
				return
			}
			orgRaw := r.PathValue("orgID")
			orgID, ok := parseUUID(orgRaw)
			if !ok {
				WriteError(w, log, http.StatusBadRequest, "bad_request", "Invalid organization id.", nil)
				return
			}
			m, err := all.Organizations.Membership(r.Context(), orgID, id.UserID)
			if err != nil {
				if !WriteStoreError(w, log, err) {
					// Membership absence must be indistinguishable from a
					// nonexistent org (no tenant enumeration).
					WriteError(w, log, http.StatusNotFound, "not_found", "Organization not found.", nil)
				}
				return
			}
			if !m.Role.Satisfies(min) {
				WriteError(w, log, http.StatusForbidden, "forbidden", "Insufficient role for this action.", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
