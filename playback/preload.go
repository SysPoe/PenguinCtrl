package playback

import (
	"strings"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

// PreloadSpec contains only the immutable source and routing data needed to
// warm a future media session. It has no live instance identity or lifecycle.
type PreloadSpec struct {
	CueID       show.CueID
	CueNumber   string
	MediaType   string
	Source      string
	OutputID    string
	ClipStartMs int64
	ClipEndMs   int64
	Preview     bool
}

// PreloadCandidates returns the selected and following playable media cues in
// cue-list order. GO still creates the authoritative runtime instance.
func (e *Engine) PreloadCandidates(limit int) []PreloadSpec {
	_, selected, ok := e.show.SelectedCueCopy()
	if !ok || limit <= 0 {
		return nil
	}
	cues, settings := e.show.Snapshot(), e.settings.Snapshot()
	result := make([]PreloadSpec, 0, limit)
	for index := selected; index < len(cues) && len(result) < limit; index++ {
		cue := cues[index]
		if e.CueActive(cue.ID) {
			continue
		}
		spec := PreloadSpec{CueID: cue.ID, CueNumber: cue.CueNumber}
		switch cue.Type {
		case show.CueTypeSound:
			if cue.Play.Sound == nil {
				continue
			}
			play := cue.Play.Sound
			spec.MediaType, spec.Source = "audio", config.Resolve(play.File, settings, cue.CueNumber)
			spec.OutputID = resolveOutput(play.OutputID, settings, cue.CueNumber)
			spec.ClipStartMs, spec.ClipEndMs = play.ClipStartMs, play.ClipEndMs
		case show.CueTypeVideo:
			if cue.Play.Video == nil {
				continue
			}
			play := cue.Play.Video
			spec.MediaType, spec.Source = "video", config.Resolve(play.File, settings, cue.CueNumber)
			spec.OutputID = resolveOutput(play.OutputID, settings, cue.CueNumber)
			spec.ClipStartMs, spec.ClipEndMs = play.ClipStartMs, play.ClipEndMs
		default:
			continue
		}
		if strings.TrimSpace(spec.Source) != "" && !strings.Contains(spec.Source, "{") {
			result = append(result, spec)
		}
	}
	return result
}

// PreloadInstances is a deprecated compatibility adapter for media consumers
// migrating to PreloadSpec. New code should use PreloadCandidates directly.
func (e *Engine) PreloadInstances(limit int) []Instance {
	specs := e.PreloadCandidates(limit)
	instances := make([]Instance, 0, len(specs))
	for _, spec := range specs {
		instances = append(instances, Instance{
			CueID: spec.CueID, CueNumber: spec.CueNumber, MediaType: spec.MediaType,
			Source: spec.Source, OutputID: spec.OutputID, ClipStartMs: spec.ClipStartMs,
			ClipEndMs: spec.ClipEndMs, Preview: spec.Preview,
		})
	}
	return instances
}
