# Threat Model

> Phase 0 deliverable. Focus: the product's defining attack surface is **untrusted destination
> URLs** — the platform is paid to make outbound requests to customer-supplied targets, which is
> exactly what an attacker wants SSRF to do for free.

## 1. Assets

| Asset | Concern |
|---|---|
| Proof of private-network reachability | Highest value: an attacker who can point monitors at clouds metadata/link-local addresses uses the platform as an SSRF oracle (status/timing/body) |
| Customer credentials / password hashes / refresh tokens | Account takeover |
| Webhook signing secrets, SMTP credentials | Exfiltration and sender spoofing |
| Monitoring data of other tenants | Cross-tenant information disclosure |
| Internal services (Postgres, Redis, Caddy admin) | Lateral movement from the app |

## 2. Trust boundaries

```
[Customer browser] ⇄ [Edge/Caddy] ⇄ [Go API] ⇄ [Postgres/Redis]   — boundary T1: untrusted input
[Go workers] → [arbitrary customer targets]                        — boundary T2: outbound, the critical one
[Webhook targets] ⇐ [Notification dispatcher]                      — boundary T3: exfil channel
```

## 3. STRIDE per boundary

### T1 — API ingress (STRIDE: S,T,R,I)

| Threat | Control |
|---|---|
| Credential stuffing / brute force | Rate limit `/auth/*` (10/min/IP), argon2id with per-call memory ceiling, generic error messages |
| Session theft | Short-lived access tokens, rotating single-use refresh tokens, reuse detection kills token family |
| Cross-tenant data access | Every handler receives scoped context `(org_id, project_id, role)`; repositories require the scope struct; repository tests assert org filter presence |
| Mass assignment / injection | Strict request structs, `DisallowUnknownFields`, SQL only via pgx parameterized queries |
| Audit evasion | `audit_logs` on all mutations: actor, IP, action, redacted diff |

### T2 — Outbound monitoring (STRIDE: all; SSRF is the headline)

| Threat | Control |
|---|---|
| Pointing monitors at `127.0.0.1`, `10.0.0.0/8`, `172.16/12`, `192.168/16`, `169.254/16` (metadata), IPv6 equivalents (`::1`, `fe80::/10`, `fc00::/7`, `ff00::/8`), `0.0.0.0/8`, `100.64/10`, `198.18/15`, `192.0.0/24`, `192.0.2/24`, `198.51.100/24`, `203.0.113/24`, `224/4`, `240/4`, `255.255.255.255`, decimal/octal/hex integer host forms (`0x7f000001`, `2130706433`) | URL host parsed only as a hostname; **resolution is performed by our resolver**, then each answer IP is checked against an exhaustive blocklist before connect; text-form IP literal URLs are checked pre-resolution |
| DNS rebinding (TOCTOU): validate resolves to public, then `http.Client` re-resolves to private | The safe transport's `DialContext` is given the **pre-resolved IP** (custom `net.Dialer` + explicit `netip.Addr` dial, or a per-request `Resolver` that bypasses the OS resolver and validates at connect time). We dial the validated IP, never re-lookup. IP re-validation also happens inside `Control` callback (`net.Dialer.Control`) as belt-and-suspenders |
| Redirect to private target or infinite loops | `CheckRedirect` closure re-runs full URL validation + IP validation for every hop; max 5 hops (configurable); redirects to non-http(s) schemes (`file:`, `gopher:`, `data:`) rejected; "unexpected final target" assertion supported via `allowed_redirect_target` |
| `file://`, `ftp://`, `gopher://`, `dict://` | Scheme must be exactly `http` or `https` after URL canonicalization |
| DNS export / exfil via webhooks to internal targets | Webhook delivery URLs validated by the same `safety` package at dispatch time, not only at creation |
| Slowloris / resource exhaustion by malicious targets | Dial timeout, TLS handshake timeout, overall response-header timeout, total check timeout from monitor config (max 30s), bounded worker count, `Context` cancellation |
| Gigantic bodies / decompression bombs | `http.Transport` with `ResponseHeaderTimeout`; body read capped at `maxBodyBytes` (keyword reads 256 KiB default, hard cap 1 MiB); `io.LimitReader` + `MaxBytesReader` semantics; `Content-Length` pre-check; gzip with decompression byte cap |
| Header smuggling in our outbound requests | Header value validation on monitor create; deny `\r\n`; reserved headers (`Host`, `Authorization` to *our* service) ignored |
| Timing/size side channels on internal services | Even on blocked targets we return the same generic downgrade (`ssl unreachable` / `connect refused` shape) — no stack traces, no dial targets, no `net` error strings to customers |
| Secrets in stored monitor headers (e.g. customer's `Authorization` bearer) | Stored encrypted at rest (AES-GCM with `ENCRYPTION_KEY` from env/KMS); decrypted only inside checker process; never returned by API (write-only fields); redacted in logs and audit diffs |

### T3 — Webhook egress

| Threat | Control |
|---|---|
| Attracting deliveries to internal addresses | Same SSRF validation at dispatch |
| Target owner forging "our" notifications | HMAC-SHA256 signature header `X-Site24x7-Signature: t=<unix>, v1=<hmac>` over timestamp+body, `WEBHOOK_SECRET` per channel |
| Liked-list amplification | Per-target rate limit (30/min), exponential backoff, hard cap 5 attempts, circuit-break after repeated failures |

### Data layer

| Threat | Control |
|---|---|
| SQL injection | Parameterized pgx only; no dynamic SQL string building; lint rule via `sqlvet`-style check in CI |
| PII over-exposure | Password hashes never leave users table; emails in webhooks as configured by the tenant; status pages expose hosts only |
| Secret sprawl | All credentials from env/secret store; `ENCRYPTION_KEY` for customer-supplied secrets; `.env.example` carries placeholders only; nothing hardcoded (enforced by grep in CI) |

## 4. Residual risks (accepted, documented)

- Single-region probes: a customer could learn whether *our* region can reach *their* internal IPs —
  by design they can only point at hosts our workers resolve; blocklists + egress network policy
  (workers egress only via a proxy allowlist in hardened deployments) keep this to public internet.
- IPv6 zone identifiers and exotic host forms are normalized/rejected at parse time rather than
  exhaustively enumerated.
- Redis-down mode degrades rate limiting to per-instance accuracy.

## 5. Security test plan (phase mapping)

| Suite | Location | Phase |
|---|---|---|
| IP blocklist table-driven tests incl. every RFC range and encodings | `internal/safety/*_test.go` | 1 |
| DNS-rebinding simulation (resolver returns public then private) | `internal/safety` + integration test with embedded DNS server | 1, 3 |
| Redirect-chain validation tests | `internal/checker/http` | 1, 8 |
| Body-size / slowloris timeout tests with httptest server | `internal/checker/http` | 1 |
| Tenant isolation: every repo query runs with cross-tenant fixture and must return zero rows | `internal/store` | 2+ |
| AuthN/AuthZ API tests (401/403 matrix per role) | `internal/api` | 5 |
| Rate limit tests | `internal/middleware` | 5 |
| Webhook signature + SSRF-at-dispatch tests | `internal/notification` | 6 |
