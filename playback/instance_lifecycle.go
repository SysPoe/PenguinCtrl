package playback

import (
	"time"

	"github.com/syspoe/cusus/show"
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
	report := outputReport(reportText)
	e.mu.Lock()
	instance := e.instances[instanceID]
	if instance == nil {
		e.mu.Unlock()
		return
	}
	if applied, retire := reduceInstanceLifecycle(instance, report, time.Now()); !applied {
		e.mu.Unlock()
		return
	} else if retire {
		delete(e.instances, instanceID)
	}
	snapshot := *instance
	e.mu.Unlock()

	switch report {
	case outputReportStarted:
		if snapshot.FadeInMs == 0 {
			e.scheduleLink(snapshot.Cue, snapshot.CueIndex, snapshot.PostWaitMs, linkFadeIn, snapshot.run.ctx)
		}
		e.scheduleInstanceLifecycle(snapshot.ID)
		e.scheduleTimecode(snapshot.ID, snapshot.Cue, snapshot.CueIndex)
	case outputReportPresented:
		e.replaceSingleLayerVisual(snapshot)
	case outputReportFadeInComplete:
		e.scheduleLink(snapshot.Cue, snapshot.CueIndex, snapshot.PostWaitMs, linkFadeIn, snapshot.run.ctx)
	case outputReportFadeOutStart:
		e.scheduleLink(snapshot.Cue, snapshot.CueIndex, snapshot.PostWaitMs, linkFadeOut, snapshot.run.ctx)
	case outputReportEnded, outputReportStopped:
		e.outputs.publish(Event{Action: "remove", OutputID: snapshot.OutputID, InstanceIDs: []string{snapshot.ID}})
		e.scheduleLink(snapshot.Cue, snapshot.CueIndex, snapshot.PostWaitMs, linkEnd, snapshot.run.ctx)
		finalization := runCompleted
		if snapshot.Link.Mode == show.CueLinkManual {
			finalization = runAborted
		}
		e.finishCueRun(snapshot.run, finalization)
	}
	e.signalState()
}
