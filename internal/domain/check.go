package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CheckStatus is the outcome of a single check.
type CheckStatus string

const (
	CheckStatusUp      CheckStatus = "up"
	CheckStatusDown    CheckStatus = "down"
	CheckStatusTimeout CheckStatus = "timeout"
)

// CheckErrorClass classifies why a check failed. Values match
// internal/checker/http so the worker can convert directly.
type CheckErrorClass string

const (
	CheckErrorNone            CheckErrorClass = "none"
	CheckErrorDNS             CheckErrorClass = "dns_error"
	CheckErrorConnect         CheckErrorClass = "connect_error"
	CheckErrorTLS             CheckErrorClass = "tls_error"
	CheckErrorTimeout         CheckErrorClass = "timeout"
	CheckErrorHTTP            CheckErrorClass = "http_error"
	CheckErrorAssertion       CheckErrorClass = "assertion_failed"
	CheckErrorRedirect        CheckErrorClass = "redirect_error"
	CheckErrorBodyTooLarge    CheckErrorClass = "response_too_large"
	CheckErrorInvalidResponse CheckErrorClass = "invalid_response"
	CheckErrorInternal        CheckErrorClass = "internal_error"
	CheckErrorBlocked         CheckErrorClass = "blocked_target"
)

// CheckResult is one observation of a monitor. This is the shape persisted to
// monitoring_checks and returned by the API.
type CheckResult struct {
	MonitorID      uuid.UUID       `json:"-"`
	CheckedAt      time.Time       `json:"checked_at"`
	Status         CheckStatus     `json:"status"`
	HTTPStatusCode int             `json:"http_status_code,omitempty"`
	DurationMs     int64           `json:"duration_ms"`
	DNSMs          int64           `json:"dns_ms,omitempty"`
	ConnectMs      int64           `json:"connect_ms,omitempty"`
	TLSMs          int64           `json:"tls_ms,omitempty"`
	TTFBMs         int64           `json:"ttfb_ms,omitempty"`
	ErrorClass     CheckErrorClass `json:"error_class"`
	ErrorDetail    string          `json:"error_detail,omitempty"`
	ResponseBytes  int64           `json:"response_bytes"`
	Region         string          `json:"region"`
	Detail         json.RawMessage `json:"detail,omitempty"` // per-type extras (cert info, DNS answers, redirect chain)
}

// Incident is a period of unavailability or degradation for one monitor.
type Incident struct {
	ID              uuid.UUID      `json:"id"`
	MonitorID       uuid.UUID      `json:"monitor_id"`
	Status          IncidentStatus `json:"status"`
	StartedAt       time.Time      `json:"started_at"`
	ResolvedAt      *time.Time     `json:"resolved_at,omitempty"`
	ResolvedSeconds *int64         `json:"resolved_duration_s,omitempty"`
	CauseClass      string         `json:"cause_class,omitempty"`
}

// IncidentEventType enumerates timeline entries.
type IncidentEventType string

const (
	IncidentEventOpened       IncidentEventType = "opened"
	IncidentEventRecovery     IncidentEventType = "recovered"
	IncidentEventNotified     IncidentEventType = "notified"
	IncidentEventNotifyFailed IncidentEventType = "notification_failed"
	IncidentEventAnnotated    IncidentEventType = "annotated"
)

// IncidentEvent is one line of an incident timeline.
type IncidentEvent struct {
	ID         uuid.UUID         `json:"id"`
	IncidentID uuid.UUID         `json:"incident_id"`
	EventType  IncidentEventType `json:"event_type"`
	Payload    json.RawMessage   `json:"payload,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}
