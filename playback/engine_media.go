package playback

import (
	"context"

	"github.com/syspoe/cusus/show"
)

// Engine retains the compatibility method surface used by cue executors and
// lifecycle orchestration; focused collaborators own each implementation.
func (e *Engine) startMedia(next command) error { return e.mediaRuntime.start(next) }

func (e *Engine) scheduleTimecode(instanceID string, cue show.Cue, cueIndex int) {
	e.timecodes.schedule(instanceID, cue, cueIndex)
}

func (e *Engine) executeMediaControl(cue show.Cue, runCtx context.Context) error {
	return e.controls.executeMedia(cue, runCtx)
}

func (e *Engine) executeOutputControl(cue show.Cue, runCtx context.Context) error {
	return e.controls.executeOutput(cue, runCtx)
}

func (e *Engine) executeWait(cue show.Cue, runCtx context.Context) error {
	return e.waits.execute(cue, runCtx)
}
