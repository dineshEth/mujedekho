// Package httpchecker performs SSRF-guarded HTTP checks with decomposed
// latency measurement (DNS, connect, TLS, TTFB, total) and classified errors.
package httpchecker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"site24x7/internal/safety"
)

// Config is a single check request. It is a pure value type so it can come
// from the monitors table or a manual-test API call.
type Config struct {
	URL               string
	Method            string
	Headers           map[string]string
	Body              string
	FollowRedirects   bool
	MaxRedirects      int // default 5, max 10
	ExpectedStatusMin int // default 200
	ExpectedStatusMax int // default 299
	Timeout           time.Duration
	MaxBodyBytes      int64 // read cap for any response body, default 1MiB
	CaptureBody       bool  // populate Result.ResponseBody (bounded by MaxBodyBytes)
	UserAgent         string
}

const (
	DefaultMethod       = "GET"
	defaultTimeout      = 10 * time.Second
	maxTimeout          = 30 * time.Second
	defaultMaxRedirects = 5
	maxMaxRedirects     = 10
	defaultMaxBodyBytes = int64(1024 * 1024)
	DefaultUserAgent    = "Site24x7-Website-Monitor/1.0"
	defaultStatusMax    = 299
	defaultStatusMin    = 200
)

// Checker performs HTTP checks through a safety.Validator-guarded transport.
type Checker struct {
	validator *safety.Validator
}

func New(v *safety.Validator) *Checker { return &Checker{validator: v} }

// timers accumulates per-check latency measurements. Safe for single-check
// use; each Check allocates its own and never touches it concurrently because
// dial and RoundTrip for one logical check complete in sequence per hop.
type timers struct {
	dnsMs     int64
	connectMs int64
	tlsMs     int64
	ttfbMs    int64
}

