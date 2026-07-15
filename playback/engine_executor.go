package playback

func (e *Engine) CommandHistory() []CommandRecord { return e.scheduler.audit.snapshot() }

func (e *Engine) startExecution(next command, phase string, durationMs int64) string {
	return e.scheduler.startExecution(next, phase, durationMs)
}

func (e *Engine) updateExecution(id, phase string, durationMs int64) {
	e.scheduler.updateExecution(id, phase, durationMs)
}

func (e *Engine) finishExecution(id string) { e.scheduler.finishExecution(id) }
