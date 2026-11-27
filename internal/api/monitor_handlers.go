package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"site24x7/internal/domain"
	"site24x7/internal/safety"
	"site24x7/internal/store"
	"site24x7/internal/uptime"
)

// Prober performs checks; implemented by worker probe adapters. Kept as an
// interface so tests substitute a fake.
type Prober interface {
	Probe(ctx context.Context, mon *domain.Monitor) domain.CheckResult
}

// MonitorHandler serves /api/v1/organizations/{orgID}/projects/{projectID}/monitors.
type MonitorHandler struct {
	All    *store.All
	Probe  Prober
	Uptime *uptime.Systems
	Log    *slog.Logger
}

var errScopeMissing = errors.New("api: scope middleware not applied")

const (
	maxInterval  = 86400
	minInterval  = 60 // per PRD: initial product floors checks at one minute
	maxTimeoutMs = 30000
	maxNameLen   = 100
	maxURLLen    = 2048
)

type monitorPayload struct {
	Name             string                `json:"name"`
	Type             string                `json:"monitor_type"`
	URL              string                `json:"url"`
	IntervalSeconds  int                   `json:"interval_seconds"`
	TimeoutMs        int                   `json:"timeout_ms"`
	FailureThreshold int                   `json:"failure_threshold"`
	RequestConfig    *domain.RequestConfig `json:"request_config,omitempty"`
	CheckConfig      *domain.CheckConfig   `json:"check_config,omitempty"`
}

// Create registers a monitor. New monitors start paused; the client calls
// /resume to arm the schedule (explicit intent beats implicit first check).
func (h *MonitorHandler) Create(w http.ResponseWriter, r *http.Request) {
	scope, ok := ProjectScopeFrom(r.Context())
	if !ok {
		WriteError(w, h.Log, http.StatusInternalServerError, "internal_error", "Scope middleware missing.", nil)
		return
	}
	id, _ := IdentityFrom(r.Context())

	var p monitorPayload
	if errs, ok := BodyUnknownFields(r, &p); !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Malformed JSON body.", errs)
		return
	}
	if errs := h.validate(&p); errs != nil {
		WriteError(w, h.Log, http.StatusUnprocessableEntity, "validation_failed", "Invalid monitor payload.", errs)
		return
	}

	mon := &domain.Monitor{
		OrgID:            scope.OrgID,
		ProjectID:        scope.ProjectID,
		Name:             strings.TrimSpace(p.Name),
		Type:             domain.MonitorType(p.Type),
		URL:              strings.TrimSpace(p.URL),
		IntervalSeconds:  orDefault(p.IntervalSeconds, 60),
		TimeoutMs:        orDefault(p.TimeoutMs, 10000),
		FailureThreshold: orDefault(p.FailureThreshold, 3),
		State:            domain.MonitorStatePaused,
		CreatedBy:        id.UserID,
	}
	applyOptionalConfigs(mon, p)
	if err := h.All.Monitors.Create(r.Context(), store.Scope{OrgID: scope.OrgID, ProjectID: scope.ProjectID}, mon); err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "create monitor")
		}
		return
	}
	writeJSON(w, h.Log, http.StatusCreated, mon)
}

