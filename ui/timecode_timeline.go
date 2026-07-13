package ui

import (
	"math"
	"sort"
	"strings"

	"gioui.org/io/pointer"
	"gioui.org/widget"
	"github.com/syspoe/cusus/show"
)

type timelineDragMode uint8

const (
	timelineDragNone timelineDragMode = iota
	timelineDragPlayhead
	timelineDragMarkers
	timelineDragRange
	timelineDragPan
	timelineDragFadeIn
	timelineDragFadeOut
	timelineDragActionDuration
)

type timecodeTimelineState struct {
	tag struct{}

	add, preview widget.Clickable
	undo, redo   widget.Clickable
	previewing   bool

	selected    map[int]bool
	clipboard   []show.TimecodeMarker
	playheadMs  int64
	hoverMs     int64
	durationMs  int64
	viewStartMs int64
	zoom        float64

	source         string
	loading        bool
	loadSerial     uint64
	waveSamples    []float32
	waveSampleRate int
	waveDurationMs int64
	waveError      string

	dragMode        timelineDragMode
	dragPointer     pointer.ID
	dragStartX      float32
	dragLastX       float32
	dragStartMs     int64
	dragCurrentMs   int64
	dragIndexes     []int
	dragIndex       int
	dragMarkerTimes []int64
	dragChanged     bool
	history         [][]show.TimecodeMarker
	future          [][]show.TimecodeMarker
}

type timecodeClipboard struct {
	Format  string                `json:"format"`
	Markers []show.TimecodeMarker `json:"markers"`
}

func (t *timecodeTimelineState) reset() {
	*t = timecodeTimelineState{selected: map[int]bool{}, zoom: 1}
}

func (t *timecodeTimelineState) ensure() {
	if t.selected == nil {
		t.selected = map[int]bool{}
	}
	if t.zoom < 1 {
		t.zoom = 1
	}
}

func (ctx *CueEditUI) timecodeMediaDetails() (source string, clipStartMs, clipEndMs, configuredDurationMs int64) {
	switch {
	case ctx.cue.Play.Sound != nil:
		p := ctx.cue.Play.Sound
		return p.File, max(int64(0), p.ClipStartMs), p.ClipEndMs, 0
	case ctx.cue.Play.Video != nil:
		p := ctx.cue.Play.Video
		return p.File, max(int64(0), p.ClipStartMs), p.ClipEndMs, 0
	case ctx.cue.Play.Image != nil:
		return "", 0, ctx.cue.Play.Image.DurationMs, ctx.cue.Play.Image.DurationMs
	}
	return "", 0, 0, 0
}

func (ctx *CueEditUI) timecodeFades() (fadeInMs, fadeOutMs int64) {
	switch {
	case ctx.cue.Play.Sound != nil:
		return max(int64(0), ctx.cue.Play.Sound.FadeInMs), max(int64(0), ctx.cue.Play.Sound.FadeOutMs)
	case ctx.cue.Play.Video != nil:
		return max(int64(0), ctx.cue.Play.Video.FadeInMs), max(int64(0), ctx.cue.Play.Video.FadeOutMs)
	case ctx.cue.Play.Image != nil:
		return max(int64(0), ctx.cue.Play.Image.FadeInMs), max(int64(0), ctx.cue.Play.Image.FadeOutMs)
	}
	return 0, 0
}

func (ctx *CueEditUI) setTimecodeFades(fadeInMs, fadeOutMs int64) {
	fadeInMs, fadeOutMs = max(int64(0), fadeInMs), max(int64(0), fadeOutMs)
	switch {
	case ctx.cue.Play.Sound != nil:
		ctx.cue.Play.Sound.FadeInMs, ctx.cue.Play.Sound.FadeOutMs = fadeInMs, fadeOutMs
		ctx.page.integer["soundFadeInMs"].Value, ctx.page.integer["soundFadeOutMs"].Value = int(fadeInMs), int(fadeOutMs)
	case ctx.cue.Play.Video != nil:
		ctx.cue.Play.Video.FadeInMs, ctx.cue.Play.Video.FadeOutMs = fadeInMs, fadeOutMs
		ctx.page.integer["videoFadeInMs"].Value, ctx.page.integer["videoFadeOutMs"].Value = int(fadeInMs), int(fadeOutMs)
	case ctx.cue.Play.Image != nil:
		ctx.cue.Play.Image.FadeInMs, ctx.cue.Play.Image.FadeOutMs = fadeInMs, fadeOutMs
		ctx.page.integer["imageFadeInMs"].Value, ctx.page.integer["imageFadeOutMs"].Value = int(fadeInMs), int(fadeOutMs)
	}
}

