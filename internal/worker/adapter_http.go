package worker

import (
	"context"
	"time"

	"site24x7/internal/checker/http"
	"site24x7/internal/domain"
	"site24x7/internal/safety"
)

// HTTPProbe adapts the safe HTTP checker to the worker Probe interface. Only
// http monitors reach it; the (multi-type) dispatching probe lands in Phase 8.
type HTTPProbe struct {
	checker *httpchecker.Checker
}

func NewHTTPProbe(v *safety.Validator) *HTTPProbe {
	return &HTTPProbe{checker: httpchecker.New(v)}
}

func (p *HTTPProbe) Probe(ctx context.Context, mon *domain.Monitor) domain.CheckResult {
	cfg := httpchecker.Config{
		URL:               mon.URL,
		Method:            mon.RequestConfig.Method,
		Headers:           mon.RequestConfig.Headers,
		Body:              mon.RequestConfig.Body,
		FollowRedirects:   mon.RequestConfig.FollowRedirects == nil || *mon.RequestConfig.FollowRedirects,
		ExpectedStatusMin: mon.RequestConfig.ExpectedStatusMin,
		ExpectedStatusMax: mon.RequestConfig.ExpectedStatusMax,
		Timeout:           time.Duration(mon.TimeoutMs) * time.Millisecond,
	}
	res := p.checker.Check(ctx, cfg)

	status := domain.CheckStatusDown
	switch res.Status {
	case httpchecker.StatusUp:
		status = domain.CheckStatusUp
	case httpchecker.StatusTimeout:
		status = domain.CheckStatusTimeout
	default:
		status = domain.CheckStatusDown
	}
	return domain.CheckResult{
		Status:         status,
		HTTPStatusCode: res.HTTPStatusCode,
		DurationMs:     res.Timings.TotalMs,
		DNSMs:          res.Timings.DNSMs,
		ConnectMs:      res.Timings.ConnectMs,
		TLSMs:          res.Timings.TLSMs,
		TTFBMs:         res.Timings.TTFBMs,
		ErrorClass:     domain.CheckErrorClass(res.ErrorClass),
		ErrorDetail:    res.ErrorDetail,
		ResponseBytes:  res.ResponseBytes,
	}
}