// List applies filters with sane bounds.
func (h *MonitorHandler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := ProjectScopeFrom(r.Context())
	if !ok {
		WriteInternal(w, h.Log, errScopeMissing, "scope middleware")
		return
	}
	f := store.ListFilter{
		State:  r.URL.Query().Get("state"),
		Type:   r.URL.Query().Get("type"),
		Search: r.URL.Query().Get("search"),
		Limit:  clampInt(queryInt(r, "limit", 50), 1, 200),
		Offset: queryInt(r, "offset", 0),
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	mons, total, err := h.All.Monitors.List(r.Context(), store.Scope{OrgID: scope.OrgID, ProjectID: scope.ProjectID}, f)
	if err != nil {
		WriteInternal(w, h.Log, err, "list monitors")
		return
	}
	if mons == nil {
		mons = []domain.Monitor{}
	}
	writeJSON(w, h.Log, http.StatusOK, map[string]any{"monitors": mons, "total": total})
}

// Get returns one monitor with its latest check attached.
func (h *MonitorHandler) Get(w http.ResponseWriter, r *http.Request) {
	mon, ok := h.loadMonitor(w, r)
	if !ok {
		return
	}
	resp := struct {
		*domain.Monitor
		LatestCheck *domain.CheckResult `json:"latest_check"`
	}{Monitor: mon}
	if latest, err := h.All.Checks.Latest(r.Context(), mon.ID); err == nil {
		resp.LatestCheck = latest
	}
	writeJSON(w, h.Log, http.StatusOK, resp)
}

// Update mutates configuration; URL changes re-validate SSRF guards.
func (h *MonitorHandler) Update(w http.ResponseWriter, r *http.Request) {
	scope, _ := ProjectScopeFrom(r.Context())
	mon, ok := h.loadMonitor(w, r)
	if !ok {
		return
	}
	var p monitorPayload
	if errs, ok := BodyUnknownFields(r, &p); !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Malformed JSON body.", errs)
		return
	}
	if p.Name != "" {
		if len(strings.TrimSpace(p.Name)) > maxNameLen {
			WriteError(w, h.Log, http.StatusUnprocessableEntity, "validation_failed", "Invalid monitor payload.", map[string]any{"name": "at most 100 characters"})
			return
		}
		mon.Name = strings.TrimSpace(p.Name)
	}
	if p.URL != "" {
		newURL := strings.TrimSpace(p.URL)
		if _, err := safety.ValidateURL(newURL); err != nil || len(newURL) > maxURLLen {
			WriteError(w, h.Log, http.StatusUnprocessableEntity, "validation_failed", "Invalid monitor payload.", map[string]any{"url": "must be a public http(s) URL"})
			return
		}
		mon.URL = newURL
	}
	if p.IntervalSeconds != 0 {
		mon.IntervalSeconds = p.IntervalSeconds
	}
	if p.TimeoutMs != 0 {
		mon.TimeoutMs = p.TimeoutMs
	}
	if p.FailureThreshold != 0 {
		mon.FailureThreshold = p.FailureThreshold
	}
	if p.RequestConfig != nil {
		mon.RequestConfig = *p.RequestConfig
	}
	if p.CheckConfig != nil {
		mon.CheckConfig = *p.CheckConfig
	}
	if errs := h.validateFields(mon.IntervalSeconds, mon.TimeoutMs, mon.FailureThreshold); errs != nil {
		WriteError(w, h.Log, http.StatusUnprocessableEntity, "validation_failed", "Invalid monitor payload.", errs)
		return
	}
	if err := h.All.Monitors.Update(r.Context(), scope2(scope), mon); err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "update monitor")
		}
		return
	}
	writeJSON(w, h.Log, http.StatusOK, mon)
}

// Pause halts scheduling.
func (h *MonitorHandler) Pause(w http.ResponseWriter, r *http.Request) {
	h.setState(w, r, domain.MonitorStatePaused)
}

// Resume arms the schedule.
func (h *MonitorHandler) Resume(w http.ResponseWriter, r *http.Request) {
	h.setState(w, r, domain.MonitorStateUp)
}

func (h *MonitorHandler) setState(w http.ResponseWriter, r *http.Request, state domain.MonitorState) {
	scope, _ := ProjectScopeFrom(r.Context())
	monID, ok := parseUUID(r.PathValue("monitorID"))
	if !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Invalid monitor id.", nil)
		return
	}
	if err := h.All.Monitors.SetState(r.Context(), scope2(scope), monID, state); err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "set monitor state")
		}
		return
	}
	writeJSON(w, h.Log, http.StatusOK, map[string]string{"id": monID.String(), "state": string(state)})
}

// Delete soft-deletes: history and incidents remain for forensics.
func (h *MonitorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	scope, _ := ProjectScopeFrom(r.Context())
	monID, ok := parseUUID(r.PathValue("monitorID"))
	if !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Invalid monitor id.", nil)
		return
	}
	if err := h.All.Monitors.SoftDelete(r.Context(), scope2(scope), monID); err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "delete monitor")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Test runs one synchronous check and persists it as a check row. It does
