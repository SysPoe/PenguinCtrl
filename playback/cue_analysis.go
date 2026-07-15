package playback

import "github.com/syspoe/cusus/show"

// CueAnalysis evaluates show-document problems against media metadata and an
// optional live runtime snapshot without exposing playback controls.
type CueAnalysis interface {
	Problems(show.Cue) []show.CueProblem
}

type cueAnalyzer struct {
	settings SettingsAccess
	show     ShowAccess
	media    *mediaCatalog
	matching func(show.MediaTarget) []Instance
}

func newCueAnalyzer(settings SettingsAccess, showAccess ShowAccess, media *mediaCatalog, matching func(show.MediaTarget) []Instance) *cueAnalyzer {
	return &cueAnalyzer{settings: settings, show: showAccess, media: media, matching: matching}
}

func (analyzer *cueAnalyzer) Problems(cue show.Cue) []show.CueProblem {
	settings := analyzer.settings.Snapshot()
	source, start, end, configured, _ := durationDetails(cue, settings)
	key := durationCacheKey(cue.Type, source, start, end, configured)
	metadata := analyzer.media.warning(cue.ID, key)
	context := show.WarningContext{
		Settings: settings, KnownDurationMs: metadata.durationMs, MediaProbeError: metadata.probeError,
		TrackMediaCheck: metadata.trackValidation, MediaCheckPending: metadata.validationPending, MediaChecked: metadata.validationChecked,
	}
	if cue.Type == show.CueTypeMediaControl && cue.Play.MediaControl != nil {
		context.HasRuntimeState = true
		context.ActiveMediaMatches = analyzer.activeMatches(cue.Play.MediaControl.Target)
	}
	if cue.Type == show.CueTypeWait && cue.Play.Wait != nil && cue.Play.Wait.Kind != show.WaitDuration {
		context.HasRuntimeState = true
		context.ActiveMediaMatches = analyzer.activeMatches(cue.Play.Wait.Media)
	}
	return show.CueProblemsWithContext(cue, analyzer.show.Snapshot(), context)
}

func (analyzer *cueAnalyzer) activeMatches(target show.MediaTarget) int {
	if analyzer.matching == nil {
		return 0
	}
	return len(analyzer.matching(target))
}

var _ CueAnalysis = (*cueAnalyzer)(nil)
