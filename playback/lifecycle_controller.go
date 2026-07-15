package playback

import (
	"context"
	"sync"
	"time"

	"github.com/syspoe/cusus/show"
)

const silenceFloorDB = -80.0

type lifecycleHost interface {
	goOwned(func()) bool
	scheduleLink(show.Cue, int, int64, linkMoment, context.Context)
	scheduleTimecode(string, show.Cue, int)
	replaceSingleLayerVisual(Instance)
	finishCueRun(cueRunToken, runFinalization)
	signalState()
}

type lifecycleController struct {
	host      lifecycleHost
	mu        *sync.RWMutex
	instances *instanceRegistry
	outputs   *outputBus
}

func newLifecycleController(host lifecycleHost, mu *sync.RWMutex, instances *instanceRegistry, outputs *outputBus) *lifecycleController {
	return &lifecycleController{host: host, mu: mu, instances: instances, outputs: outputs}
}

type instanceLifecycleSchedule struct {
	instance   Instance
	generation uint64
	fadeAfter  time.Duration
	fadeFor    int64
	endAfter   time.Duration
}

func prepareInstanceLifecycle(instance *Instance, now time.Time) (instanceLifecycleSchedule, bool) {
	if instance == nil || !instance.BackendStarted || instance.DurationMs <= 0 || instance.EndScheduled {
		return instanceLifecycleSchedule{}, false
	}
	materializeInstance(instance, now)
	remainingMs := max(int64(0), instance.DurationMs-(instance.PositionMs-instance.ClipStartMs))
	instance.EndScheduled = true
	instance.LifecycleGeneration++
	fadeMs := min(max(int64(0), instance.FadeOutMs), remainingMs)
	return instanceLifecycleSchedule{
		instance:   *instance,
		generation: instance.LifecycleGeneration,
		fadeAfter:  time.Duration(remainingMs-fadeMs) * time.Millisecond,
		fadeFor:    fadeMs,
		endAfter:   time.Duration(remainingMs) * time.Millisecond,
	}, true
}

func (c *lifecycleController) schedule(instanceID string) {
	c.mu.Lock()
	schedule, ok := prepareInstanceLifecycle(c.instances.get(instanceID), time.Now())
	c.mu.Unlock()
	if !ok {
		return
	}

	if schedule.fadeFor > 0 {
		c.host.goOwned(func() {
			if !waitContext(schedule.instance.run.ctx, schedule.fadeAfter) || !c.current(schedule.instance.ID, schedule.generation) {
				return
			}
			c.outputs.publish(Event{
				Action: "control", OutputID: schedule.instance.OutputID, InstanceIDs: []string{schedule.instance.ID},
				Control: "fade-out", FadeMs: schedule.fadeFor,
			})
			c.mu.Lock()
			if active := c.instances.get(schedule.instance.ID); active != nil && active.LifecycleGeneration == schedule.generation && !active.Paused {
				now := time.Now()
				materializeInstance(active, now)
				startInstanceFade(active, silenceFloorDB, schedule.fadeFor, now)
			}
			c.mu.Unlock()
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
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.instances.lifecycleCurrent(instanceID, generation)
}

func (c *lifecycleController) handleOutputReport(instanceID string, report outputReport) {
	c.mu.Lock()
	instance := c.instances.get(instanceID)
	if instance == nil {
		c.mu.Unlock()
		return
	}
	if applied, retire := reduceInstanceLifecycle(instance, report, time.Now()); !applied {
		c.mu.Unlock()
		return
	} else if retire {
		c.instances.retire(instanceID)
	}
	snapshot := *instance
	c.mu.Unlock()

	switch report {
	case outputReportStarted:
		if snapshot.FadeInMs == 0 {
			c.dispatchLink(snapshot, linkFadeIn)
		}
		c.schedule(snapshot.ID)
		c.host.scheduleTimecode(snapshot.ID, snapshot.Cue, snapshot.CueIndex)
	case outputReportPresented:
		c.host.replaceSingleLayerVisual(snapshot)
	case outputReportFadeInComplete:
		c.dispatchLink(snapshot, linkFadeIn)
	case outputReportFadeOutStart:
		c.dispatchLink(snapshot, linkFadeOut)
	case outputReportEnded, outputReportStopped:
		c.outputs.publish(Event{Action: "remove", OutputID: snapshot.OutputID, InstanceIDs: []string{snapshot.ID}})
		c.dispatchLink(snapshot, linkEnd)
		finalization := runCompleted
		if snapshot.Link.Mode == show.CueLinkManual {
			finalization = runAborted
		}
		c.host.finishCueRun(snapshot.run, finalization)
	}
	c.host.signalState()
}

func (c *lifecycleController) dispatchLink(instance Instance, moment linkMoment) {
	c.host.scheduleLink(instance.Cue, instance.CueIndex, instance.PostWaitMs, moment, instance.run.ctx)
}