func markerActionDuration(marker *show.TimecodeMarker) *int64 {
	if marker == nil || marker.Type != show.CueTypeMediaControl || marker.Action.MediaControl == nil {
		return nil
	}
	switch marker.Action.MediaControl.Action {
	case show.MediaControlFadeTo, show.MediaControlFadeOut, show.MediaControlStop, show.MediaControlSetVolume:
		return &marker.Action.MediaControl.FadeMs
	}
	return nil
}

func (ctx *CueEditUI) ensureTimelineWaveform() {
	t := &ctx.timeline
	t.ensure()
	source, clipStart, clipEnd, configured := ctx.timecodeMediaDetails()
	if configured > 0 {
		t.durationMs = configured
	}
	if source == t.source {
		ctx.updateTimelineDuration(clipStart, clipEnd)
		return
	}
	t.source, t.waveSamples, t.waveError = source, nil, ""
	t.waveSampleRate, t.waveDurationMs = 0, 0
	t.loading = false
	t.loadSerial++
	serial := t.loadSerial
	if strings.TrimSpace(source) == "" || ctx.loadWaveform == nil {
		ctx.updateTimelineDuration(clipStart, clipEnd)
		return
	}
	t.loading = true
	ctx.loadWaveform(source, func(samples []float32, sampleRate int, durationMs int64, err error) {
		if serial != t.loadSerial || source != t.source {
			return
		}
		t.loading = false
		t.waveSamples, t.waveSampleRate, t.waveDurationMs = samples, sampleRate, durationMs
		if err != nil {
			t.waveError = err.Error()
		}
		ctx.updateTimelineDuration(clipStart, clipEnd)
	})
	ctx.updateTimelineDuration(clipStart, clipEnd)
}

func (ctx *CueEditUI) updateTimelineDuration(clipStart, clipEnd int64) {
	t := &ctx.timeline
	duration := t.waveDurationMs - clipStart
	if clipEnd > clipStart {
		duration = clipEnd - clipStart
	}
	if ctx.cue.Play.Image != nil {
		duration = ctx.cue.Play.Image.DurationMs
	}
	if markers := cueTimecodeMarkers(&ctx.cue); markers != nil {
		for _, marker := range *markers {
			duration = max(duration, marker.TimeMs+1000)
		}
	}
	t.durationMs = max(int64(1000), duration)
	t.playheadMs = min(max(int64(0), t.playheadMs), t.durationMs)
	t.clampView()
}

func (t *timecodeTimelineState) viewDuration() int64 {
	return max(int64(1), int64(float64(t.durationMs)/t.zoom))
}

func (t *timecodeTimelineState) clampView() {
	t.viewStartMs = min(max(int64(0), t.viewStartMs), max(int64(0), t.durationMs-t.viewDuration()))
}

func (t *timecodeTimelineState) xToMs(x float32, width int) int64 {
	if width <= 0 {
		return t.viewStartMs
	}
	ratio := math.Max(0, math.Min(1, float64(x)/float64(width)))
	return min(t.durationMs, max(int64(0), t.viewStartMs+int64(ratio*float64(t.viewDuration()))))
}

func (t *timecodeTimelineState) msToX(ms int64, width int) int {
	if width <= 0 {
		return 0
	}
	return int(float64(ms-t.viewStartMs) / float64(t.viewDuration()) * float64(width))
}

func selectedTimelineIndexes(t *timecodeTimelineState, count int) []int {
	result := make([]int, 0, len(t.selected))
	for i := range t.selected {
		if i >= 0 && i < count {
			result = append(result, i)
		}
	}
	sort.Ints(result)
	return result
}

func sortTimecodeMarkers(markers *[]show.TimecodeMarker) bool {
	if markers == nil {
		return false
	}
	changed := false
	for i := range *markers {
		marker := &(*markers)[i]
		if marker.Type == show.CueTypeMediaControl && marker.Action.MediaControl != nil && marker.Action.MediaControl.Target.Kind != show.MediaTargetCurrentTrack {
			marker.Action.MediaControl.Target = show.MediaTarget{Kind: show.MediaTargetCurrentTrack}
			changed = true
		}
	}
	if len(*markers) >= 2 && !sort.SliceIsSorted(*markers, func(i, j int) bool { return (*markers)[i].TimeMs < (*markers)[j].TimeMs }) {
		sort.SliceStable(*markers, func(i, j int) bool { return (*markers)[i].TimeMs < (*markers)[j].TimeMs })
		changed = true
	}
	return changed
}

