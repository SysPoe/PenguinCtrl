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
	reference, ok := showAssetReferenceForCue(&cue)
	if !ok {
		return nil
	}
	source := config.Resolve(reference.Path(), settings, reference.CueNumber)
	source = strings.TrimSpace(source)
	if source == "" || strings.Contains(source, "{") {
		return nil
	}
	return []string{source}
}
