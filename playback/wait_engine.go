package playback

import (
	"context"
	"errors"
	"time"

	"github.com/syspoe/cusus/show"
)

const waitStatePollInterval = 100 * time.Millisecond

type waitEngine struct {
	engine *Engine
}

func newWaitEngine(engine *Engine) *waitEngine { return &waitEngine{engine: engine} }

func (waits *waitEngine) execute(cue show.Cue, runCtx context.Context) error {
	if cue.Play.Wait == nil {
		return errors.New("wait cue has no wait settings")
	}
	wait := cue.Play.Wait
	if wait.Kind == show.WaitDuration {
		if !waitContext(runCtx, time.Duration(max(0, wait.DurationMs))*time.Millisecond) {
			return runCtx.Err()
		}
		return nil
	}
	ticker := time.NewTicker(waitStatePollInterval)
	defer ticker.Stop()
	for {
		if waits.satisfied(*wait) {
			return nil
		}
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case <-waits.engine.stateEvent:
		case <-ticker.C:
		}
	}
}

func (waits *waitEngine) satisfied(wait show.WaitPlay) bool {
	e := waits.engine
	instances := e.matchingInstances(wait.Media)
	switch wait.Kind {
	case show.WaitMediaStart:
		return len(instances) > 0
	case show.WaitMediaEnd, show.WaitInstanceStopped, show.WaitFadeOutComplete:
		return len(instances) == 0
	case show.WaitAllAudioStopped:
		return !e.hasMediaType(MediaTypeAudio)
	case show.WaitAllVideoStopped:
		return !e.hasMediaType(MediaTypeVideo)
	case show.WaitAllMediaStopped:
		return e.instanceCount() == 0
	case show.WaitFadeInComplete:
		for _, instance := range instances {
			if !instance.FadeInComplete {
				return false
			}
		}
		return len(instances) > 0
	default:
		return false
	}
}
