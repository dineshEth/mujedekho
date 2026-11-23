package safety

import (
	"net/netip"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		// loopback
		"127.0.0.1", "127.255.255.254", "127.0.0.0",
		// private
		"10.1.2.3", "172.16.0.1", "172.31.255.254", "192.168.1.1",
		// link-local and cloud metadata
		"169.254.169.254", "169.254.0.1",
		// this network, CGNAT, benchmarking, docs, multicast, reserved
		"0.0.0.0", "0.1.2.3", "100.64.0.1", "198.18.0.1", "198.19.255.255",
		"192.0.2.9", "198.51.100.7", "203.0.113.7", "192.0.0.1", "192.88.99.1",
		"224.0.0.1", "239.255.255.255", "240.0.0.1", "255.255.255.255",
		// IPv6
		"::1", "::", "fe80::1", "fc00::1", "fd12:3456::1", "ff02::1",
		"2001:db8::1", "2002::1", "2001::1", "100::1", "64:ff9b::7f00:1",
		// IPv4-mapped IPv6 smuggling
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:169.254.169.254",
	}
	for _, s := range blocked {
		t.Run("blocked/"+s, func(t *testing.T) {
			a := netip.MustParseAddr(s)
			if !IsBlockedIP(a) {
				t.Fatalf("%s must be blocked", s)
			}
		})
	}

	allowed := []string{
		"8.8.8.8", "1.1.1.1", "93.184.216.34",
		"172.32.0.1",  // just above the private /12
		"100.128.0.1", // just above CGNAT /10
		"2606:4700:4700::1111", "2404:6800:4003::1",
	}
	for _, s := range allowed {
		t.Run("allowed/"+s, func(t *testing.T) {
			a := netip.MustParseAddr(s)
			if IsBlockedIP(a) {
				t.Fatalf("%s must not be blocked", s)
			}
		})
	}
}

func TestValidateIPInvalid(t *testing.T) {
	if err := ValidateIP(netip.Addr{}); err == nil {
		t.Fatal("invalid (zero) addr must be rejected")
	}
}
