package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"site24x7/internal/domain"
	"site24x7/internal/store"
)

func storeJob(mon *domain.Monitor) store.Job {
	return store.Job{Monitor: mon, ScheduledAt: time.Now().UTC()}
}

func testMonitor(timeoutMs int) *domain.Monitor {
	return &domain.Monitor{
		ID:               uuid.New(),
		IntervalSeconds:  60,
		TimeoutMs:        timeoutMs,
		FailureThreshold: 3,
		State:            domain.MonitorStateUp,
	}
}

// fakeProbe returns a scripted result and records concurrency.
type fakeProbe struct {
	mu          sync.Mutex
	inflight    int
	maxInflight int
	calls       int
	delay       time.Duration
	result      domain.CheckResult
}

func (f *fakeProbe) Probe(ctx context.Context, _ *domain.Monitor) domain.CheckResult {
	f.mu.Lock()
	f.inflight++
	if f.inflight > f.maxInflight {
		f.maxInflight = f.inflight
	}
	f.calls++
	f.mu.Unlock()

	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
		}
	}

	f.mu.Lock()
	f.inflight--
	f.mu.Unlock()
	return f.result
}

func (f *fakeProbe) stats() (inflightNow, maxInflight, calls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inflight, f.maxInflight, f.calls
}

// sequenceProbe returns scripted results in order; when exhausted it repeats
// the last one.
type sequenceProbe struct {
	mu      sync.Mutex
	results []domain.CheckResult
	i       int
	callsN  int
}

func (s *sequenceProbe) Probe(context.Context, *domain.Monitor) domain.CheckResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.i < len(s.results) {
		r := s.results[s.i]
		s.i++
		s.callsN++
		return r
	}
	s.callsN++
	return s.results[len(s.results)-1]
}

func (s *sequenceProbe) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callsN
}

// recorderProcessor records processed results.
type recorderProcessor struct {
	mu      sync.Mutex
	results []domain.CheckResult
}

func (r *recorderProcessor) Process(_ context.Context, _ *domain.Monitor, res domain.CheckResult, _ time.Time) error {
	r.mu.Lock()
	r.results = append(r.results, res)
	r.mu.Unlock()
	return nil
}

func (r *recorderProcessor) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.results)
}

func (r *recorderProcessor) lastStatus() domain.CheckStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.results) == 0 {
		return ""
	}
	return r.results[len(r.results)-1].Status
}

func noBackoff(int) time.Duration { return 0 }

// TestPoolBoundedConcurrency: queued jobs processed with maxConcurrency never
// exceed the pool width in flight.
func TestPoolBoundedConcurrency(t *testing.T) {
	const width, depth = 3, 8
	mon := testMonitor(500)
	probe := &fakeProbe{delay: 20 * time.Millisecond, result: domain.CheckResult{Status: domain.CheckStatusUp}}
	proc := &recorderProcessor{}

	p := NewPool(width, depth, nil, probe, proc, noBackoff, nil)
	p.Start(context.Background())

	for i := 0; i < depth; i++ {
		if !p.Submit(storeJob(mon)) {
			t.Fatalf("submit %d rejected before Stop", i)
		}
	}
	p.Stop()

	if _, maxInf, calls := probe.stats(); maxInf > width || calls < depth {
		t.Fatalf("maxInflight=%d calls=%d; want maxInflight<=%d calls>=%d", maxInf, calls, width, depth)
	}
	if proc.count() != depth {
		t.Fatalf("processed %d results, want %d", proc.count(), depth)
	}
}

// TestPoolSubmitAfterStop: Submit returns false after Stop.
func TestPoolSubmitAfterStop(t *testing.T) {
	p := NewPool(1, 1, nil, &fakeProbe{}, &recorderProcessor{}, noBackoff, nil)
	p.Start(context.Background())
	p.Stop()
	if p.Submit(storeJob(testMonitor(100))) {
		t.Fatal("Submit after Stop returned true")
	}
}

// TestPoolRetryOnce: a transient (connect error) result is re-probed once and
// the retried result — not the original — reaches the processor.
func TestPoolRetryOnce(t *testing.T) {
	mon := testMonitor(500)
	probe := &sequenceProbe{
		results: []domain.CheckResult{
			{Status: domain.CheckStatusDown, ErrorClass: domain.CheckErrorConnect},
			{Status: domain.CheckStatusUp, ErrorClass: domain.CheckErrorNone},
		},
	}
	proc := &recorderProcessor{}

	p := NewPool(1, 1, nil, probe, proc, func(int) time.Duration { return time.Millisecond }, nil)
	p.Start(context.Background())
	p.Submit(storeJob(mon))
	p.Stop()

	if got := probe.calls(); got != 2 {
		t.Fatalf("probe calls = %d, want 2 (one retry)", got)
	}
	if proc.count() != 1 {
		t.Fatalf("processor calls = %d, want 1", proc.count())
	}
	if got := proc.lastStatus(); got != domain.CheckStatusUp {
		t.Fatalf("processed status = %q, want up (retry result wins)", got)
	}
}

// TestPoolNoRetryOnDefinitiveResult: an HTTP 500 is not retried.
func TestPoolNoRetryOnDefinitiveResult(t *testing.T) {
	mon := testMonitor(500)
	probe := &sequenceProbe{results: []domain.CheckResult{
		{Status: domain.CheckStatusDown, ErrorClass: domain.CheckErrorHTTP, HTTPStatusCode: 500},
	}}
	proc := &recorderProcessor{}

	p := NewPool(1, 1, nil, probe, proc, noBackoff, nil)
	p.Start(context.Background())
	p.Submit(storeJob(mon))
	p.Stop()

	if got := probe.calls(); got != 1 {
		t.Fatalf("probe calls = %d, want 1 (no retry for http_error)", got)
	}
}

// TestRetryable: classification table for the retry policy.
func TestRetryable(t *testing.T) {
	cases := []struct {
		class domain.CheckErrorClass
		want  bool
	}{
		{domain.CheckErrorNone, false},
		{domain.CheckErrorHTTP, false},
		{domain.CheckErrorAssertion, false},
		{domain.CheckErrorTimeout, true},
		{domain.CheckErrorDNS, true},
		{domain.CheckErrorConnect, true},
		{domain.CheckErrorTLS, false}, // TLS failures are usually deterministic
	}
	for _, tc := range cases {
		if got := retryable(domain.CheckResult{Status: domain.CheckStatusDown, ErrorClass: tc.class}); got != tc.want {
			t.Errorf("retryable(%q) = %v, want %v", tc.class, got, tc.want)
		}
	}
}

// TestPoolDrainsQueueOnStop: Stop lets queued jobs finish.
func TestPoolDrainsQueueOnStop(t *testing.T) {
	const depth = 5
	mon := testMonitor(500)
	probe := &fakeProbe{delay: 5 * time.Millisecond, result: domain.CheckResult{Status: domain.CheckStatusUp}}
	proc := &recorderProcessor{}

	p := NewPool(1, depth, nil, probe, proc, noBackoff, nil)
	p.Start(context.Background())
	for i := 0; i < depth; i++ {
		p.Submit(storeJob(mon))
	}
	p.Stop()

	if proc.count() != depth {
		t.Fatalf("drained %d results, want %d", proc.count(), depth)
	}
}

// Compile-time interface checks.
var (
	_ Probe     = (*fakeProbe)(nil)
	_ Probe     = (*sequenceProbe)(nil)
	_ Processor = (*recorderProcessor)(nil)
)
