package playback

import (
	"time"
)

type outputReport string

const (
	outputReportStarted        outputReport = "started"
	outputReportPresented      outputReport = "presented"
	outputReportFadeInComplete outputReport = "fade-in-complete"
	outputReportFadeOutStart   outputReport = "fade-out-start"
	outputReportEnded          outputReport = "ended"
	outputReportStopped        outputReport = "stopped"
)

// reduceInstanceLifecycle applies report-driven instance state changes. Its
// applied result is false only when an idempotent report is already reflected
// in the instance; retire indicates that the caller should unregister it.
func reduceInstanceLifecycle(instance *Instance, report outputReport, now time.Time) (applied, retire bool) {
	switch report {
	case outputReportStarted:
		if instance.BackendStarted {
			return false, false
		}
		instance.BackendStarted = true
		instance.LoadState = "playing"
		instance.StartLatencyMs = max(int64(0), now.Sub(instance.RequestedAt).Milliseconds())
		instance.StartedAt, instance.PositionAt = now, now
	case outputReportFadeInComplete:
		if instance.FadeInComplete {
			return false, false
		}
		instance.FadeInComplete = true
	case outputReportFadeOutStart:
		if instance.FadeOutStarted {
			return false, false
		}
		instance.FadeOutStarted = true
	case outputReportPresented:
		if instance.Presented {
			return false, false
		}
		instance.Presented = true
	case outputReportEnded, outputReportStopped:
		return true, true
	}
	return true, false
}

func (e *Engine) HandleOutputReport(instanceID, reportText string) {
	e.lifecycle.handleOutputReport(instanceID, outputReport(reportText))
}

func (e *Engine) scheduleInstanceLifecycle(instanceID string) {
	e.lifecycle.schedule(instanceID)
}