// NOT touch incident state (a manual check must never paging-resolve an
// incident); threshold counters are likewise untouched.
func (h *MonitorHandler) Test(w http.ResponseWriter, r *http.Request) {
	mon, ok := h.loadMonitor(w, r)
	if !ok {
		return
	}
	if h.Probe == nil {
		WriteInternal(w, h.Log, errScopeMissing, "probe wiring")
		return
	}
	res := h.Probe.Probe(r.Context(), mon)
	res.MonitorID = mon.ID
	res.CheckedAt = time.Now().UTC()
	if res.Region == "" {
		res.Region = "api"
	}
	// Persist for a complete audit trail; insert is idempotent on PK. Never
	// fail the response over persistence of this audit copy.
	resCopy := res
	if err := h.All.Checks.Insert(r.Context(), &resCopy); err != nil {
		h.Log.Warn("api: manual test check not persisted", "err", err)
	}
	writeJSON(w, h.Log, http.StatusOK, map[string]any{"result": res})
}

// History returns cursor-paged check results over a time window.
func (h *MonitorHandler) History(w http.ResponseWriter, r *http.Request) {
	mon, ok := h.loadMonitor(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	minutes := clampInt(queryInt(r, "window_minutes", 1440), 1, 60*24*90)
	to := time.Now().UTC()
	from := to.Add(-time.Duration(minutes) * time.Minute)
	var cursor *time.Time
	if c := q.Get("cursor"); c != "" {
		if t, err := time.Parse(time.RFC3339Nano, c); err == nil {
			cursor = &t
		}
	}
	page, err := h.All.Checks.List(r.Context(), mon.ID, from, to, cursor, clampInt(queryInt(r, "limit", 100), 1, 1000))
	if err != nil {
		WriteInternal(w, h.Log, err, "check history")
		return
	}
	if page.Items == nil {
		page.Items = []domain.CheckResult{}
	}
	next := ""
	if page.NextCursor != nil {
		next = page.NextCursor.Format(time.RFC3339Nano)
	}
	writeJSON(w, h.Log, http.StatusOK, map[string]any{"checks": page.Items, "next_cursor": next})
}

// UptimeSummary computes window uptime from rollups maintained by the worker.
func (h *MonitorHandler) UptimeSummary(w http.ResponseWriter, r *http.Request) {
	mon, ok := h.loadMonitor(w, r)
	if !ok {
		return
	}
	if h.Uptime == nil {
		WriteError(w, h.Log, http.StatusNotImplemented, "not_implemented", "Uptime analytics not wired.", nil)
		return
	}
	q := r.URL.Query()
	var from, to time.Time
	if raw := q.Get("from"); raw != "" {
		parsedFrom, perr := time.Parse(time.RFC3339, raw)
		if perr != nil {
			WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "from must be RFC3339.", nil)
			return
		}
		from, to = parsedFrom.UTC(), time.Now().UTC()
	} else {
		days, ok := presetWindow(q.Get("window"))
		if !ok {
			WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "window must be one of 24h, 7d, 30d, 90d or custom from/to.", nil)
			return
		}
		to = time.Now().UTC()
		from = to.Add(-days)
	}
	ctx := r.Context()
	// The worker keeps rollups current in the background; only refresh the
	// recent tail here so the answer includes the last few checks without
	// re-aggregating the whole (up to 90-day) window on every request.
	rollFrom := to.Add(-15 * time.Minute)
	if from.After(rollFrom) {
		rollFrom = from
	}
	if _, err := h.Uptime.RollupMins(ctx, rollFrom, to.Add(time.Minute)); err != nil {
		WriteInternal(w, h.Log, err, "pre-rollup")
		return
	}
	sum, err := h.Uptime.Summary(ctx, mon.ID.String(), from, to)
	if err != nil {
		WriteInternal(w, h.Log, err, "uptime summary")
		return
	}
	series, err := h.Uptime.Series(ctx, mon.ID.String(), from, to, 120)
	if err != nil {
		WriteInternal(w, h.Log, err, "uptime series")
		return
	}
	writeJSON(w, h.Log, http.StatusOK, map[string]any{
		"window": sum,
		"series": series,
	})
}

// ListIncidents lists incidents for the project.
func (h *MonitorHandler) ListIncidents(w http.ResponseWriter, r *http.Request) {
	scope, _ := ProjectScopeFrom(r.Context())
	f := store.IncidentFilter{
		Status: r.URL.Query().Get("status"),
		Limit:  clampInt(queryInt(r, "limit", 50), 1, 200),
		Offset: queryInt(r, "offset", 0),
	}
	if mid := r.URL.Query().Get("monitor_id"); mid != "" {
		if id, ok := parseUUID(mid); ok {
			f.MonitorID = &id
		}
	}
	incidents, total, err := h.All.Incidents.List(r.Context(), scope.OrgID, &scope.ProjectID, f)
	if err != nil {
		WriteInternal(w, h.Log, err, "list incidents")
		return
	}
	if incidents == nil {
		incidents = []domain.Incident{}
	}
	writeJSON(w, h.Log, http.StatusOK, map[string]any{"incidents": incidents, "total": total})
}