func (c *Checker) normalize(cfg *Config) error {
	if cfg.Method == "" {
		cfg.Method = DefaultMethod
	}
	cfg.Method = strings.ToUpper(cfg.Method)
	switch cfg.Method {
	case "GET", "HEAD", "POST", "PUT", "OPTIONS", "PATCH", "DELETE":
	default:
		return fmt.Errorf("unsupported method %q", cfg.Method)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Timeout > maxTimeout {
		cfg.Timeout = maxTimeout
	}
	if cfg.MaxRedirects <= 0 {
		cfg.MaxRedirects = defaultMaxRedirects
	}
	if cfg.MaxRedirects > maxMaxRedirects {
		cfg.MaxRedirects = maxMaxRedirects
	}
	if cfg.ExpectedStatusMin == 0 {
		cfg.ExpectedStatusMin = defaultStatusMin
	}
	if cfg.ExpectedStatusMax == 0 {
		cfg.ExpectedStatusMax = defaultStatusMax
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	return nil
}

func (c *Checker) Check(ctx context.Context, cfg Config) Result {
	start := time.Now()
	var res Result
	if err := c.normalize(&cfg); err != nil {
		return Result{Status: StatusDown, ErrorClass: ErrorInternal, ErrorDetail: err.Error(),
			Timings: Timings{TotalMs: msSince(start)}}
	}

	// Static validation happens before any network contact.
	u, uerr := c.validator.ValidateURL(cfg.URL)
	if uerr != nil {
		return Result{Status: StatusDown, ErrorClass: ErrorBlocked, ErrorDetail: "URL is not allowed",
			Timings: Timings{TotalMs: msSince(start)}}
	}

	// Explicit, measured, validated resolution. The Validator dialer repeats
	// resolution itself at dial time and rejects blocklisted answers, so this
	// measurement does not create a time-of-check/time-of-use window.
	dnsStart := time.Now()
	_, dnsErr := c.validator.ResolveAll(ctx, u.Hostname())
	res.Timings.DNSMs = msSince(dnsStart)
	if dnsErr != nil {
		var dnsX *net.DNSError
		switch {
		case errors.As(dnsErr, &dnsX):
			return Result{Status: StatusDown, ErrorClass: ErrorDNS, ErrorDetail: "DNS resolution failed",
				Timings: res.Timings}
		case errors.Is(dnsErr, safety.ErrBlocked):
			return Result{Status: StatusDown, ErrorClass: ErrorBlocked, ErrorDetail: "target resolves to a blocked address",
				Timings: res.Timings}
		default:
			return Result{Status: StatusDown, ErrorClass: ErrorInternal, ErrorDetail: "DNS lookup error",
				Timings: res.Timings}
		}
	}

	t := &timers{}
	redirectChain := make([]string, 0, cfg.MaxRedirects)
	transport := c.validator.SafeTransport(cfg.Timeout)
	transport.DialTLSContext = c.tlsDialer(t)
	transport.DialContext = c.observedDialer(transport.DialContext, t)
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: &timingRT{rt: transport, t: t},
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !cfg.FollowRedirects {
			return http.ErrUseLastResponse
		}
		if len(via) > cfg.MaxRedirects {
			return fmt.Errorf("stopped after %d redirects", cfg.MaxRedirects)
		}
		if err := c.validator.ValidateRedirect(req.URL.String()); err != nil {
			return fmt.Errorf("redirect target rejected")
		}
		redirectChain = append(redirectChain, safeHost(req.URL.String()))
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, cfg.Method, u.String(), bodyReader(cfg))
	if err != nil {
		return Result{Status: StatusDown, ErrorClass: ErrorInternal, ErrorDetail: "could not build request",
			Timings: Timings{DNSMs: res.Timings.DNSMs, TotalMs: msSince(start)}}
	}
	req.ContentLength = int64(len(cfg.Body))
	req.Header.Set("User-Agent", cfg.UserAgent)
	for k, v := range cfg.Headers {
		if err := safety.ValidateHeaderValue(k, v); err != nil {
			continue // invalid customer headers are dropped, never smuggled
		}
		req.Header.Set(k, v)
	}

	resp, reqErr := client.Do(req)
	if reqErr != nil {
		res.Timings.TotalMs = msSince(start)
		return c.classifyTransportError(res.Timings, reqErr)
	}
	defer func() { _ = resp.Body.Close() }()

	total := msSince(start)
	ttfb := clampMs(t.ttfbMs)

	status := StatusUp
	class := ErrorNone
	detail := ""
	if resp.StatusCode < cfg.ExpectedStatusMin || resp.StatusCode > cfg.ExpectedStatusMax {
		status = StatusDown
		class = ErrorHTTP
		detail = fmt.Sprintf("HTTP %d outside expected range %d-%d", resp.StatusCode, cfg.ExpectedStatusMin, cfg.ExpectedStatusMax)
	}

	bodyBytes, body, tooLarge := c.readBody(resp, &cfg)
	resp.Body.Close()
	if tooLarge {
		status = StatusDown
		class = ErrorBodyTooLarge
		detail = "response body exceeded the size limit"
	}

	res.HTTPStatusCode = resp.StatusCode
	res.Status = status
	res.ErrorClass = class
	res.ErrorDetail = detail
	res.ResponseBody = body
	res.ResponseBytes = bodyBytes
	res.FinalURL = ""
	if resp.Request != nil && resp.Request.URL != nil {
		res.FinalURL = resp.Request.URL.String()
	}
	res.RedirectChain = redirectChain
	res.Timings.ConnectMs = clampMs(t.connectMs)
	res.Timings.TLSMs = clampMs(t.tlsMs)
	res.Timings.TTFBMs = ttfb
	res.Timings.TotalMs = total
	return res
}

func bodyReader(cfg Config) io.Reader {
	if cfg.Body == "" {
		return nil
	}
	return strings.NewReader(cfg.Body)
}

// readBody reads up to MaxBodyBytes, reporting whether the limit was exceeded.
func (c *Checker) readBody(resp *http.Response, cfg *Config) (int64, []byte, bool) {
	if cfg.CaptureBody {
		buf, err := io.ReadAll(io.LimitReader(resp.Body, cfg.MaxBodyBytes+1))
		if err != nil {
			return int64(len(buf)), buf, false
		}
		if int64(len(buf)) > cfg.MaxBodyBytes {
			return int64(len(buf)), buf[:cfg.MaxBodyBytes], true
		}
		return int64(len(buf)), buf, false
	}
	n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, cfg.MaxBodyBytes+1))
	return n, nil, n > cfg.MaxBodyBytes
}

