package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"site24x7/internal/auth"
	"site24x7/internal/domain"
	"site24x7/internal/store"
	"site24x7/internal/uptime"
)

// API bundles every handler dependency. Handlers remain methods on their own
// types; this struct is only the wiring root.
type API struct {
	All    *store.All
	Tokens *auth.Tokens
	Probe  MonitorProber // used by the synchronous manual-test endpoint
	Up     *uptime.Systems
	Log    *slog.Logger
}

// MonitorProber is satisfied by *worker.HTTPProbe.
type MonitorProber interface {
	Probe(ctx context.Context, mon *domain.Monitor) domain.CheckResult
}

// NewRouter builds the full /api/v1 stack.
func NewRouter(a *API) http.Handler {
	authH := &AuthHandler{All: a.All, Tokens: a.Tokens, Log: a.Log}
	orgs := &OrgHandler{All: a.All, Log: a.Log}
	projs := &ProjectHandler{All: a.All, Log: a.Log}
	mons := &MonitorHandler{All: a.All, Probe: a.Probe, Uptime: a.Up, Log: a.Log}

	r := chi.NewRouter()

	r.Use(recoverMiddleware(a.Log))
	r.Use(requestLog(a.Log))
	r.Use(securityHeaders)
	r.Use(httprate.LimitByIP(600, time.Minute)) // coarse per-IP ceiling beneath per-route limits

	// Health endpoints (unversioned, unauthenticated).
	r.Get("/healthz", handleHealth(a.Log))
	r.Get("/readyz", a.handleReady)

	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Get("/openapi.yaml", OpenAPISchema())
		v1.Group(func(pub chi.Router) {
			pub.Use(httprate.LimitByIP(30, time.Minute)) // auth endpoints are abuse-sensitive
			pub.Post("/auth/register", authH.Register)
			pub.Post("/auth/login", authH.Login)
		})
		v1.With(RequireAuth(a.Tokens, a.Log)).Get("/auth/me", authH.Me)

		v1.Route("/organizations", func(orgRoot chi.Router) {
			orgRoot.Use(RequireAuth(a.Tokens, a.Log))
			orgRoot.Post("/", orgs.Create)  // creator becomes owner
			orgRoot.Get("/", orgs.ListMine) // memberships of caller
			orgRoot.Route("/{orgID}", func(org chi.Router) {
				org.Use(RequireRole(a.All, domain.RoleViewer, a.Log))
				org.Get("/", orgs.Get)
				org.Get("/projects", projs.List)
				org.With(RequireRole(a.All, domain.RoleEditor, a.Log)).Post("/projects", projs.Create)

				org.Route("/projects/{projectID}/monitors", func(m chi.Router) {
					m.Use(RequireProject(a.All, a.Log))
					m.Get("/", mons.List)
					m.With(RequireRole(a.All, domain.RoleEditor, a.Log)).Post("/", mons.Create)
					m.Route("/{monitorID}", func(one chi.Router) {
						one.Get("/", mons.Get)
						one.Get("/history", mons.History)
						one.Get("/uptime", mons.UptimeSummary)
						one.With(RequireRole(a.All, domain.RoleEditor, a.Log)).Put("/", mons.Update)
						one.With(RequireRole(a.All, domain.RoleEditor, a.Log)).Delete("/", mons.Delete)
						one.With(RequireRole(a.All, domain.RoleEditor, a.Log)).Post("/pause", mons.Pause)
						one.With(RequireRole(a.All, domain.RoleEditor, a.Log)).Post("/resume", mons.Resume)
						one.With(RequireRole(a.All, domain.RoleViewer, a.Log)).Post("/test", mons.Test)
					})
				})

				org.Route("/projects/{projectID}/incidents", func(inc chi.Router) {
					inc.Use(RequireProject(a.All, a.Log))
					inc.Get("/", mons.ListIncidents)
					inc.Get("/{incidentID}", mons.GetIncident)
				})
			})
		})
	})

	return r
}

func handleHealth(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, log, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func (a *API) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := a.All.Pool.Ping(r.Context()); err != nil {
		WriteError(w, a.Log, http.StatusServiceUnavailable, "not_ready", "Database unreachable.", nil)
		return
	}
	writeJSON(w, a.Log, http.StatusOK, map[string]string{"status": "ready"})
}

func recoverMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("api: panic", "panic", rec, "path", r.URL.Path)
					WriteError(w, log, http.StatusInternalServerError, "internal_error", "An internal error occurred.", nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func requestLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)
			log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Write keeps the underlying implementation via embedding; only WriteHeader
// is overridden. No extra methods needed.

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
