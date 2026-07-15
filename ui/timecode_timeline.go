package ui

import (
	"math"
	"strings"

	"gioui.org/io/pointer"
	"gioui.org/unit"
	"gioui.org/widget"
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

// timecodeTimelineState owns the Gio-facing timeline viewport, waveform, widgets,
// and in-progress gesture state. Its marker document and history live in model.
// TODO(macro): Promote the remaining CueEditUI input/render/waveform methods to a
// TimecodeEditor view component with an explicit cue/media adapter. The pure marker
// document is now separate, but clip/fade callbacks still reach through CueEditUI.
type timecodeTimelineState struct {
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

type timecodeClipboard struct {
	Format  string                `json:"format"`
	Markers []show.TimecodeMarker `json:"markers"`
}

func (t *timecodeTimelineState) reset() {
	*t = timecodeTimelineState{zoom: 1}
}

func (t *timecodeTimelineState) ensure() {
	if t.model.selected == nil {
		t.model.selected = map[int]bool{}
	}
	if t.zoom < 1 {
		t.zoom = 1
	}
}

func (ctx *CueEditUI) timelineMarkers() *[]show.TimecodeMarker {
	markers := cueTimecodeMarkers(&ctx.cue)
	if markers == nil {
		return nil
	}
	ctx.timeline.model.ensure(*markers)
	return &ctx.timeline.model.markers
}

func (ctx *CueEditUI) syncTimelineMarkers() {
	markers := cueTimecodeMarkers(&ctx.cue)
	if markers != nil && ctx.timeline.model.initialized {
		*markers = ctx.timeline.model.snapshot()
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

func (ctx *CueEditUI) timecodeClipRange() (startMs, endMs int64, ok bool) {
	switch {
	case ctx.cue.Play.Sound != nil:
		return ctx.cue.Play.Sound.ClipStartMs, ctx.cue.Play.Sound.ClipEndMs, true
	case ctx.cue.Play.Video != nil:
		return ctx.cue.Play.Video.ClipStartMs, ctx.cue.Play.Video.ClipEndMs, true
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

func (ctx *CueEditUI) applyTimecodeClipRange(startMs, endMs int64) {
	switch {
	case ctx.cue.Play.Sound != nil:
		ctx.cue.Play.Sound.ClipStartMs, ctx.cue.Play.Sound.ClipEndMs = startMs, endMs
	case ctx.cue.Play.Video != nil:
		ctx.cue.Play.Video.ClipStartMs, ctx.cue.Play.Video.ClipEndMs = startMs, endMs
	}
	if fields := ctx.page.media; fields != nil && fields.clipStartMs != nil && fields.clipEndMs != nil {
		fields.clipStartMs.Value, fields.clipEndMs.Value = int(startMs), int(endMs)
	}
}

func (ctx *CueEditUI) syncTimecodeClipToTrack(defaultEnd bool) {
	startMs, endMs, ok := ctx.timecodeClipRange()
	if !ok {
		return
	}
	startMs, endMs = clampMediaClipRange(startMs, endMs, ctx.timeline.waveDurationMs, defaultEnd)
	ctx.applyTimecodeClipRange(startMs, endMs)
}

func (ctx *CueEditUI) setTimecodeClipStart(startMs int64) {
	_, endMs, ok := ctx.timecodeClipRange()
	if !ok {
		return
	}
	trackDurationMs := ctx.timeline.waveDurationMs
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
	ctx.applyTimecodeClipRange(startMs, endMs)
	ctx.updateTimelineDuration()
}

func (ctx *CueEditUI) setTimecodeClipEnd(endMs int64) {
	startMs, _, ok := ctx.timecodeClipRange()
	if !ok {
		return
	}
	trackDurationMs := ctx.timeline.waveDurationMs
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
	ctx.applyTimecodeClipRange(startMs, endMs)
	ctx.updateTimelineDuration()
}

func (ctx *CueEditUI) setTimecodeMediaSource(file *string, clipEndMs *int64, endInput *input.Integer, value string) {
	changed := !sameFilePath(*file, value)
	*file = value
	if !changed {
		return
	}
	*clipEndMs = 0
	if endInput != nil {
		endInput.Value = 0
	}
	ctx.timeline.defaultClipEnd = true
	ctx.ensureTimelineWaveform()
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
	case ctx.cue.Play.Video != nil:
		ctx.cue.Play.Video.FadeInMs, ctx.cue.Play.Video.FadeOutMs = fadeInMs, fadeOutMs
	case ctx.cue.Play.Image != nil:
		ctx.cue.Play.Image.FadeInMs, ctx.cue.Play.Image.FadeOutMs = fadeInMs, fadeOutMs
	}
	if fields := ctx.page.media; fields != nil {
		fields.fadeInMs.Value, fields.fadeOutMs.Value = int(fadeInMs), int(fadeOutMs)
	}
}

func (ctx *CueEditUI) ensureTimelineWaveform() {
	t := &ctx.timeline
	t.ensure()
	source, _, _, configured := ctx.timecodeMediaDetails()
	if configured > 0 {
		t.durationMs = configured
	}
	if source == t.source {
		ctx.syncTimecodeClipToTrack(false)
		ctx.updateTimelineDuration()
		return
	}
	t.source, t.waveSamples, t.waveError = source, nil, ""
	t.waveSampleRate, t.waveDurationMs = 0, 0
	t.loading = false
	t.loadSerial++
	serial := t.loadSerial
	if strings.TrimSpace(source) == "" || ctx.loadWaveform == nil {
		ctx.updateTimelineDuration()
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
		ctx.syncTimecodeClipToTrack(t.defaultClipEnd)
		t.defaultClipEnd = false
		ctx.updateTimelineDuration()
	})
	ctx.updateTimelineDuration()
}

func (ctx *CueEditUI) updateTimelineDuration() {
	t := &ctx.timeline
	_, clipStart, clipEnd, _ := ctx.timecodeMediaDetails()
	duration := t.waveDurationMs
	if duration <= 0 && clipEnd > 0 {
		duration = clipEnd
	}
	if ctx.cue.Play.Image != nil {
		duration = ctx.cue.Play.Image.DurationMs
	}
	if duration <= 0 {
		if markers := ctx.timelineMarkers(); markers != nil {
			for _, marker := range *markers {
				duration = max(duration, clipStart+marker.TimeMs+timelineMinDurationMs)
			}
		}
	}
	t.durationMs = max(int64(timelineMinDurationMs), duration)
	t.playheadMs = min(max(int64(0), t.playheadMs), ctx.timecodeCueDuration())
	t.clampView()
}

func (ctx *CueEditUI) timecodeCueDuration() int64 {
	startMs, endMs, ok := ctx.timecodeClipRange()
	if ok && endMs > startMs {
		return endMs - startMs
	}
	if ctx.cue.Play.Image != nil {
		return max(int64(0), ctx.cue.Play.Image.DurationMs)
	}
	return max(int64(0), ctx.timeline.durationMs-startMs)
}

func (ctx *CueEditUI) timelineCueToTrackMs(cueMs int64) int64 {
	startMs, _, ok := ctx.timecodeClipRange()
	if !ok {
		startMs = 0
	}
	return startMs + cueMs
}

func (ctx *CueEditUI) timelineTrackToCueMs(trackMs int64) int64 {
	startMs, _, ok := ctx.timecodeClipRange()
	if !ok {
		startMs = 0
	}
	return min(ctx.timecodeCueDuration(), max(int64(0), trackMs-startMs))
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

func (ctx *CueEditUI) undoTimeline() {
	if ctx.timeline.model.undo() {
		ctx.resetTimecodeInputs()
		ctx.syncTimelineMarkers()
	}
}

func (ctx *CueEditUI) redoTimeline() {
	if ctx.timeline.model.redo() {
		ctx.resetTimecodeInputs()
		ctx.syncTimelineMarkers()
	}
}

func (ctx *CueEditUI) resetTimecodeInputs() {
	markers := ctx.timelineMarkers()
	if markers == nil {
		ctx.page.markers = nil
		return
	}
	ctx.page.markers = make([]timecodeMarkerInputs, len(*markers))
	for index, marker := range *markers {
		ctx.page.markers[index] = newTimecodeMarkerInputs(marker)
	}
}
