package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

type controlActions struct {
	engine *Engine
}

func newControlActions(engine *Engine) *controlActions {
	return &controlActions{engine: engine}
}

func (actions *controlActions) executeMedia(cue show.Cue, runCtx context.Context) error {
	e := actions.engine
	if cue.Play.MediaControl == nil {
		return errors.New("media-control cue has no control settings")
	}
	playCopy := *cue.Play.MediaControl
	settings := e.settings.Snapshot()
	playCopy.Target.OutputID = config.Resolve(playCopy.Target.OutputID, settings, cue.CueNumber)
	playCopy.Target.InstanceID = config.Resolve(playCopy.Target.InstanceID, settings, cue.CueNumber)
	play := &playCopy
	if play.Action < show.MediaControlFadeTo || play.Action > show.MediaControlUnmute {
		return fmt.Errorf("invalid media control action %d", play.Action)
	}
	instances := e.matchingInstances(play.Target)
	if log := e.operatorLogStore(); len(instances) == 0 && log != nil {
		log.Add(operatorlog.Warning, "Media control result", "No active media matched", cue.ID, cue.CueNumber)
	}
	idsByOutput := map[string][]string{}
	for _, instance := range instances {
		idsByOutput[instance.OutputID] = append(idsByOutput[instance.OutputID], instance.ID)
	}
	control := mediaControlName(play.Action)
	for outputID, ids := range idsByOutput {
		e.outputs.publish(mediaControlOutputEvent{
			outputID: outputID, instanceIDs: ids, command: control, fadeMs: play.FadeMs,
			levelDB: play.LevelDB, positionMs: play.SeekToMs, curve: play.Curve,
		})
	}

	e.runtime.mu.Lock()
	now := time.Now()
	reschedule := make([]string, 0, len(instances))
	linkInstances := make([]liveInstance, 0, len(instances))
	for _, matched := range instances {
		instance := e.runtime.instances.get(matched.ID)
		if instance == nil {
			continue
		}
		materializeLiveInstance(instance, now)
		reduceMediaControl(instance, play, now, &reschedule)
		if play.Action == show.MediaControlFadeOut {
			linkInstances = append(linkInstances, *instance)
		}
	}
	e.runtime.mu.Unlock()
	for _, id := range reschedule {
		e.scheduleInstanceLifecycle(id)
	}
	if play.Action == show.MediaControlFadeOut {
		for _, instance := range linkInstances {
			e.lifecycle.dispatchLink(instance, linkFadeOut)
		}
	}
	if play.Action == show.MediaControlStop || play.Action == show.MediaControlFadeOut {
		delay := time.Duration(max(0, play.FadeMs)) * time.Millisecond
		for _, instance := range instances {
			id := instance.ID
			e.goOwned(func() {
				if waitContext(runCtx, delay) {
					e.HandleOutputReport(id, "ended")
				}
			})
		}
	}
	e.signalState()
	return nil
}

func reduceMediaControl(instance *liveInstance, play *show.MediaControlPlay, now time.Time, reschedule *[]string) {
	switch play.Action {
	case show.MediaControlPause:
		instance.Paused = true
		instance.positionAt = time.Time{}
		instance.endScheduled = false
		instance.lifecycleGeneration++
	case show.MediaControlResume:
		instance.Paused = false
		instance.positionAt = now
		instance.endScheduled = false
		instance.lifecycleGeneration++
		*reschedule = append(*reschedule, instance.ID)
	case show.MediaControlSeek:
		if play.SeekToMs != nil {
			instance.PositionMs = max(instance.ClipStartMs, *play.SeekToMs)
			if instance.ClipEndMs > instance.ClipStartMs {
				instance.PositionMs = min(instance.PositionMs, instance.ClipEndMs)
			}
			if instance.Paused {
				instance.positionAt = time.Time{}
			} else {
				instance.positionAt = now
				*reschedule = append(*reschedule, instance.ID)
			}
			instance.endScheduled = false
			instance.lifecycleGeneration++
		}
	case show.MediaControlFadeTo, show.MediaControlSetVolume:
		if play.LevelDB != nil {
			startInstanceFade(instance, *play.LevelDB, play.FadeMs, now)
		}
	case show.MediaControlFadeOut:
		startInstanceFade(instance, silenceFloorDB, play.FadeMs, now)
	case show.MediaControlStop:
		if play.FadeMs > 0 {
			startInstanceFade(instance, silenceFloorDB, play.FadeMs, now)
		}
	case show.MediaControlMute:
		instance.Muted = true
	case show.MediaControlUnmute:
		instance.Muted = false
	}
}

func (actions *controlActions) executeOutput(cue show.Cue, runCtx context.Context) error {
	e := actions.engine
	if cue.Play.OutputControl == nil {
		return errors.New("output-control cue has no control settings")
	}
	playCopy := *cue.Play.OutputControl
	settings := e.settings.Snapshot()
	playCopy.Message = config.Resolve(playCopy.Message, settings, cue.CueNumber)
	play := &playCopy
	if play.Action < show.OutputControlBlackout || play.Action > show.OutputControlExitFullscreen {
		return fmt.Errorf("invalid output control action %d", play.Action)
	}
	outputID := resolveOutput(play.OutputID, settings, cue.CueNumber)
	payload := outputControlOutputEvent{
		outputID: outputID, command: outputControlName(play.Action), fadeOutMs: max(int64(0), play.FadeOutMs),
		fadeInMs: max(int64(0), play.FadeInMs), message: play.Message,
	}
	switch play.Action {
	case show.OutputControlBlackout, show.OutputControlClear, show.OutputControlTestPattern, show.OutputControlIdentify:
		e.outputs.rememberVisual(outputID, payload.compatibilityEvent())
	case show.OutputControlFullscreen, show.OutputControlExitFullscreen:
		e.outputs.rememberWindow(outputID, payload.compatibilityEvent())
	}
	e.outputs.publish(payload)
	if play.Action == show.OutputControlBlackout {
		e.goOwned(func() {
			if waitContext(runCtx, time.Duration(max(int64(0), play.FadeOutMs))*time.Millisecond) {
				e.mediaRuntime.freezeImages(outputID)
			}
		})
	}
	if play.Action == show.OutputControlClear {
		instances := e.instancesForOutput(outputID)
		e.goOwned(func() {
			if !waitContext(runCtx, time.Duration(max(int64(0), play.FadeOutMs))*time.Millisecond) {
				return
			}
			for _, instance := range instances {
				e.HandleOutputReport(instance.ID, "ended")
			}
		})
	}
	return nil
}
