package playback

import (
	"context"

	"github.com/syspoe/cusus/show"
)

func (e *Engine) StopAll() { e.operator.stopAll() }

func (e *Engine) resetPlaybackRuns() {
	e.mu.Lock()
	e.runCancel()
	e.runCtx, e.runCancel = context.WithCancel(e.ctx)
	e.runs.reset()
	e.mu.Unlock()
}

// BlackoutAll immediately asserts black on every configured/active output.
// It is deliberately independent from cue selection and keyboard focus.
func (e *Engine) BlackoutAll() { e.operator.blackoutAll() }

// ControlMedia applies an operator control directly to matching live media.
// It is the runtime equivalent of playing a media-control cue, without adding
// an artificial cue to the show.
func (e *Engine) ControlMedia(target show.MediaTarget, action show.MediaControlAction, levelDB *float64, positionMs *int64, fadeMs int64) error {
	return e.operator.controlMedia(target, action, levelDB, positionMs, fadeMs)
}

func (e *Engine) currentRunContext() context.Context {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.runCtx
}

// FadeInstance performs the fixed two-second fade used by the operator panel.
func (e *Engine) FadeInstance(instanceID string) error { return e.operator.fadeInstance(instanceID) }

// FadeAll performs the fixed two-second operator fade on every live instance.
func (e *Engine) FadeAll() { e.operator.fadeAll() }

// EndInstance jumps a live instance to its logical end, including normal end
// link handling, rather than seeking beyond a configured clip boundary.
func (e *Engine) EndInstance(instanceID string) { e.operator.endInstance(instanceID) }

func (e *Engine) publishOutput(event outputEvent) { e.outputs.publish(event) }

func (e *Engine) rememberOutputVisual(outputID string, event Event) {
	e.mu.Lock()
	e.outputVisuals[outputID] = event
	e.mu.Unlock()
}
