package ui

import (
	"math"
	"strings"

	"gioui.org/io/pointer"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

type timelineDragMode uint8

const (
	timelineHeightDp = unit.Dp(190)

	timelineDragNone timelineDragMode = iota
	timelineDragPlayhead
	timelineDragMarkers
	timelineDragRange
	timelineDragPan
	timelineDragClipStart
	timelineDragClipEnd
	timelineDragFadeIn
	timelineDragFadeOut
	timelineDragActionDuration
)

// timecodeEditor owns the timeline document, Gio-facing interaction state,
// waveform lifecycle, and rendering. The adapter is rebound by CueEditUI so the
// component can edit cue/media data without reaching through the editor shell.
type timecodeEditor struct {
	adapter timecodeEditorAdapter

	tag   struct{}
	model timecodeTimelineModel

	add, preview widget.Clickable
	undo, redo   widget.Clickable
	previewing   bool

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
	defaultClipEnd bool

	dragMode        timelineDragMode
	dragPointer     pointer.ID
	dragStartX      float32
	dragLastX       float32
	dragStartMs     int64
	dragCurrentMs   int64
	dragClipStartMs int64
	dragClipEndMs   int64
	dragViewMs      int64
	dragIndexes     []int
	dragIndex       int
	dragMarkerTimes []int64
	dragChanged     bool
}

type timecodeEditorAdapter struct {
	cue            *show.Cue
	mediaInputs    func() *mediaPlayInputs
	markerInputs   *[]timecodeMarkerInputs
	mediaRangeRows func(*material.Theme, mediaRangeLabels, bool) []cueEditFormRow
	loadWaveform   func(source string, completed func(samples []float32, sampleRate int, durationMs int64, err error))
	togglePreview  func(cue show.Cue) (bool, error)
	stopPreview    func()
	previewError   *string
}

type timecodeClipboard struct {
	Format  string                `json:"format"`
	Markers []show.TimecodeMarker `json:"markers"`
}

func (t *timecodeEditor) reset() {
	adapter := t.adapter
	*t = timecodeEditor{adapter: adapter, zoom: 1}
}

func (t *timecodeEditor) ensure() {
	if t.model.selected == nil {
		t.model.selected = map[int]bool{}
	}
	if t.zoom < 1 {
		t.zoom = 1
	}
}

func (t *timecodeEditor) cue() *show.Cue {
	return t.adapter.cue
}

func (t *timecodeEditor) markers() *[]show.TimecodeMarker {
	markers := cueTimecodeMarkers(t.cue())
	if markers == nil {
		return nil
	}
	t.model.ensure(*markers)
	return &t.model.markers
}

func (t *timecodeEditor) syncMarkers() {
	markers := cueTimecodeMarkers(t.cue())
	if markers != nil && t.model.initialized {
		*markers = t.model.snapshot()
	}
}

func (t *timecodeEditor) mediaDetails() (source string, clipStartMs, clipEndMs, configuredDurationMs int64) {
	cue := t.cue()
	if cue == nil {
		return "", 0, 0, 0
	}
	switch {
	case cue.Play.Sound != nil:
		p := cue.Play.Sound
		return p.File, max(int64(0), p.ClipStartMs), p.ClipEndMs, 0
	case cue.Play.Video != nil:
		p := cue.Play.Video
		return p.File, max(int64(0), p.ClipStartMs), p.ClipEndMs, 0
	case cue.Play.Image != nil:
		return "", 0, cue.Play.Image.DurationMs, cue.Play.Image.DurationMs
	}
	return "", 0, 0, 0
}

func (t *timecodeEditor) clipRange() (startMs, endMs int64, ok bool) {
	cue := t.cue()
	if cue == nil {
		return 0, 0, false
	}
	switch {
	case cue.Play.Sound != nil:
		return cue.Play.Sound.ClipStartMs, cue.Play.Sound.ClipEndMs, true
	case cue.Play.Video != nil:
		return cue.Play.Video.ClipStartMs, cue.Play.Video.ClipEndMs, true
	default:
		return 0, 0, false
	}
}

func clampMediaClipRange(startMs, endMs, trackDurationMs int64, defaultEnd bool) (int64, int64) {
	startMs, endMs = max(int64(0), startMs), max(int64(0), endMs)
	if trackDurationMs <= 0 {
		if endMs > 0 && startMs >= endMs {
			startMs = max(int64(0), endMs-1)
		}
		return startMs, endMs
	}
	if defaultEnd || endMs <= 0 || endMs > trackDurationMs {
		endMs = trackDurationMs
	}
	startMs = min(startMs, max(int64(0), endMs-1))
	return startMs, endMs
}

func (t *timecodeEditor) applyClipRange(startMs, endMs int64) {
	cue := t.cue()
	if cue == nil {
		return
	}
	switch {
	case cue.Play.Sound != nil:
		cue.Play.Sound.ClipStartMs, cue.Play.Sound.ClipEndMs = startMs, endMs
	case cue.Play.Video != nil:
		cue.Play.Video.ClipStartMs, cue.Play.Video.ClipEndMs = startMs, endMs
	}
	if t.adapter.mediaInputs != nil {
		if fields := t.adapter.mediaInputs(); fields != nil && fields.clipStartMs != nil && fields.clipEndMs != nil {
			fields.clipStartMs.Value, fields.clipEndMs.Value = int(startMs), int(endMs)
		}
	}
}

func (t *timecodeEditor) syncClipToTrack(defaultEnd bool) {
	startMs, endMs, ok := t.clipRange()
	if !ok {
		return
	}
	startMs, endMs = clampMediaClipRange(startMs, endMs, t.waveDurationMs, defaultEnd)
	t.applyClipRange(startMs, endMs)
}

func (t *timecodeEditor) setClipStart(startMs int64) {
	_, endMs, ok := t.clipRange()
	if !ok {
		return
	}
	trackDurationMs := t.waveDurationMs
	if trackDurationMs > 0 && (endMs <= 0 || endMs > trackDurationMs) {
		endMs = trackDurationMs
	}
	limit := trackDurationMs
	if endMs > 0 {
		limit = endMs
	}
	startMs = max(int64(0), startMs)
	if limit > 0 {
		startMs = min(startMs, max(int64(0), limit-1))
	}
	t.applyClipRange(startMs, endMs)
	t.updateDuration()
}

func (t *timecodeEditor) setClipEnd(endMs int64) {
	startMs, _, ok := t.clipRange()
	if !ok {
		return
	}
	trackDurationMs := t.waveDurationMs
	startMs = max(int64(0), startMs)
	if trackDurationMs > 0 {
		startMs = min(startMs, max(int64(0), trackDurationMs-1))
		if endMs <= 0 {
			endMs = trackDurationMs
		}
		endMs = min(trackDurationMs, max(startMs+1, endMs))
	} else {
		endMs = max(int64(0), endMs)
		if endMs > 0 {
			endMs = max(startMs+1, endMs)
		}
	}
	t.applyClipRange(startMs, endMs)
	t.updateDuration()
}

func (t *timecodeEditor) setMediaSource(file *string, clipEndMs *int64, endInput *input.Integer, value string) {
	changed := !sameFilePath(*file, value)
	*file = value
	if !changed {
		return
	}
	*clipEndMs = 0
	if endInput != nil {
		endInput.Value = 0
	}
	t.defaultClipEnd = true
	t.ensureWaveform()
}

func (t *timecodeEditor) fades() (fadeInMs, fadeOutMs int64) {
	cue := t.cue()
	if cue == nil {
		return 0, 0
	}
	switch {
	case cue.Play.Sound != nil:
		return max(int64(0), cue.Play.Sound.FadeInMs), max(int64(0), cue.Play.Sound.FadeOutMs)
	case cue.Play.Video != nil:
		return max(int64(0), cue.Play.Video.FadeInMs), max(int64(0), cue.Play.Video.FadeOutMs)
	case cue.Play.Image != nil:
		return max(int64(0), cue.Play.Image.FadeInMs), max(int64(0), cue.Play.Image.FadeOutMs)
	}
	return 0, 0
}

func (t *timecodeEditor) setFades(fadeInMs, fadeOutMs int64) {
	fadeInMs, fadeOutMs = max(int64(0), fadeInMs), max(int64(0), fadeOutMs)
	cue := t.cue()
	if cue == nil {
		return
	}
	switch {
	case cue.Play.Sound != nil:
		cue.Play.Sound.FadeInMs, cue.Play.Sound.FadeOutMs = fadeInMs, fadeOutMs
	case cue.Play.Video != nil:
		cue.Play.Video.FadeInMs, cue.Play.Video.FadeOutMs = fadeInMs, fadeOutMs
	case cue.Play.Image != nil:
		cue.Play.Image.FadeInMs, cue.Play.Image.FadeOutMs = fadeInMs, fadeOutMs
	}
	if t.adapter.mediaInputs != nil {
		if fields := t.adapter.mediaInputs(); fields != nil {
			fields.fadeInMs.Value, fields.fadeOutMs.Value = int(fadeInMs), int(fadeOutMs)
		}
	}
}

func (t *timecodeEditor) ensureWaveform() {
	t.ensure()
	source, _, _, configured := t.mediaDetails()
	if configured > 0 {
		t.durationMs = configured
	}
	if source == t.source {
		t.syncClipToTrack(false)
		t.updateDuration()
		return
	}
	t.source, t.waveSamples, t.waveError = source, nil, ""
	t.waveSampleRate, t.waveDurationMs = 0, 0
	t.loading = false
	t.loadSerial++
	serial := t.loadSerial
	if strings.TrimSpace(source) == "" || t.adapter.loadWaveform == nil {
		t.updateDuration()
		return
	}
	t.loading = true
	t.adapter.loadWaveform(source, func(samples []float32, sampleRate int, durationMs int64, err error) {
		if serial != t.loadSerial || source != t.source {
			return
		}
		t.loading = false
		t.waveSamples, t.waveSampleRate, t.waveDurationMs = samples, sampleRate, durationMs
		if err != nil {
			t.waveError = err.Error()
		}
		t.syncClipToTrack(t.defaultClipEnd)
		t.defaultClipEnd = false
		t.updateDuration()
	})
	t.updateDuration()
}

func (t *timecodeEditor) updateDuration() {
	_, clipStart, clipEnd, _ := t.mediaDetails()
	duration := t.waveDurationMs
	if duration <= 0 && clipEnd > 0 {
		duration = clipEnd
	}
	if cue := t.cue(); cue != nil && cue.Play.Image != nil {
		duration = cue.Play.Image.DurationMs
	}
	if duration <= 0 {
		if markers := t.markers(); markers != nil {
			for _, marker := range *markers {
				duration = max(duration, clipStart+marker.TimeMs+timelineMinDurationMs)
			}
		}
	}
	t.durationMs = max(int64(timelineMinDurationMs), duration)
	t.playheadMs = min(max(int64(0), t.playheadMs), t.cueDuration())
	t.clampView()
}

func (t *timecodeEditor) cueDuration() int64 {
	startMs, endMs, ok := t.clipRange()
	if ok && endMs > startMs {
		return endMs - startMs
	}
	if cue := t.cue(); cue != nil && cue.Play.Image != nil {
		return max(int64(0), cue.Play.Image.DurationMs)
	}
	return max(int64(0), t.durationMs-startMs)
}

func (t *timecodeEditor) cueToTrackMs(cueMs int64) int64 {
	startMs, _, ok := t.clipRange()
	if !ok {
		startMs = 0
	}
	return startMs + cueMs
}

func (t *timecodeEditor) trackToCueMs(trackMs int64) int64 {
	startMs, _, ok := t.clipRange()
	if !ok {
		startMs = 0
	}
	return min(t.cueDuration(), max(int64(0), trackMs-startMs))
}

func (t *timecodeEditor) viewDuration() int64 {
	return max(int64(1), int64(float64(t.durationMs)/t.zoom))
}

func (t *timecodeEditor) clampView() {
	t.viewStartMs = min(max(int64(0), t.viewStartMs), max(int64(0), t.durationMs-t.viewDuration()))
}

func (t *timecodeEditor) xToMs(x float32, width int) int64 {
	if width <= 0 {
		return t.viewStartMs
	}
	ratio := math.Max(0, math.Min(1, float64(x)/float64(width)))
	return min(t.durationMs, max(int64(0), t.viewStartMs+int64(ratio*float64(t.viewDuration()))))
}

func (t *timecodeEditor) msToX(ms int64, width int) int {
	if width <= 0 {
		return 0
	}
	return int(float64(ms-t.viewStartMs) / float64(t.viewDuration()) * float64(width))
}

func (t *timecodeEditor) undoEdit() {
	if t.model.undo() {
		t.resetInputs()
		t.syncMarkers()
	}
}

func (t *timecodeEditor) redoEdit() {
	if t.model.redo() {
		t.resetInputs()
		t.syncMarkers()
	}
}

func (t *timecodeEditor) resetInputs() {
	markers := t.markers()
	inputs := t.adapter.markerInputs
	if inputs == nil {
		return
	}
	if markers == nil {
		*inputs = nil
		return
	}
	*inputs = make([]timecodeMarkerInputs, len(*markers))
	for index, marker := range *markers {
		(*inputs)[index] = newTimecodeMarkerInputs(marker)
	}
}

func (t *timecodeEditor) togglePreview() {
	if t.adapter.togglePreview == nil {
		return
	}
	cue := t.cue()
	if cue == nil || cue.Play.Sound == nil {
		return
	}
	playing, err := t.adapter.togglePreview(*cue)
	if err != nil {
		t.previewing = false
		if t.adapter.previewError != nil {
			*t.adapter.previewError = err.Error()
		}
		return
	}
	if t.adapter.previewError != nil {
		*t.adapter.previewError = ""
	}
	t.previewing = playing
}

func (t *timecodeEditor) stopPreview() {
	if t.adapter.stopPreview != nil {
		t.adapter.stopPreview()
	}
	t.previewing = false
	if t.adapter.previewError != nil {
		*t.adapter.previewError = ""
	}
}

// Compatibility entry points keep the existing cue-tab binders and host close
// path stable while all timeline behavior lives on timecodeEditor.
func (ctx *CueEditUI) timelineMarkers() *[]show.TimecodeMarker {
	return ctx.timecodeEditor().markers()
}

func (ctx *CueEditUI) syncTimelineMarkers() {
	ctx.timecodeEditor().syncMarkers()
}

func (ctx *CueEditUI) setTimecodeMediaSource(file *string, clipEndMs *int64, endInput *input.Integer, value string) {
	ctx.timecodeEditor().setMediaSource(file, clipEndMs, endInput, value)
}

func (ctx *CueEditUI) setTimecodeClipStart(startMs int64) {
	ctx.timecodeEditor().setClipStart(startMs)
}

func (ctx *CueEditUI) setTimecodeClipEnd(endMs int64) {
	ctx.timecodeEditor().setClipEnd(endMs)
}

func (ctx *CueEditUI) resetTimecodeInputs() {
	ctx.timecodeEditor().resetInputs()
}
