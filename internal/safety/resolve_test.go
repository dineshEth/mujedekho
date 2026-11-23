package safety

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"
)

// fakeResolver answers lookups from a canned map, simulating DNS behavior
// including rebinding attacks.
type fakeResolver struct {
	answers map[string][]netip.Addr
	err     error
}

func (f *fakeResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	if f.err != nil {
		return nil, f.err
	}
	if a, ok := f.answers[host]; ok {
		return a, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func p(s string) netip.Addr { return netip.MustParseAddr(s) }

func TestResolveAllRejectsPrivateAnswer(t *testing.T) {
	r := &fakeResolver{answers: map[string][]netip.Addr{
		"public.example.com":   {p("93.184.216.34")},
		"rebind.example.com":   {p("93.184.216.34"), p("10.0.0.5")},
		"internal.example.com": {p("192.168.0.9")},
	}}
	v := NewValidator(nil, time.Second, WithResolver(r))
	if _, err := v.ResolveAll(context.Background(), "public.example.com"); err != nil {
		t.Fatalf("public answer must pass: %v", err)
	}
	if _, err := v.ResolveAll(context.Background(), "rebind.example.com"); err == nil {
		t.Fatal("mixed answers containing a private IP must be rejected")
	}
	if _, err := v.ResolveAll(context.Background(), "internal.example.com"); err == nil {
		t.Fatal("all-private answers must be rejected")
	}
}

func TestResolveAllIPLiteral(t *testing.T) {
	v := NewValidator(nil, time.Second)
	if _, err := v.ResolveAll(context.Background(), "8.8.8.8"); err != nil {
		t.Fatalf("public literal must pass: %v", err)
	}
	if _, err := v.ResolveAll(context.Background(), "10.9.9.9"); err == nil {
		t.Fatal("private literal must be rejected")
	}
	if _, err := v.ResolveAll(context.Background(), "2130706433"); err == nil {
		t.Fatal("etc-form numeric host must not resolve to a connectable loopback; NXDOMAIN or blocked expected")
	}
}

func TestDialContextBlocksLoopbackWhenNotPermitted(t *testing.T) {
	v := NewValidator(nil, 500*time.Millisecond)
	_, err := v.dialTransportContext(context.Background(), "tcp", "127.0.0.1:1")
	if err == nil {
		t.Fatal("dialing loopback must be blocked by default")
	}
}
