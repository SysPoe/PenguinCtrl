package preflight

import (
	"time"

	"github.com/syspoe/cusus/operatorlog"
)

// RuntimeReadiness is one observed runtime-state snapshot. Unlike show and
// settings analysis, runtime observations are not cached for refreshInterval:
// every new ObservedAt/check set becomes part of the signed result immediately.
// FreshFor optionally fails the gate closed when collection has stopped.
type RuntimeReadiness struct {
	ObservedAt time.Time
	FreshFor   time.Duration
	Checks     []Check
}

func (r RuntimeReadiness) expiresAt() time.Time {
	if r.FreshFor <= 0 || r.ObservedAt.IsZero() {
		return time.Time{}
	}
	return r.ObservedAt.Add(r.FreshFor)
}

func (r RuntimeReadiness) checksAt(now time.Time) []Check {
	checks := append([]Check(nil), r.Checks...)
	if r.FreshFor > 0 && r.ObservedAt.IsZero() {
		return append(checks, Check{
			Severity: operatorlog.ShowStopping, Code: "preflight.runtime.pending", Source: "Runtime readiness",
			Message: "Runtime health observations have not been collected yet", Consequence: "The GO gate cannot verify current device and service readiness",
			Fix: "Wait for health monitoring to complete its first collection before pressing GO",
		})
	}
	if expires := r.expiresAt(); !expires.IsZero() && !now.Before(expires) {
		checks = append(checks, Check{
			Severity: operatorlog.ShowStopping, Code: "preflight.runtime.stale", Source: "Runtime readiness",
			Message: "Runtime health observations are stale", Consequence: "The GO gate cannot verify current device and service readiness",
			Fix: "Wait for health monitoring to refresh before pressing GO",
		})
	}
	return checks
}
