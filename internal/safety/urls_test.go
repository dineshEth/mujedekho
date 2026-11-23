package safety

import (
	"testing"
)

func TestValidateURL(t *testing.T) {
	ok := []string{
		"https://example.com",
		"http://example.com/path?q=1",
		"https://sub.domain.example.co.uk/deep/path",
		"https://93.184.216.34",          // public IP literal
		"https://[2606:4700:4700::1111]", // public IPv6 literal
	}
	for _, raw := range ok {
		t.Run("ok/"+raw, func(t *testing.T) {
			if _, err := ValidateURL(raw); err != nil {
				t.Fatalf("expected %q to be allowed, got %v", raw, err)
			}
		})
	}

	blocked := []string{
		"file:///etc/passwd",
		"ftp://example.com",
		"gopher://example.com",
		"example.com", // no scheme
		"https://user:pass@example.com",
		"https://",
		"https://127.0.0.1",
		"https://10.0.0.1",
		"https://169.254.169.254/latest/meta-data",
		"https://[::1]/",
		"https://[fe80::1%25eth0]/", // zone syntax
		"https://localhost:8080",
		"https://api.internal/x",
		"https://service.local",
		"https://printer.home.arpa",
		"https://metadata.google.internal/computeMetadata/v1/",
	}
	for _, raw := range blocked {
		t.Run("blocked/"+raw, func(t *testing.T) {
			u, err := ValidateURL(raw)
			if err == nil {
				t.Fatalf("expected %q to be rejected, got url %v", raw, u)
			}
		})
	}
}

func TestValidateURLRejectsControlChars(t *testing.T) {
	if _, err := ValidateURL("https://example.com/\r\nX-Injected: 1"); err == nil {
		t.Fatal("CRLF in URL must be rejected")
	}
}

func TestValidateHeaderValue(t *testing.T) {
	if err := ValidateHeaderValue("X-Auth", "Bearer abc"); err != nil {
		t.Fatalf("ordinary header must pass: %v", err)
	}
	for _, v := range []string{"Bearer abc\r\nHost: internal", "abc\nHost: x", "ab\x00c"} {
		if err := ValidateHeaderValue("X-Auth", v); err == nil {
			t.Fatalf("value %q must be rejected", v)
		}
	}
	if err := ValidateHeaderValue("Bad\r\nName", "v"); err == nil {
		t.Fatal("header name with CRLF must be rejected")
	}
}
