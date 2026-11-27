package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"site24x7/internal/api"
	"site24x7/internal/auth"
	"site24x7/internal/domain"
	"site24x7/internal/observability"
	"site24x7/internal/store"
	"site24x7/internal/uptime"
)

func testDB(t *testing.T) *store.All {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	all, err := store.NewAll(ctx, dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(all.Close)
	return all
}

func newTestServer(t *testing.T) http.Handler {
	all := testDB(t)
	return api.NewRouter(&api.API{
		All:    all,
		Tokens: auth.NewTokens([]byte("test-secret-at-least-32-bytes-long!!"), 15*time.Minute),
		Probe:  &fakeProbe{},
		Up:     uptime.New(all.Pool),
		Log:    observability.Nop(),
	})
}

type fakeProbe struct{}

func (f *fakeProbe) Probe(_ context.Context, _ *domain.Monitor) domain.CheckResult {
	return domain.CheckResult{}
}

var _ api.MonitorProber = (*fakeProbe)(nil)

func do(t *testing.T, h http.Handler, method, path, token string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	out := map[string]any{}
	if res.Body != nil {
		_ = json.NewDecoder(res.Body).Decode(&out)
		_ = res.Body.Close()
	}
	return res, out
}

func TestAPIAuthMatrix(t *testing.T) {
	h := newTestServer(t)

	// Unauthenticated access is rejected.
	res, _ := do(t, h, "GET", "/api/v1/auth/me", "", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me without token: %d, want 401", res.StatusCode)
	}

	// Register.
	res, reg := do(t, h, "POST", "/api/v1/auth/register", "", map[string]any{
		"email":     "api-" + time.Now().Format("150405.000000000") + "@example.com",
		"password":  "correct-horse-battery",
		"full_name": "API Tester",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d: %v", res.StatusCode, reg)
	}
	token, _ := reg["access_token"].(string)
	if token == "" {
		t.Fatalf("register returned no token: %v", reg)
	}

	// Duplicate email conflicts.
	res, _ = do(t, h, "POST", "/api/v1/auth/register", "", map[string]any{
		"email": reg["user"].(map[string]any)["email"], "password": "correct-horse-battery",
	})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate register: %d, want 409", res.StatusCode)
	}

	// Weak password rejected.
	res, _ = do(t, h, "POST", "/api/v1/auth/register", "", map[string]any{
		"email": "weak-" + time.Now().Format("150405") + "@example.com", "password": "short",
	})
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("weak password: %d, want 422", res.StatusCode)
	}

	// Login works.
	res, login := do(t, h, "POST", "/api/v1/auth/login", "", map[string]any{
		"email": reg["user"].(map[string]any)["email"], "password": "correct-horse-battery",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", res.StatusCode)
	}
	token, _ = login["access_token"].(string)

	// Wrong password rejected.
	res, _ = do(t, h, "POST", "/api/v1/auth/login", "", map[string]any{
		"email": reg["user"].(map[string]any)["email"], "password": "definitely-wrong-password",
	})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password login: %d, want 401", res.StatusCode)
	}

	// Me returns the user.
	res, me := do(t, h, "GET", "/api/v1/auth/me", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("me: %d: %v", res.StatusCode, me)
	}

	// Create org + project + monitor lifecycle.
	res, org := do(t, h, "POST", "/api/v1/organizations", token, map[string]any{
		"name": "Org API", "slug": "org-api-" + strconv.FormatInt(time.Now().UnixNano(), 36),
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create org: %d: %v", res.StatusCode, org)
	}
	orgID, _ := org["id"].(string)

	// A second, unrelated user cannot see this org.
	res, reg2 := do(t, h, "POST", "/api/v1/auth/register", "", map[string]any{
		"email":    "other-" + time.Now().Format("150405.000000000") + "@example.com",
		"password": "another-good-password",
	})
	res, _ = do(t, h, "GET", "/api/v1/organizations/"+orgID, reg2["access_token"].(string), nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-tenant org read: %d, want 404", res.StatusCode)
	}

	res, proj := do(t, h, "POST", "/api/v1/organizations/"+orgID+"/projects", token, map[string]any{"name": "prod"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create project: %d: %v", res.StatusCode, proj)
	}
	projID, _ := proj["id"].(string)

	base := "/api/v1/organizations/" + orgID + "/projects/" + projID + "/monitors"

	res, mon := do(t, h, "POST", base, token, map[string]any{
		"name": "example", "url": "https://example.com/", "interval_seconds": 60,
		"timeout_ms": 10000, "failure_threshold": 2,
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create monitor: %d: %v", res.StatusCode, mon)
	}
	monID, _ := mon["id"].(string)
	if mon["state"] != "paused" {
		t.Fatalf("new monitor state = %v, want paused", mon["state"])
	}

	// Private URL rejected (SSRF guard at the API edge).
	res, _ = do(t, h, "POST", base, token, map[string]any{
		"name": "evil", "url": "http://169.254.169.254/latest/meta-data/",
	})
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("metadata URL creation: %d, want 422", res.StatusCode)
	}
	res, _ = do(t, h, "POST", base, token, map[string]any{
		"name": "loopback", "url": "http://127.0.0.1:8080/",
	})
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("loopback URL creation: %d, want 422", res.StatusCode)
	}

	// Pause/resume state transitions.
	res, _ = do(t, h, "POST", base+"/"+monID+"/resume", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("resume: %d", res.StatusCode)
	}
	res, got := do(t, h, "GET", base+"/"+monID, token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get monitor: %d", res.StatusCode)
	}
	if got["state"] != "up" {
		t.Fatalf("monitor after resume: %v, want up", got["state"])
	}
	res, _ = do(t, h, "POST", base+"/"+monID+"/pause", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("pause: %d", res.StatusCode)
	}

	// Manual test executes without code 500.
	res, _ = do(t, h, "POST", base+"/"+monID+"/test", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("manual test: %d", res.StatusCode)
	}

	// History is empty but well-formed.
	res, hist := do(t, h, "GET", base+"/"+monID+"/history", token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("history: %d", res.StatusCode)
	}
	if hist["checks"] == nil {
		t.Fatalf("history missing checks array: %v", hist)
	}

	// 404 for a foreign monitor id inside the same path shape.
	res, _ = do(t, h, "GET", base+"/00000000-0000-0000-0000-000000000000", token, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown monitor: %d, want 404", res.StatusCode)
	}

	// 401 when the bearer token is garbage.
	garbage := "eyJhbGciOiJIUzI1NiJ9.not.valid"
	res, _ = do(t, h, "GET", base, garbage, nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("garbage token: %d, want 401", res.StatusCode)
	}
}

func TestAPIAuthMatrixRoleEnforcement(t *testing.T) {
	h := newTestServer(t)
	// Owner creates org; a viewer added is implied by role matrix in store
	// tests. Here: unknown org reads 404 even when authenticated.
	res, reg := do(t, h, "POST", "/api/v1/auth/register", "", map[string]any{
		"email":    "role-" + time.Now().Format("150405.000000000") + "@example.com",
		"password": "another-good-password",
	})
	res, _ = do(t, h, "GET", "/api/v1/organizations/00000000-0000-0000-0000-000000000000", reg["access_token"].(string), nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown org: %d, want 404", res.StatusCode)
	}
}
