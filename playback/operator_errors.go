package playback

import (
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

func (e *Engine) LastError() string {
	value := e.lastError.Load()
	if value == nil {
		return ""
	}
	message, _ := value.(string)
	return message
}

func (e *Engine) recordError(source string, err error) {
	e.recordOperatorError(operatorlog.Recoverable, source, err, show.CueID{}, "")
}

func (e *Engine) recordOperatorError(severity operatorlog.Severity, source string, err error, cueID show.CueID, cueNumber string) {
	if err == nil {
		return
	}
	e.lastError.Store(err.Error())
	if log := e.operatorLogStore(); log != nil {
		log.Add(severity, source, err.Error(), cueID, cueNumber)
	}
	for _, outputID := range e.OutputIDs() {
		e.outputs.publish(errorOutputEvent{outputID: outputID, err: err.Error()})
	}
	e.changed()
}

func (e *Engine) recordCueError(cue show.Cue, source string, err error) {
	e.recordOperatorError(operatorlog.CueFailure, source, err, cue.ID, cue.CueNumber)
}

func cueFailureSource(cue show.Cue) string {
	switch cue.Type {
	case show.CueTypeRemote:
		return "Network / remote cue"
	case show.CueTypeSound, show.CueTypeVideo, show.CueTypeImage:
		return "FFmpeg / media cue"
	case show.CueTypeWait:
		return "Wait cue"
	case show.CueTypeMediaControl, show.CueTypeOutputControl:
		return "Playback control cue"
	default:
		return "Playback engine"
	}
}