func timecodeActionIndex(cueType show.CueType) int {
	switch cueType {
	case show.CueTypeOutputControl:
		return 1
	case show.CueTypeRemote:
		return 2
	default:
		return 0
	}
}

func defaultTimecodeMediaControl() *show.MediaControlPlay {
	level := 0.0
	return &show.MediaControlPlay{
		Action:  show.MediaControlFadeTo,
		Target:  show.MediaTarget{Kind: show.MediaTargetCurrentTrack},
		LevelDB: &level,
		FadeMs:  1000,
		Curve:   show.FadeCurveLinear,
	}
}

func newTimecodeMarker(at int64) show.TimecodeMarker {
	return show.TimecodeMarker{
		TimeMs: max(int64(0), at),
		Type:   show.CueTypeMediaControl,
		Action: show.CuePlay{MediaControl: defaultTimecodeMediaControl()},
	}
}

func setTimecodeActionType(marker *show.TimecodeMarker, selected int) {
	switch selected {
	case 1:
		marker.Type = show.CueTypeOutputControl
		marker.Action = show.NewOutputControlCue().Play
	case 2:
		marker.Type = show.CueTypeRemote
		marker.Action = show.NewRemoteCue().Play
	default:
		marker.Type = show.CueTypeMediaControl
		marker.Action = show.CuePlay{MediaControl: defaultTimecodeMediaControl()}
	}
}

func cloneTimecodeMarkers(markers []show.TimecodeMarker) []show.TimecodeMarker {
	cloned := append([]show.TimecodeMarker(nil), markers...)
	for i := range cloned {
		if value := markers[i].Action.MediaControl; value != nil {
			copy := *value
			if value.LevelDB != nil {
				level := *value.LevelDB
				copy.LevelDB = &level
			}
			if value.SeekToMs != nil {
				seek := *value.SeekToMs
				copy.SeekToMs = &seek
			}
			cloned[i].Action.MediaControl = &copy
		}
		if value := markers[i].Action.OutputControl; value != nil {
			copy := *value
			cloned[i].Action.OutputControl = &copy
		}
		if value := markers[i].Action.Remote; value != nil {
			copy := *value
			copy.Values = append([]show.RemoteValue(nil), value.Values...)
			cloned[i].Action.Remote = &copy
		}
	}
	return cloned
}

func (t *timecodeTimelineState) checkpoint(markers []show.TimecodeMarker) {
	t.history = append(t.history, cloneTimecodeMarkers(markers))
	if len(t.history) > 100 {
		t.history = t.history[len(t.history)-100:]
	}
	t.future = nil
}

func (ctx *CueEditUI) undoTimeline(markers *[]show.TimecodeMarker) bool {
	t := &ctx.timeline
	if len(t.history) == 0 {
		return false
	}
	t.future = append(t.future, cloneTimecodeMarkers(*markers))
	*markers = t.history[len(t.history)-1]
	t.history = t.history[:len(t.history)-1]
	t.selected = map[int]bool{}
	ctx.resetTimecodeInputs()
	return true
}

func (ctx *CueEditUI) redoTimeline(markers *[]show.TimecodeMarker) bool {
	t := &ctx.timeline
	if len(t.future) == 0 {
		return false
	}
	t.history = append(t.history, cloneTimecodeMarkers(*markers))
	*markers = t.future[len(t.future)-1]
	t.future = t.future[:len(t.future)-1]
	t.selected = map[int]bool{}
	ctx.resetTimecodeInputs()
	return true
}

func (ctx *CueEditUI) resetTimecodeInputs() {
	markers := cueTimecodeMarkers(&ctx.cue)
	if markers == nil {
		return
	}
	for key := range ctx.page.integer {
		if strings.HasPrefix(key, "timecode.") {
			delete(ctx.page.integer, key)
		}
	}
	for key := range ctx.page.checkbox {
		if strings.HasPrefix(key, "timecode.") {
			delete(ctx.page.checkbox, key)
		}
	}
	for key := range ctx.page.dropdown {
		if strings.HasPrefix(key, "timecode.") {
			delete(ctx.page.dropdown, key)
		}
	}
	for key := range ctx.page.text {
		if strings.HasPrefix(key, "timecode.") {
			delete(ctx.page.text, key)
		}
	}
	for key := range ctx.page.float {
		if strings.HasPrefix(key, "timecode.") {
			delete(ctx.page.float, key)
		}
	}
	for i, marker := range *markers {
		initTimecodeMarkerInputs(&ctx.page, i, marker)
	}
}

// Tiny constructors keep timecode_timeline.go independent from input widget internals.
