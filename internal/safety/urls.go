package safety

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// blockedHostNames are hostnames that must never be probed even though they
// are syntactically ordinary names. Resolution-based defenses catch most, but
// rejecting them early makes intent explicit and avoids resolver load.
var blockedHostNames = map[string]bool{
	"localhost":                true,
	"metadata.google.internal": true,
	"instance-data":            true,
}

// blockedHostSuffixes applies to every super-domain of the requested host.
var blockedHostSuffixes = []string{
	".localhost",
	".internal.",
	".internal",
	".local",
	".localdomain",
	".home.arpa",
}

// ValidateURL performs all static, pre-network checks on a customer-supplied
// URL with the strict default rules.
func ValidateURL(raw string) (*url.URL, error) {
	return validateURLWithIPCheck(raw, ValidateIP)
}

// ValidateURL is the Validator-scoped variant: identical static rules, but
// IP-literal checks honor the validator's options (test-only overrides).
func (v *Validator) ValidateURL(raw string) (*url.URL, error) {
	return validateURLWithIPCheck(raw, v.validateIP)
}

func validateURLWithIPCheck(raw string, ipCheck func(netip.Addr) error) (*url.URL, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: empty url", ErrBlocked)
	}
	if strings.ContainsAny(raw, "\x00\r\n\t") {
		return nil, fmt.Errorf("%w: url contains control characters", ErrBlocked)
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: not a valid url", ErrBlocked)
	}
	if u.Opaque != "" {
		return nil, fmt.Errorf("%w: non-hierarchical url", ErrBlocked)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: only http and https schemes are allowed", ErrBlocked)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: credentials in url are not allowed", ErrBlocked)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%w: missing host", ErrBlocked)
	}
	if u.Port() == "0" {
		return nil, fmt.Errorf("%w: invalid port", ErrBlocked)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("%w: missing host", ErrBlocked)
	}
	if strings.Contains(host, "%") {
		return nil, fmt.Errorf("%w: scoped addresses are not allowed", ErrBlocked)
	}
	lower := strings.ToLower(strings.TrimSuffix(host, "."))
	if blockedHostNames[lower] {
		return nil, fmt.Errorf("%w: host is blocked", ErrBlocked)
	}
	for _, sfx := range blockedHostSuffixes {
		if strings.HasSuffix(lower, sfx) {
			return nil, fmt.Errorf("%w: host is in a blocked domain", ErrBlocked)
		}
	}
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if err := ipCheck(addr); err != nil {
			return nil, err
		}
	}
	// Otherwise it is a hostname; resolution defenses apply at dial time.
	return u, nil
}

// ValidateHeaderValue rejects header names/values that could smuggle a second
// request line or crlf-split an outbound request.
func ValidateHeaderValue(name, value string) error {
	if name == "" || strings.ContainsAny(name, "\r\n\x00 ") {
		return fmt.Errorf("%w: invalid header name", ErrBlocked)
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%w: header value contains control characters", ErrBlocked)
	}
	return nil
}
