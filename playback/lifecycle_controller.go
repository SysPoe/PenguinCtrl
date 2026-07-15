package playback

import (
	"context"
	"time"

	"github.com/syspoe/cusus/show"
)

const silenceFloorDB = -80.0

type lifecycleHost interface {
	goOwned(func())
	scheduleLink(show.Cue, int, int64, linkMoment, context.Context)
	scheduleTimecode(string, show.Cue, int)
	replaceSingleLayerVisual(liveInstance)
	finishCueRun(cueRunToken, runFinalization)
	signalState()
}

type outputPublisher interface {
	publish(outputEvent)
}

type lifecycleController struct {
	host    lifecycleHost
	runtime *runtimeState
	outputs outputPublisher
}

func newLifecycleController(host lifecycleHost, runtime *runtimeState, outputs outputPublisher) *lifecycleController {
	return &lifecycleController{host: host, runtime: runtime, outputs: outputs}
}

type instanceLifecycleSchedule struct {
	instance   liveInstance
	generation uint64
	fadeAfter  time.Duration
	fadeFor    int64
	endAfter   time.Duration
}

func prepareInstanceLifecycle(instance *liveInstance, now time.Time) (instanceLifecycleSchedule, bool) {
	if instance == nil || !instance.BackendStarted || instance.DurationMs <= 0 || instance.endScheduled {
		return instanceLifecycleSchedule{}, false
	}
	materializeLiveInstance(instance, now)
	remainingMs := max(int64(0), instance.DurationMs-(instance.PositionMs-instance.ClipStartMs))
	instance.endScheduled = true
	instance.lifecycleGeneration++
	fadeMs := min(max(int64(0), instance.FadeOutMs), remainingMs)
	return instanceLifecycleSchedule{
		instance:   *instance,
		generation: instance.lifecycleGeneration,
		fadeAfter:  time.Duration(remainingMs-fadeMs) * time.Millisecond,
		fadeFor:    fadeMs,
		endAfter:   time.Duration(remainingMs) * time.Millisecond,
	}, true
}

func (c *lifecycleController) schedule(instanceID string) {
	c.runtime.mu.Lock()
	schedule, ok := prepareInstanceLifecycle(c.runtime.instances.get(instanceID), time.Now())
	c.runtime.mu.Unlock()
	if !ok {
		return
	}

	if schedule.fadeFor > 0 {
		c.host.goOwned(func() {
			if !waitContext(schedule.instance.run.ctx, schedule.fadeAfter) || !c.current(schedule.instance.ID, schedule.generation) {
				return
			}
			c.outputs.publish(mediaControlOutputEvent{
				outputID: schedule.instance.OutputID, instanceIDs: []string{schedule.instance.ID},
				command: mediaCommandFadeOut, fadeMs: schedule.fadeFor,
			})
			c.runtime.mu.Lock()
			if active := c.runtime.instances.get(schedule.instance.ID); active != nil && active.lifecycleGeneration == schedule.generation && !active.Paused {
				now := time.Now()
				materializeLiveInstance(active, now)
				startInstanceFade(active, silenceFloorDB, schedule.fadeFor, now)
			}
			c.runtime.mu.Unlock()
			c.handleOutputReport(schedule.instance.ID, outputReportFadeOutStart)
		})
	}
	c.host.goOwned(func() {
		if waitContext(schedule.instance.run.ctx, schedule.endAfter) && c.current(schedule.instance.ID, schedule.generation) {
			c.handleOutputReport(schedule.instance.ID, outputReportEnded)
		}
	})
}

func (c *lifecycleController) current(instanceID string, generation uint64) bool {
	c.runtime.mu.RLock()
	defer c.runtime.mu.RUnlock()
	return c.runtime.instances.lifecycleCurrent(instanceID, generation)
}

func (c *lifecycleController) handleOutputReport(instanceID string, report outputReport) {
	c.runtime.mu.Lock()
	instance := c.runtime.instances.get(instanceID)
	if instance == nil {
		c.runtime.mu.Unlock()
		return
	}
	if applied, retire := reduceInstanceLifecycle(instance, report, time.Now()); !applied {
		c.runtime.mu.Unlock()
		return
	} else if retire {
		c.runtime.instances.retire(instanceID)
	}
	snapshot := *instance
	c.runtime.mu.Unlock()

	switch report {
	case outputReportStarted:
		if snapshot.FadeInMs == 0 {
			c.dispatchLink(snapshot, linkFadeIn)
		}
		c.schedule(snapshot.ID)
		c.host.scheduleTimecode(snapshot.ID, snapshot.cue, snapshot.cueIndex)
	case outputReportPresented:
		c.host.replaceSingleLayerVisual(snapshot)
	case outputReportFadeInComplete:
		c.dispatchLink(snapshot, linkFadeIn)
	case outputReportFadeOutStart:
		c.dispatchLink(snapshot, linkFadeOut)
	case outputReportEnded, outputReportStopped:
		c.outputs.publish(removeOutputEvent{outputID: snapshot.OutputID, instanceIDs: []string{snapshot.ID}})
		c.dispatchLink(snapshot, linkEnd)
		finalization := runCompleted
		if snapshot.link.Mode == show.CueLinkManual {
			finalization = runAborted
		}
		c.host.finishCueRun(snapshot.run, finalization)
	}
	c.host.signalState()
}

func (c *lifecycleController) dispatchLink(instance liveInstance, moment linkMoment) {
	c.host.scheduleLink(instance.cue, instance.cueIndex, instance.postWaitMs, moment, instance.run.ctx)
}
