package playback

import (
	"errors"
	"sync"

	"github.com/syspoe/cusus/show"
)

type previewSession struct {
	mu     sync.RWMutex
	cueID  show.CueID
	paused bool
}

func (p *previewSession) snapshot() (show.CueID, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cueID, p.paused
}

func (p *previewSession) begin(cueID show.CueID) {
	p.mu.Lock()
	p.cueID = cueID
	p.paused = false
	p.mu.Unlock()
}

func (p *previewSession) setPaused(cueID show.CueID, paused bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cueID != cueID {
		return false
	}
	p.paused = paused
	return true
}

func (p *previewSession) clearIf(cueID show.CueID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cueID != cueID {
		return false
	}
	p.cueID = show.CueID{}
	p.paused = false
	return true
}

func (p *previewSession) clear() show.CueID {
	p.mu.Lock()
	defer p.mu.Unlock()
	cueID := p.cueID
	p.cueID = show.CueID{}
	p.paused = false
	return cueID
}

// TogglePreview starts or pauses a sound-cue preview. Timecode and cue links
// are stripped so previewing cannot trigger show actions.
func (e *Engine) TogglePreview(cue show.Cue) (bool, error) {
	if cue.Play.Sound == nil {
		return false, errors.New("only sound cues can be previewed")
	}
	id, paused := e.preview.snapshot()
	if id != (show.CueID{}) && len(e.matchingInstances(show.MediaTarget{Kind: show.MediaTargetCue, CueID: id})) > 0 {
		action := show.MediaControlPause
		playing := false
		if paused {
			action, playing = show.MediaControlResume, true
		}
		if err := e.ControlMedia(show.MediaTarget{Kind: show.MediaTargetCue, CueID: id}, action, nil, nil, 0); err != nil {
			return !paused, err
		}
		e.preview.setPaused(id, !playing)
		return playing, nil
	}

	preview := show.CloneCue(cue)
	preview.ID = show.NewCueID()
	preview.GroupID, preview.GroupTitle = show.GroupID{}, ""
	preview.Timing = show.CueTiming{}
	preview.Link = show.CueLink{Mode: show.CueLinkManual}
	preview.Play.Sound.Timecode = nil
	e.preview.begin(preview.ID)
	if err := e.enqueueCommand(preview, -1, previewCommand, "Preview", rejectBlockers); err != nil {
		e.preview.clearIf(preview.ID)
		return false, err
	}
	return true, nil
}

func (e *Engine) StopPreview() {
	id := e.preview.clear()
	if id != (show.CueID{}) {
		_ = e.ControlMedia(show.MediaTarget{Kind: show.MediaTargetCue, CueID: id}, show.MediaControlStop, nil, nil, 0)
	}
}
