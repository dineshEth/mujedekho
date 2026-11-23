// Package safety implements the defenses that make customer-supplied URLs safe
// to probe: exhaustive IP blocklists, strict URL validation, DNS resolution
// with per-answer validation, and a dialer that re-validates the address
// actually dialed, closing the DNS-rebinding time-of-check/time-of-use gap.
package safety

import (
	"errors"
	"net/netip"
)

// ErrBlocked is returned when a URL, host or IP is rejected by the safety
// rules. Callers must return only a generic classification to customers; the
// detailed reason stays in internal logs.
var ErrBlocked = errors.New("safety: destination is not allowed")

var blockedV4 = []netip.Prefix{
	mustPrefix("0.0.0.0/8"),       // "this network", includes 0.0.0.0
	mustPrefix("10.0.0.0/8"),      // RFC 1918 private
	mustPrefix("100.64.0.0/10"),   // RFC 6598 carrier-grade NAT
	mustPrefix("127.0.0.0/8"),     // loopback
	mustPrefix("169.254.0.0/16"),  // link-local, includes cloud metadata 169.254.169.254
	mustPrefix("172.16.0.0/12"),   // RFC 1918 private
	mustPrefix("192.0.0.0/24"),    // IETF protocol assignments
	mustPrefix("192.0.2.0/24"),    // TEST-NET-1
	mustPrefix("192.88.99.0/24"),  // 6to4 relay anycast (deprecated by RFC 7526)
	mustPrefix("192.168.0.0/16"),  // RFC 1918 private
	mustPrefix("198.18.0.0/15"),   // benchmarking
	mustPrefix("198.51.100.0/24"), // TEST-NET-2
	mustPrefix("203.0.113.0/24"),  // TEST-NET-3
	mustPrefix("224.0.0.0/4"),     // multicast
	mustPrefix("240.0.0.0/4"),     // reserved, includes 255.255.255.255 broadcast
}

var blockedV6 = []netip.Prefix{
	mustPrefix("::/128"),        // unspecified
	mustPrefix("::1/128"),       // loopback
	mustPrefix("::/96"),         // IPv4-compatible (deprecated)
	mustPrefix("64:ff9b::/96"),  // NAT64 well-known prefix
	mustPrefix("100::/64"),      // discard-only
	mustPrefix("2001::/32"),     // Teredo
	mustPrefix("2001:db8::/32"), // documentation
	mustPrefix("2002::/16"),     // 6to4
	mustPrefix("fc00::/7"),      // unique local
	mustPrefix("fe80::/10"),     // link-local
	mustPrefix("ff00::/8"),      // multicast
}

func mustPrefix(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic("safety: invalid builtin prefix " + s)
	}
	return p
}

// IsBlockedIP reports whether the address falls in any blocklisted range.
// IPv4-mapped IPv6 addresses are checked after unmapping so an attacker cannot
// smuggle a private IPv4 target through its IPv6 representation.
func IsBlockedIP(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	if addr.Zone() != "" {
		return true
	}
	addr = addr.Unmap()
	if addr.Is4() || addr.Is4In6() {
		for _, p := range blockedV4 {
			if p.Contains(addr) {
				return true
			}
		}
		return false
	}
	for _, p := range blockedV6 {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ValidateIP returns nil if the address is public and unicast.
func ValidateIP(addr netip.Addr) error {
	if IsBlockedIP(addr) {
		return ErrBlocked
	}
	return nil
}
