package httpchecker

// ErrorClass is the coarse taxonomy recorded with every check result. Values
// map to monitoring_checks.error_class.
type ErrorClass string

const (
	ErrorNone            ErrorClass = "none"
	ErrorDNS             ErrorClass = "dns_error"
	ErrorConnect         ErrorClass = "connect_error"
	ErrorTLS             ErrorClass = "tls_error"
	ErrorTimeout         ErrorClass = "timeout"
	ErrorHTTP            ErrorClass = "http_error" // got a response outside expected range
	ErrorAssertion       ErrorClass = "assertion_failed"
	ErrorRedirect        ErrorClass = "redirect_error"
	ErrorBodyTooLarge    ErrorClass = "response_too_large"
	ErrorInvalidResponse ErrorClass = "invalid_response"
	ErrorInternal        ErrorClass = "internal_error"
	ErrorBlocked         ErrorClass = "blocked_target"
)

// Status is the check outcome stored in monitoring_checks.status.
type Status string

const (
	StatusUp      Status = "up"
	StatusDown    Status = "down"
	StatusTimeout Status = "timeout"
)

// Timings carries the decomposed latency of a check, in milliseconds. Fields
// that never activated point to 0 (e.g. no TLS handshake for plain http, or
// sub-millisecond local hops).
type Timings struct {
	DNSMs     int64 `json:"dns_ms"`
	ConnectMs int64 `json:"connect_ms"`
	TLSMs     int64 `json:"tls_ms"`
	TTFBMs    int64 `json:"ttfb_ms"`
	TotalMs   int64 `json:"total_ms"`
}

// Result is the raw output of one HTTP check.
type Result struct {
	Status         Status
	ErrorClass     ErrorClass
	ErrorDetail    string // sanitized; safe to show to customers
	HTTPStatusCode int
	Timings        Timings
	ResponseBytes  int64
	ResponseBody   []byte // only when the caller requested body capture
	FinalURL       string
	RedirectChain  []string // empty when no redirects followed
}
