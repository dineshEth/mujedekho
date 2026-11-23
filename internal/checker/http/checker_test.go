package httpchecker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"site24x7/internal/safety"
)

func testValidator(t *testing.T) *safety.Validator {
	t.Helper()
	return safety.NewValidator(nil, time.Second, safety.WithLoopbackAllowed())
}

func TestCheckUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello world")
	}))
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{URL: srv.URL, FollowRedirects: true})
	if res.Status != StatusUp {
		t.Fatalf("want up, got %s (%s: %s)", res.Status, res.ErrorClass, res.ErrorDetail)
	}
	if res.HTTPStatusCode != 200 {
		t.Fatalf("want 200, got %d", res.HTTPStatusCode)
	}
	if res.Timings.TTFBMs < 0 {
		t.Fatalf("TTFB must be measured, got %d", res.Timings.TTFBMs)
	}
	if res.ResponseBytes != int64(len("hello world")) {
		t.Fatalf("want %d bytes, got %d", len("hello world"), res.ResponseBytes)
	}
	if res.Timings.TotalMs < 0 {
		t.Fatalf("total ms invalid: %d", res.Timings.TotalMs)
	}
}

func TestCheckHTTPStatusOutsideExpected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{URL: srv.URL})
	if res.Status != StatusDown || res.ErrorClass != ErrorHTTP {
		t.Fatalf("want down/http_error, got %s/%s", res.Status, res.ErrorClass)
	}
	if !strings.Contains(res.ErrorDetail, "404") {
		t.Fatalf("detail should mention the status code, got %q", res.ErrorDetail)
	}
}

func TestCheckCustomExpectedRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{URL: srv.URL, ExpectedStatusMin: 400, ExpectedStatusMax: 404})
	if res.Status != StatusUp {
		t.Fatalf("custom expected range should pass, got %s: %s", res.Status, res.ErrorDetail)
	}
}

func TestCheckFollowsRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth") != "secret-value" {
			t.Errorf("custom header not forwarded")
		}
		fmt.Fprint(w, "final page")
	})
	mux.Handle("/final", final)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{
		URL:             srv.URL + "/redirect",
		FollowRedirects: true,
		Headers:         map[string]string{"X-Auth": "secret-value"},
	})
	if res.Status != StatusUp {
		t.Fatalf("want up, got %s (%s: %s)", res.Status, res.ErrorClass, res.ErrorDetail)
	}
	if len(res.RedirectChain) != 1 || !strings.HasSuffix(res.RedirectChain[0], "/final") {
		t.Fatalf("redirect chain = %v", res.RedirectChain)
	}
	if !strings.HasSuffix(res.FinalURL, "/final") {
		t.Fatalf("final url = %s", res.FinalURL)
	}
}

// httptest serves on loopback; the test validator permits loopback targets.
func TestCheckRedirectDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/moved")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{URL: srv.URL, FollowRedirects: false})
	if res.HTTPStatusCode != http.StatusFound {
		t.Fatalf("want 302, got %d", res.HTTPStatusCode)
	}
	if res.ErrorClass != ErrorHTTP {
		t.Fatalf("302 outside default expected range must be http_error, got %s", res.ErrorClass)
	}
}

func TestCheckTooManyRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/hop", http.StatusFound)
	}))
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{URL: srv.URL, FollowRedirects: true, MaxRedirects: 3})
	if res.Status != StatusDown {
		t.Fatalf("want down, got %s", res.Status)
	}
	if res.ErrorClass != ErrorRedirect {
		t.Fatalf("want redirect_error, got %s (%s)", res.ErrorClass, res.ErrorDetail)
	}
}

func TestCheckTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		fmt.Fprint(w, "late")
	}))
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{URL: srv.URL, Timeout: 50 * time.Millisecond})
	if res.Status != StatusDown || res.ErrorClass != ErrorTimeout {
		t.Fatalf("want down/timeout, got %s/%s (%s)", res.Status, res.ErrorClass, res.ErrorDetail)
	}
}

func TestCheckBodySizeLimit(t *testing.T) {
	big := strings.Repeat("x", 5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, big)
	}))
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{URL: srv.URL, MaxBodyBytes: 100, CaptureBody: true})
	if res.ErrorClass != ErrorBodyTooLarge {
		t.Fatalf("want response_too_large, got %s", res.ErrorClass)
	}
	if int64(len(res.ResponseBody)) != 100 {
		t.Fatalf("captured body should be truncated to 100 bytes, got %d", len(res.ResponseBody))
	}
	if res.ResponseBytes != 101 {
		t.Fatalf("bytes read is bounded by the limit plus overflow marker, got %d", res.ResponseBytes)
	}
}

func TestCheckDropsMaliciousHeader(t *testing.T) {
	var gotXHostHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXHostHeader = r.Header.Get("X-Injected")
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{
		URL:     srv.URL,
		Headers: map[string]string{"X-Injected": "value\r\nEvil: 1", "X-Good": "fine"},
	})
	if res.Status != StatusUp {
		t.Fatalf("want up, got %s: %s", res.Status, res.ErrorDetail)
	}
	if gotXHostHeader != "" {
		t.Fatalf("malicious header must be dropped, server saw %q", gotXHostHeader)
	}
}

func TestCheckBogusURL(t *testing.T) {
	c := New(testValidator(t))
	for _, raw := range []string{"", "file:///etc/passwd", "https://localhost", "https://10.1.1.1"} {
		res := c.Check(context.Background(), Config{URL: raw})
		if res.Status != StatusDown || res.ErrorClass != ErrorBlocked {
			t.Fatalf("url %q want down/blocked, got %s/%s", raw, res.Status, res.ErrorClass)
		}
	}
}

func TestCheckMethodDefaultAndNormalization(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Method
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{URL: srv.URL, Method: "get"})
	if res.Status != StatusUp || seen != "GET" {
		t.Fatalf("method normalize failed: seen=%s status=%s", seen, res.Status)
	}
}

func TestCheckBoundedHeaderBlocking(t *testing.T) {
	// Response header limit: fine to trust 64KiB transport setting; verify by
	// sending an oversized header.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Big", strings.Repeat("a", 100*1024))
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	c := New(testValidator(t))
	res := c.Check(context.Background(), Config{URL: srv.URL})
	if res.Status == StatusUp {
		t.Fatalf("oversized response headers must not be reported as up: %+v", res)
	}
}
