package playback

import (
	"context"

	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

const manualFadeOutMs int64 = 2000

type operatorControlHost interface {
	operatorLogStore() *operatorlog.Store
	resetPlaybackRuns()
	ActiveInstances() []Instance
	OutputIDs() []string
	publishOutput(outputEvent)
	HandleOutputReport(string, string)
	executeMediaControl(show.Cue, context.Context) error
	currentRunContext() context.Context
	matchingInstances(show.MediaTarget) []Instance
	recordError(string, error)
	rememberOutputVisual(string, Event)
	signalState()
}

type operatorController struct {
	host operatorControlHost
}

func newOperatorController(host operatorControlHost) *operatorController {
	return &operatorController{host: host}
}

func (controller *operatorController) stopAll() {
	if log := controller.host.operatorLogStore(); log != nil {
		log.Diagnostic("Operator action", "STOP ALL dispatched", nil)
	}
	controller.host.resetPlaybackRuns()
	instances := controller.host.ActiveInstances()
	// STOP ALL is output-wide rather than derived only from known instances, so
	// repeated presses can still close a late player after registry retirement.
	for _, outputID := range controller.host.OutputIDs() {
		controller.host.publishOutput(mediaControlOutputEvent{outputID: outputID, command: mediaCommandStopAll})
	}
	for _, instance := range instances {
		controller.host.HandleOutputReport(instance.ID, "stopped")
	}
}

func (controller *operatorController) blackoutAll() {
	for _, outputID := range controller.host.OutputIDs() {
		payload := outputControlOutputEvent{outputID: outputID, command: outputCommandBlackout}
		controller.host.rememberOutputVisual(outputID, payload.compatibilityEvent())
		controller.host.publishOutput(payload)
	}
	if log := controller.host.operatorLogStore(); log != nil {
		log.Diagnostic("Operator action", "Emergency blackout asserted on all outputs", nil)
	}
	controller.host.signalState()
}

func (controller *operatorController) controlMedia(target show.MediaTarget, action show.MediaControlAction, levelDB *float64, positionMs *int64, fadeMs int64) error {
	return controller.host.executeMediaControl(show.Cue{Play: show.CuePlay{MediaControl: &show.MediaControlPlay{
		Action: action, Target: target, LevelDB: levelDB, SeekToMs: positionMs,
		FadeMs: max(int64(0), fadeMs), Curve: show.FadeCurveLinear,
	}}}, controller.host.currentRunContext())
}

func (controller *operatorController) fadeInstance(instanceID string) error {
	return controller.controlMedia(
		show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instanceID},
		show.MediaControlFadeOut, nil, nil, manualFadeOutMs,
	)
}

func (controller *operatorController) fadeAll() {
	for _, instance := range controller.host.ActiveInstances() {
		if err := controller.fadeInstance(instance.ID); err != nil {
			controller.host.recordError("Operator Fade All", err)
		}
	}
}

func (controller *operatorController) endInstance(instanceID string) {
	instances := controller.host.matchingInstances(show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instanceID})
	if len(instances) == 0 {
		return
	}
	instance := instances[0]
	controller.host.publishOutput(mediaControlOutputEvent{
		outputID: instance.OutputID, instanceIDs: []string{instance.ID}, command: mediaCommandStop,
	})
	controller.host.HandleOutputReport(instance.ID, "ended")
}