// GetIncident returns one incident with its timeline.
func (h *MonitorHandler) GetIncident(w http.ResponseWriter, r *http.Request) {
	scope, _ := ProjectScopeFrom(r.Context())
	incID, ok := parseUUID(r.PathValue("incidentID"))
	if !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Invalid incident id.", nil)
		return
	}
	inc, err := h.All.Incidents.Get(r.Context(), scope.OrgID, incID)
	if err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "get incident")
		}
		return
	}
	events, err := h.All.Incidents.Events(r.Context(), inc.ID)
	if err != nil {
		WriteInternal(w, h.Log, err, "incident events")
		return
	}
	if events == nil {
		events = []domain.IncidentEvent{}
	}
	writeJSON(w, h.Log, http.StatusOK, map[string]any{"incident": inc, "events": events})
}

// --- helpers --------------------------------------------------------------

// loadMonitor fetches within scope and writes errors on failure.
func (h *MonitorHandler) loadMonitor(w http.ResponseWriter, r *http.Request) (*domain.Monitor, bool) {
	scope, ok := ProjectScopeFrom(r.Context())
	if !ok {
		WriteInternal(w, h.Log, errScopeMissing, "scope middleware")
		return nil, false
	}
	monID, ok := parseUUID(r.PathValue("monitorID"))
	if !ok {
		WriteError(w, h.Log, http.StatusBadRequest, "bad_request", "Invalid monitor id.", nil)
		return nil, false
	}
	mon, err := h.All.Monitors.Get(r.Context(), scope.OrgID, monID)
	if err != nil {
		if !WriteStoreError(w, h.Log, err) {
			WriteInternal(w, h.Log, err, "load monitor")
		}
		return nil, false
	}
	return mon, true
}

// validate applies payload-level rules; field-level rules validateFields.
func (h *MonitorHandler) validate(p *monitorPayload) map[string]any {
	errs := map[string]any{}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > maxNameLen {
		errs["name"] = "1..100 characters required"
	}
	p.URL = strings.TrimSpace(p.URL)
	if _, err := safety.ValidateURL(p.URL); err != nil || len(p.URL) > maxURLLen {
		errs["url"] = "must be a public http(s) URL (private and loopback targets are blocked)"
	}
	switch p.Type {
	case string(domain.MonitorTypeHTTP):
	case "":
		p.Type = string(domain.MonitorTypeHTTP)
	default:
		// Keyword/SSL/DNS/redirect monitors land in Phase 8.
		errs["monitor_type"] = "currently supported: http"
	}
	if errs2 := h.validateFields(p.IntervalSeconds, p.TimeoutMs, p.FailureThreshold); errs2 != nil {
		for k, v := range errs2 {
			errs[k] = v
		}
	}
	if p.URL != "" {
		// URL already validated in validate(); nothing further.
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func (h *MonitorHandler) validateFields(interval, timeoutMs, threshold int) map[string]any {
	if interval != 0 && (interval < minInterval || interval > maxInterval) {
		return map[string]any{"interval_seconds": "must be between 60 and 86400"}
	}
	if timeoutMs != 0 && (timeoutMs < 1000 || timeoutMs > maxTimeoutMs) {
		return map[string]any{"timeout_ms": "must be between 1000 and 30000"}
	}
	if threshold != 0 && (threshold < 1 || threshold > 20) {
		return map[string]any{"failure_threshold": "must be between 1 and 20"}
	}
	return nil
}

func applyOptionalConfigs(mon *domain.Monitor, p monitorPayload) {
	if p.RequestConfig != nil {
		mon.RequestConfig = *p.RequestConfig
	}
	if p.CheckConfig != nil {
		mon.CheckConfig = *p.CheckConfig
	}
}

func orDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func scope2(ps projectScope) store.Scope {
	return store.Scope{OrgID: ps.OrgID, ProjectID: ps.ProjectID}
}

func presetWindow(w string) (time.Duration, bool) {
	switch w {
	case "24h", "":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	case "30d":
		return 30 * 24 * time.Hour, true
	case "90d":
		return 90 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func queryInt(r *http.Request, key string, def int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