// observedDialer wraps the transport's safe DialContext to record connect
// completion times without bypassing validation.
func (c *Checker) observedDialer(next func(context.Context, string, string) (net.Conn, error), t *timers) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialStart := time.Now()
		conn, err := next(ctx, network, addr)
		if err == nil {
			t.connectMs = time.Since(dialStart).Milliseconds() - t.tlsMs
			if t.connectMs < 0 {
				t.connectMs = 0
			}
		}
		return conn, err
	}
}

// tlsDialer performs the TLS handshake explicitly so the checker can measure
// it; certificate verification stays standard against the request hostname.
func (c *Checker) tlsDialer(t *timers) func(context.Context, string, string) (net.Conn, error) {
	v := c.validator
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, connectMs, tlsMs, err := v.DialTLSWithTiming(ctx, network, addr)
		if err == nil {
			t.connectMs = connectMs
			t.tlsMs = tlsMs
		}
		return conn, err
	}
}

// timingRT records TTFB for each round trip (headers arrival for the final
// request during redirects).
type timingRT struct {
	rt http.RoundTripper
	t  *timers
}

func (r *timingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	hopStart := time.Now()
	resp, err := r.rt.RoundTrip(req)
	if err == nil {
		ms := time.Since(hopStart).Milliseconds() - r.t.connectMs - r.t.tlsMs
		if ms > 0 {
			r.t.ttfbMs = ms
		}
	}
	return resp, err
}

func (c *Checker) classifyTransportError(t Timings, err error) Result {
	res := Result{Status: StatusDown, Timings: t}
	msg := err.Error()
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		msg = urlErr.Err.Error()
	}
	lower := strings.ToLower(msg)
	switch {
	case errors.Is(err, context.DeadlineExceeded),
		strings.Contains(lower, "client.timeout exceeded"),
		strings.Contains(lower, "i/o timeout"):
		res.ErrorClass = ErrorTimeout
		res.ErrorDetail = "request timed out"
	case errors.Is(err, safety.ErrBlocked):
		res.ErrorClass = ErrorBlocked
		res.ErrorDetail = "target is a blocked address"
	case containsAny(lower, "no such host", "nxdomain", "lookup failed", "server misbehaving"):
		res.ErrorClass = ErrorDNS
		res.ErrorDetail = "DNS resolution failed"
	case containsAny(lower, "tls:", "x509:", "handshake failure", "certificate"):
		res.ErrorClass = ErrorTLS
		res.ErrorDetail = tlsDetail(lower)
	case containsAny(lower, "connection refused", "no route to host", "network is unreachable",
		"connection reset", "use of closed network connection", "concurrent connect"):
		res.ErrorClass = ErrorConnect
		res.ErrorDetail = "connection failed"
	case containsAny(lower, "stopped after", "too many redirects"):
		res.ErrorClass = ErrorRedirect
		res.ErrorDetail = "too many redirects"
	default:
		res.ErrorClass = ErrorConnect
		res.ErrorDetail = "request failed"
	}
	return res
}

func tlsDetail(lower string) string {
	switch {
	case strings.Contains(lower, "expired"):
		return "certificate expired"
	case strings.Contains(lower, "not valid for"):
		return "certificate hostname mismatch"
	case strings.Contains(lower, "unknown authority"):
		return "certificate signed by unknown authority"
	default:
		return "TLS handshake failed"
	}
}

func containsAny(s string, subs ...string) bool {
	return slices.ContainsFunc(subs, func(sub string) bool { return strings.Contains(s, sub) })
}

func clampMs(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func msSince(t time.Time) int64 {
	return time.Since(t).Milliseconds()
}

// safeHost strips userinfo from a URL string for the redirect chain record.
func safeHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.User = nil
	return u.String()
}
