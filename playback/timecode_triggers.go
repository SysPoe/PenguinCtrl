package playback

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/syspoe/cusus/show"
)

type timecodeTriggers struct {
	engine *Engine
}

func newTimecodeTriggers(engine *Engine) *timecodeTriggers {
	return &timecodeTriggers{engine: engine}
}

func (triggers *timecodeTriggers) schedule(instanceID string, cue show.Cue, cueIndex int) {
	e := triggers.engine
	markers := mediaTimecode(cue)
	sort.SliceStable(markers, func(i, j int) bool { return markers[i].TimeMs < markers[j].TimeMs })
	e.mu.RLock()
	timeline := e.timeline
	parent := e.instances.get(instanceID)
	parentRun := cueRunToken{}
	if parent != nil {
		parentRun = parent.run
	}
	e.mu.RUnlock()
	if parentRun.ctx == nil {
		return
	}
	external := timeline != nil && timeline.Enabled()
	base := time.Duration(0)
	if external {
		base = timeline.Position()
	}
	for _, marker := range markers {
		if marker.Disabled || marker.TimeMs < 0 {
			continue
		}
		e.goOwned(func() {
			var reached bool
			if external {
				reached = timeline.WaitUntil(parentRun.ctx, base+time.Duration(marker.TimeMs)*time.Millisecond)
			} else {
				reached = waitContext(parentRun.ctx, time.Duration(marker.TimeMs)*time.Millisecond)
			}
			if !reached || !e.hasInstance(instanceID) {
				return
			}
			action := marker.Action
			if mediaControl := action.MediaControl(); mediaControl != nil {
				control := *mediaControl
				control.Target = show.MediaTarget{Kind: show.MediaTargetInstance, InstanceID: instanceID}
				action = show.NewTimecodeMediaAction(&control)
			}
			embedded := show.Cue{
				ID: cue.ID, CueNumber: cue.CueNumber, Description: cue.Description,
				Type: action.CueType(), Play: action.CuePlay(), Link: show.CueLink{Mode: show.CueLinkManual},
			}
			_ = e.enqueueEmbeddedCommand(embedded, cueIndex, "Timecode at "+formatPlaybackTime(marker.TimeMs), parentRun)
		})
	}
}

func mediaTimecode(cue show.Cue) []show.TimecodeMarker {
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			return append([]show.TimecodeMarker(nil), cue.Play.Sound.Timecode...)
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			return append([]show.TimecodeMarker(nil), cue.Play.Video.Timecode...)
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			return append([]show.TimecodeMarker(nil), cue.Play.Image.Timecode...)
		}
	}
	return nil
}

func formatPlaybackTime(ms int64) string {
	ms = max(int64(0), ms)
	return fmt.Sprintf("%02d:%02d.%03d", ms/60000, (ms%60000)/1000, ms%1000)
}

func cueDisplayNumber(cue show.Cue) string {
	if strings.TrimSpace(cue.CueNumber) == "" {
		return "an unnumbered cue"
	}
	return "cue " + cue.CueNumber
}
