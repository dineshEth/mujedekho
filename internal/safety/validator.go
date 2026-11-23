package safety

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// Resolver is the resolve step used by the validator. It matches the subset of
// *net.Resolver that this package needs, so custom resolvers (for tests and
// for pinning queries to configured DNS servers) can be injected.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Validator is the entry point of the safety package. One instance is shared
// by the API (URL validation on monitor create) and the checkers (resolution
// and dialing at check time).
//
// The options exist ONLY to support tests: httptest servers listen on
// 127.0.0.1, which is otherwise always blocked. Never enable them in
// production configuration.
type Validator struct {
	resolver Resolver
	dialer   *net.Dialer

	// allowLoopback lets loopback targets through every check (tests only).
	allowLoopback bool
	// allowPrivate lets all normally blocklisted targets through (tests only).
	allowPrivate bool
}

type Option func(*Validator)

// WithLoopbackAllowed permits loopback addresses. For tests only.
func WithLoopbackAllowed() Option { return func(v *Validator) { v.allowLoopback = true } }

// WithPrivateAllowed permits all normally blocklisted ranges. For tests only.
func WithPrivateAllowed() Option { return func(v *Validator) { v.allowPrivate = true } }

// WithResolver injects a custom resolver (tests only).
func WithResolver(r Resolver) Option { return func(v *Validator) { v.resolver = r } }

// NewValidator builds a validator. dnsServers is optional; when empty the
// system resolver is used. The pure Go resolver is always preferred so that
// behavior is identical across deployments.
func NewValidator(dnsServers []string, timeout time.Duration, opts ...Option) *Validator {
	v := &Validator{
		dialer: &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second},
	}
	if len(dnsServers) > 0 {
		servers := append([]string(nil), dnsServers...)
		v.resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				// Dial targets are replaced with the operator-configured
				// recursive resolvers. Only the first server is used; the
				// address here is operator-supplied, never customer-supplied.
				var d net.Dialer
				return d.DialContext(ctx, network, net.JoinHostPort(servers[0], "53"))
			},
		}
	} else {
		v.resolver = net.DefaultResolver
	}
	for _, o := range opts {
		o(v)
	}
	return v
}

// ResolveAll is the public resolution entry point: resolve a hostname and
// validate every DNS answer, or validate an IP literal host directly. Any
// blocklisted answer rejects the whole lookup, which is the defense against
// rebinding from public to private between checks.
func (v *Validator) ResolveAll(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if err := v.validateIP(ip); err != nil {
			return nil, err
		}
		return []netip.Addr{ip.Unmap()}, nil
	}
	addrs, err := v.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			return nil, dnsErr
		}
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses for %q", host)
	}
	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		if err := v.validateIP(a); err != nil {
			return nil, err
		}
		out = append(out, a.Unmap())
	}
	return out, nil
}

// ValidateRedirect checks a redirect hop target with the same static rules as
// the initial URL before the client follows it.
func (v *Validator) ValidateRedirect(raw string) error {
	_, err := v.ValidateURL(raw)
	return err
}

// validateIP applies the blocklists unless test overrides are active.
func (v *Validator) validateIP(addr netip.Addr) error {
	if v.allowPrivate {
		return nil
	}
	if v.allowLoopback && addr.Unmap().IsLoopback() {
		return nil
	}
	return ValidateIP(addr)
}

// SafeHTTPClient builds an *http.Client whose connections can only reach
// validated public addresses. Every dial re-resolves and re-validates, and a
// per-connection Control callback inspects the address actually handed to the
// kernel, so a rebinding between validation and connect cannot reach a private
// network. The returned cleanup closes idle connections.
func (v *Validator) SafeHTTPClient(timeout time.Duration) (*http.Client, func()) {
	t := v.SafeTransport(timeout)
	return &http.Client{Timeout: timeout, Transport: t}, t.CloseIdleConnections
}

// SafeTransport builds an *http.Transport with the validating dialers. TLS is
// handled by the transport's default path unless the caller overrides
// DialTLSContext (the checkers do, to capture handshake timing).
func (v *Validator) SafeTransport(timeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy:                  nil, // direct egress only
		DialContext:            v.dialTransportContext,
		ForceAttemptHTTP2:      false,
		DisableKeepAlives:      true, // every check measures a full connection
		MaxIdleConns:           1,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  timeout,
		ExpectContinueTimeout:  time.Second,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		MaxResponseHeaderBytes: 64 * 1024,
	}
}

// dialTransportContext dials an unencrypted connection. host may be a name or
// an IP literal; names are resolved with the validating resolver and connected
// only to validated addresses.
func (v *Validator) dialTransportContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, rerr := v.ResolveAll(ctx, host)
	if rerr != nil {
		return nil, rerr
	}
	return v.dialValidated(ctx, network, ips, port)
}

// dialValidated dials the given addresses in order, returning the first
// success. The Control callback re-validates the address actually passed to
// the kernel.
func (v *Validator) dialValidated(ctx context.Context, network string, ips []netip.Addr, port string) (net.Conn, error) {
	var lastErr error
	for _, ip := range ips {
		if err := v.validateIP(ip); err != nil {
			lastErr = err
			continue
		}
		d := *v.dialer
		d.Control = func(_, address string, _ syscall.RawConn) error {
			h, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			a, err := netip.ParseAddr(h)
			if err != nil {
				return err
			}
			return v.validateIP(a)
		}
		conn, err := d.DialContext(ctx, network, ip.String()+":"+port)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrBlocked
	}
	return nil, lastErr
}

// DialTLSWithTiming dials through the validating dialer and performs the TLS
// handshake itself, SNI pinned to the request host, with standard certificate
// verification. It returns connect and handshake durations so checkers can
// decompose latency. connectMs includes the dial-time re-resolution.
func (v *Validator) DialTLSWithTiming(ctx context.Context, network, addr string) (net.Conn, int64, int64, error) {
	start := time.Now()
	conn, err := v.dialTransportContext(ctx, network, addr)
	if err != nil {
		return nil, 0, 0, err
	}
	connectMs := time.Since(start).Milliseconds()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		_ = conn.Close()
		return nil, connectMs, 0, err
	}
	tlsConn := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host})
	hsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	hsStart := time.Now()
	if err := tlsConn.HandshakeContext(hsCtx); err != nil {
		_ = conn.Close()
		return nil, connectMs, 0, err
	}
	return tlsConn, connectMs, time.Since(hsStart).Milliseconds(), nil
}
