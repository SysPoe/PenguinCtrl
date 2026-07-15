package project

import (
	"strings"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

// ResolvedMediaSources returns the concrete local media paths referenced by a
// cue. Unresolved templates are omitted because callers cannot safely stat,
// cache, package, or fingerprint them yet.
func ResolvedMediaSources(cue show.Cue, settings config.Settings) []string {
	var source string
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			source = config.Resolve(cue.Play.Sound.File, settings, cue.CueNumber)
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			source = config.Resolve(cue.Play.Video.File, settings, cue.CueNumber)
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			source = config.Resolve(cue.Play.Image.File, settings, cue.CueNumber)
		}
	}
	source = strings.TrimSpace(source)
	if source == "" || strings.Contains(source, "{") {
		return nil
	}
	return []string{source}
}
