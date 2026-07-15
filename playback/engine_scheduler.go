package playback

import "github.com/syspoe/cusus/show"

func (e *Engine) PlaySelected() error {
	return e.scheduler.playSelected(rejectBlockers)
}

// PlaySelectedOverride accepts the selected cue even when validation or the
// signed readiness barrier reports blockers.
func (e *Engine) PlaySelectedOverride() error {
	return e.scheduler.playSelected(overrideBlockers)
}

func (e *Engine) PlayIndex(index int) error { return e.scheduler.playIndex(index) }

func (e *Engine) PlayCueID(id show.CueID) error { return e.scheduler.playCueID(id) }

func (e *Engine) enqueueCommand(cue show.Cue, index int, intent commandIntent, origin string, blocker blockerPolicy) error {
	return e.scheduler.enqueue(cue, index, intent, origin, blocker)
}

func (e *Engine) enqueueEmbeddedCommand(cue show.Cue, index int, origin string, run cueRunToken) error {
	return e.scheduler.enqueueEmbedded(cue, index, origin, run)
}

func (e *Engine) enqueueAcceptedCommand(next command) error { return e.scheduler.accept(next) }
