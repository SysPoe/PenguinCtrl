package playback

import (
	"context"
	"errors"
	"time"

	"github.com/syspoe/cusus/show"
)

type linkMoment int

const (
	linkStart linkMoment = iota
	linkFadeIn
	linkFadeOut
	linkEnd
)

type cueSelectionPort interface {
	cues() []show.Cue
	selectCue(int)
	deselectCue()
}

type showAccessCueSelection struct {
	show ShowAccess
}

func (port showAccessCueSelection) cues() []show.Cue {
	return port.show.Snapshot()
}

func (port showAccessCueSelection) selectCue(index int) {
	port.show.SelectCue(index)
}

func (port showAccessCueSelection) deselectCue() {
	port.show.DeselectCue()
}

type linkNavigationHost interface {
	goOwned(func()) bool
	startExecution(command, string, int64) string
	finishExecution(string)
	enqueueCommand(show.Cue, int, commandIntent, string, blockerPolicy) error
	recordCueError(show.Cue, string, error)
	recordError(string, error)
	changed()
}

type linkNavigator struct {
	host      linkNavigationHost
	selection cueSelectionPort
}

func newLinkNavigator(host linkNavigationHost, selection cueSelectionPort) *linkNavigator {
	return &linkNavigator{host: host, selection: selection}
}

func (navigator *linkNavigator) schedule(source show.Cue, sourceIndex int, delayMs int64, moment linkMoment, runCtx context.Context) {
	if !linkMatches(source.Link.Mode, moment) {
		return
	}
	navigator.host.goOwned(func() {
		executionID := ""
		if delayMs > 0 {
			executionID = navigator.host.startExecution(command{cue: source, index: sourceIndex}, "post-wait", delayMs)
			defer navigator.host.finishExecution(executionID)
		}
		if !waitContext(runCtx, time.Duration(max(0, delayMs))*time.Millisecond) {
			return
		}
		target, targetIndex, ok := navigator.resolveTarget(source.Link.Target, sourceIndex)
		if !ok {
			cues := navigator.selection.cues()
			if nextLinkFallsPastEnd(source.Link.Target, sourceIndex, len(cues)) {
				navigator.selection.deselectCue()
				navigator.host.changed()
				return
			}
			if sourceIndex >= 0 && sourceIndex < len(cues) {
				navigator.host.recordCueError(cues[sourceIndex], "Cue link", errors.New("linked cue target does not exist"))
			} else {
				navigator.host.recordError("Cue link", errors.New("linked cue target does not exist"))
			}
			return
		}
		navigator.selection.selectCue(targetIndex)
		navigator.host.changed()
		if isAdvanceLink(source.Link.Mode) {
			return
		}
		_ = navigator.host.enqueueCommand(target, targetIndex, liveCommand, "Cue link from "+cueDisplayNumber(source), rejectBlockers)
	})
}

func (navigator *linkNavigator) resolveTarget(target show.CueTarget, sourceIndex int) (show.Cue, int, bool) {
	cues := navigator.selection.cues()
	index := -1
	switch target.Kind {
	case show.CueTargetNone, show.CueTargetNext:
		// Older cues can have a non-manual link mode but no explicit target.
		// Treat that combination as the conventional "next cue" target.
		index = sourceIndex + 1
	case show.CueTargetPrevious:
		index = sourceIndex - 1
	case show.CueTargetCue:
		for i := range cues {
			if cues[i].ID == target.CueID {
				index = i
				break
			}
		}
	}
	if index < 0 || index >= len(cues) {
		return show.Cue{}, -1, false
	}
	return cues[index], index, true
}

func nextLinkFallsPastEnd(target show.CueTarget, sourceIndex, cueCount int) bool {
	return (target.Kind == show.CueTargetNone || target.Kind == show.CueTargetNext) &&
		cueCount > 0 && sourceIndex == cueCount-1
}

func linkMatches(mode show.CueLinkMode, moment linkMoment) bool {
	return (moment == linkStart && (mode == show.CueLinkStartAdvance || mode == show.CueLinkStartPlay)) ||
		(moment == linkFadeIn && (mode == show.CueLinkFadeInAdvance || mode == show.CueLinkFadeInPlay)) ||
		(moment == linkFadeOut && (mode == show.CueLinkFadeOutAdvance || mode == show.CueLinkFadeOutPlay)) ||
		(moment == linkEnd && (mode == show.CueLinkEndAdvance || mode == show.CueLinkEndPlay))
}

func isAdvanceLink(mode show.CueLinkMode) bool {
	return mode == show.CueLinkStartAdvance || mode == show.CueLinkFadeInAdvance ||
		mode == show.CueLinkFadeOutAdvance || mode == show.CueLinkEndAdvance
}

func (e *Engine) scheduleLink(source show.Cue, sourceIndex int, delayMs int64, moment linkMoment, runCtx context.Context) {
	e.links.schedule(source, sourceIndex, delayMs, moment, runCtx)
}
