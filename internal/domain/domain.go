// Package domain holds the core entities and pure business rules of the
// platform. It depends on nothing outside the standard library; stores and
// APIs map to and from these types.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// MonitorType selects which parts of RequestConfig/CheckConfig apply.
type MonitorType string

const (
	MonitorTypeHTTP     MonitorType = "http"
	MonitorTypeKeyword  MonitorType = "keyword"
	MonitorTypeSSL      MonitorType = "ssl"
	MonitorTypeDNS      MonitorType = "dns"
	MonitorTypeRedirect MonitorType = "redirect"
)

// MonitorState is the customer-visible state of a monitor.
type MonitorState string

const (
	MonitorStateUp     MonitorState = "up"
	MonitorStateDown   MonitorState = "down"
	MonitorStatePaused MonitorState = "paused"
)

// IncidentStatus is the lifecycle of an incident.
type IncidentStatus string

const (
	IncidentOpen     IncidentStatus = "open"
	IncidentResolved IncidentStatus = "resolved"
)

// RequestConfig configures the outbound HTTP request.
type RequestConfig struct {
	Method            string            `json:"method,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	Body              string            `json:"body,omitempty"`
	ExpectedStatusMin int               `json:"expected_status_min,omitempty"`
	ExpectedStatusMax int               `json:"expected_status_max,omitempty"`
	FollowRedirects   *bool             `json:"follow_redirects,omitempty"`
}

// CheckConfig is the per-type union of check-specific settings. Only fields
// matching the monitor type are meaningful; the API validates the
// intersection.
type CheckConfig struct {
	Keyword                 string `json:"keyword,omitempty"`
	KeywordCondition        string `json:"keyword_condition,omitempty"` // contains | not_contains
	SSLExpiryWarnDays       int    `json:"ssl_expiry_warn_days,omitempty"`
	DNSRecordType           string `json:"record_type,omitempty"`
	AllowedRedirectCount    int    `json:"allowed_redirect_count,omitempty"`
	AllowedRedirectTarget   string `json:"allowed_redirect_target,omitempty"`
	ResponseTimeThresholdMs int    `json:"response_time_threshold_ms,omitempty"`
}

// Monitor is a registered check target.
type Monitor struct {
	ID                  uuid.UUID     `json:"id"`
	OrgID               uuid.UUID     `json:"org_id"`
	ProjectID           uuid.UUID     `json:"project_id"`
	Name                string        `json:"name"`
	Type                MonitorType   `json:"monitor_type"`
	URL                 string        `json:"url"`
	RequestConfig       RequestConfig `json:"request_config"`
	CheckConfig         CheckConfig   `json:"check_config"`
	IntervalSeconds     int           `json:"interval_seconds"`
	TimeoutMs           int           `json:"timeout_ms"`
	FailureThreshold    int           `json:"failure_threshold"`
	State               MonitorState  `json:"state"`
	Deleted             bool          `json:"-"`
	ConsecutiveFailures int           `json:"consecutive_failures,omitempty"`
	CreatedBy           uuid.UUID     `json:"created_by"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
}

// User is an authenticated account.
type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	FullName     string    `json:"full_name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Role is an organization-scoped role.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

// roleRank orders roles by privilege; higher wins.
var roleRank = map[Role]int{RoleViewer: 1, RoleEditor: 2, RoleAdmin: 3, RoleOwner: 4}

// Satisfies reports whether the holder's role is at least the requested one.
func (r Role) Satisfies(min Role) bool {
	return roleRank[r] >= roleRank[min]
}

// Valid roles are constrained here so stores can trust ParseRole.
func ValidRole(r string) bool {
	_, ok := roleRank[Role(r)]
	return ok
}

// Organization is a tenant and billing boundary.
type Organization struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// Membership links a user to an organization with a role.
type Membership struct {
	OrgID     uuid.UUID `json:"org_id"`
	UserID    uuid.UUID `json:"user_id"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// Project groups monitors inside an organization.
type Project struct {
	ID        uuid.UUID `json:"id"`
	OrgID     uuid.UUID `json:"org_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
