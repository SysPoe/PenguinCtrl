package playback

import (
	"sort"
	"time"

	"github.com/syspoe/cusus/show"
)

func (e *Engine) matchingInstances(target show.MediaTarget) []Instance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances.matching(target, time.Now())
}

func (e *Engine) instancesForOutput(outputID string) []Instance {
	return e.matchingInstances(show.MediaTarget{Kind: show.MediaTargetOutput, OutputID: outputID})
}

func (e *Engine) OutputIDs() []string {
	settings := e.settings.Snapshot()
	seen := map[string]struct{}{settings.DefaultMediaOutput: {}}
	for _, cue := range e.show.Snapshot() {
		var output string
		switch cue.Type {
		case show.CueTypeSound:
			if cue.Play.Sound != nil {
				output = cue.Play.Sound.OutputID
			}
		case show.CueTypeVideo:
			if cue.Play.Video != nil {
				output = cue.Play.Video.OutputID
			}
		case show.CueTypeImage:
			if cue.Play.Image != nil {
				output = cue.Play.Image.OutputID
			}
		case show.CueTypeOutputControl:
			if cue.Play.OutputControl != nil {
				output = cue.Play.OutputControl.OutputID
			}
		case show.CueTypeMediaControl:
			if cue.Play.MediaControl != nil && cue.Play.MediaControl.Target.Kind == show.MediaTargetOutput {
				output = cue.Play.MediaControl.Target.OutputID
			}
		}
		output = resolveOutput(output, settings, cue.CueNumber)
		if output != "" {
			seen[output] = struct{}{}
		}
	}
	for _, instance := range e.ActiveInstances() {
		seen[instance.OutputID] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for output := range seen {
		result = append(result, output)
	}
	sort.Strings(result)
	return result
}

func (e *Engine) hasMediaType(mediaType string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances.hasMediaType(mediaType)
}

func (e *Engine) instanceCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances.count()
}

func (e *Engine) hasInstance(id string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.instances.has(id)
}
