package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"sort"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
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
func inputInteger(label string, value int) *input.Integer    { return input.NewInteger(label, value) }
func inputCheckbox(label string, value bool) *input.Checkbox { return input.NewCheckbox(label, value) }

func (ctx *CueEditUI) copyTimelineSelection(gtx layout.Context, markers []show.TimecodeMarker) {
	t := &ctx.timeline
	indexes := selectedTimelineIndexes(t, len(markers))
	if len(indexes) == 0 {
		return
	}
	t.clipboard = make([]show.TimecodeMarker, len(indexes))
	for i, index := range indexes {
		t.clipboard[i] = markers[index]
	}
	payload, _ := json.Marshal(timecodeClipboard{Format: "cusus-timecode-markers", Markers: t.clipboard})
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(string(payload)))})
}

func (ctx *CueEditUI) pasteTimelineMarkers(markers *[]show.TimecodeMarker, pasted []show.TimecodeMarker) {
	if len(pasted) == 0 {
		return
	}
	t := &ctx.timeline
	t.checkpoint(*markers)
	minimum := pasted[0].TimeMs
	maximumOffset := int64(0)
	for _, marker := range pasted {
		minimum = min(minimum, marker.TimeMs)
	}
	for _, marker := range pasted {
		maximumOffset = max(maximumOffset, marker.TimeMs-minimum)
	}
	base := min(max(int64(0), t.hoverMs), max(int64(0), t.durationMs-maximumOffset))
	t.selected = map[int]bool{}
	for _, marker := range pasted {
		marker.TimeMs = base + marker.TimeMs - minimum
		*markers = append(*markers, marker)
	}
	sortTimecodeMarkers(markers)
	t.selected = map[int]bool{}
	ctx.resetTimecodeInputs()
	ctx.updateTimelineDuration(0, 0)
}

func (ctx *CueEditUI) handleTimelineKeys(gtx layout.Context, markers *[]show.TimecodeMarker) {
	t := &ctx.timeline
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &t.tag, Name: "A", Required: key.ModShortcut},
			key.Filter{Focus: &t.tag, Name: "C", Required: key.ModShortcut},
			key.Filter{Focus: &t.tag, Name: "V", Required: key.ModShortcut},
			key.Filter{Focus: &t.tag, Name: "Z", Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Focus: &t.tag, Name: "Y", Required: key.ModShortcut},
			key.Filter{Focus: &t.tag, Name: key.NameDeleteBackward},
			key.Filter{Focus: &t.tag, Name: key.NameDeleteForward},
			transfer.TargetFilter{Target: &t.tag, Type: "application/text"},
		)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case key.Event:
			if e.State != key.Press {
				continue
			}
			switch e.Name {
			case "A":
				t.selected = map[int]bool{}
				for i := range *markers {
					t.selected[i] = true
				}
			case "C":
				ctx.copyTimelineSelection(gtx, *markers)
			case "V":
				gtx.Execute(clipboard.ReadCmd{Tag: &t.tag})
			case "Z":
				if e.Modifiers.Contain(key.ModShift) {
					ctx.redoTimeline(markers)
				} else {
					ctx.undoTimeline(markers)
				}
			case "Y":
				ctx.redoTimeline(markers)
			case key.NameDeleteBackward, key.NameDeleteForward:
				ctx.deleteSelectedTimelineMarkers(markers)
			}
		case transfer.DataEvent:
			reader := e.Open()
			raw, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				continue
			}
			var payload timecodeClipboard
			if json.Unmarshal(raw, &payload) == nil && payload.Format == "cusus-timecode-markers" {
				ctx.pasteTimelineMarkers(markers, payload.Markers)
			}
		}
	}
}

func (ctx *CueEditUI) deleteSelectedTimelineMarkers(markers *[]show.TimecodeMarker) {
	t := &ctx.timeline
	selected := selectedTimelineIndexes(t, len(*markers))
	if len(selected) == 0 {
		return
	}
	t.checkpoint(*markers)
	for i := len(selected) - 1; i >= 0; i-- {
		index := selected[i]
		*markers = append((*markers)[:index], (*markers)[index+1:]...)
	}
	t.selected = map[int]bool{}
	ctx.resetTimecodeInputs()
}

func (ctx *CueEditUI) timelineToolbar(th *material.Theme, gtx layout.Context, markers *[]show.TimecodeMarker) layout.Dimensions {
	t := &ctx.timeline
	if t.add.Clicked(gtx) {
		t.checkpoint(*markers)
		*markers = append(*markers, newTimecodeMarker(t.playheadMs))
		sortTimecodeMarkers(markers)
		t.selected = map[int]bool{}
		for i := range *markers {
			if (*markers)[i].TimeMs == t.playheadMs {
				t.selected[i] = true
				break
			}
		}
		ctx.resetTimecodeInputs()
		ctx.updateTimelineDuration(0, 0)
	}
	if t.preview.Clicked(gtx) {
		ctx.toggleTimecodePreview()
	}
	if t.undo.Clicked(gtx) {
		ctx.undoTimeline(markers)
	}
	if t.redo.Clicked(gtx) {
		ctx.redoTimeline(markers)
	}
	button := func(click *widget.Clickable, label string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, material.Button(th, click, label).Layout)
		})
	}
	status := fmt.Sprintf("%s  |  %d action(s), %d selected", formatTimelineMs(t.playheadMs), len(*markers), len(selectedTimelineIndexes(t, len(*markers))))
	previewLabel := "Play preview"
	if t.previewing {
		previewLabel = "Pause preview"
	}
	children := []layout.FlexChild{button(&t.add, "+ Action")}
	if ctx.cue.Play.Sound != nil {
		children = append(children, button(&t.preview, previewLabel))
	}
	children = append(children, button(&t.undo, "Undo"), button(&t.redo, "Redo"), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		label := material.Body2(th, status)
		return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, label.Layout)
	}))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func formatTimelineMs(ms int64) string {
	minutes := ms / 60000
	seconds := (ms % 60000) / 1000
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d.%03d", minutes, seconds, millis)
}

func (ctx *CueEditUI) handleTimelinePointer(gtx layout.Context, size image.Point, markers *[]show.TimecodeMarker) {
	t := &ctx.timeline
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &t.tag, Kinds: pointer.Press | pointer.Drag | pointer.Move | pointer.Release | pointer.Cancel | pointer.Scroll, ScrollX: pointer.ScrollRange{Min: -10000, Max: 10000}, ScrollY: pointer.ScrollRange{Min: -10000, Max: 10000}})
		if !ok {
			break
		}
		e := ev.(pointer.Event)
		t.hoverMs = t.xToMs(e.Position.X, size.X)
		switch e.Kind {
		case pointer.Scroll:
			if e.Modifiers.Contain(key.ModShortcut) {
				anchor := t.hoverMs
				oldDur := t.viewDuration()
				frac := float64(anchor-t.viewStartMs) / float64(oldDur)
				t.zoom = math.Max(1, math.Min(64, t.zoom*math.Pow(1.12, float64(-e.Scroll.Y)/50)))
				t.viewStartMs = anchor - int64(frac*float64(t.viewDuration()))
				t.clampView()
			} else {
				t.viewStartMs += int64(float64(t.viewDuration()) * float64(e.Scroll.Y+e.Scroll.X) / float64(max(1, size.X)))
				t.clampView()
			}
		case pointer.Press:
			gtx.Execute(key.FocusCmd{Tag: &t.tag})
			gtx.Execute(pointer.GrabCmd{Tag: &t.tag, ID: e.PointerID})
			t.dragPointer, t.dragStartX, t.dragLastX = e.PointerID, e.Position.X, e.Position.X
			index := ctx.timelineMarkerAt(e.Position.X, size.X, *markers)
			if e.Buttons.Contain(pointer.ButtonTertiary) {
				t.dragMode = timelineDragPan
				break
			}
			fadeIn, fadeOut := ctx.timecodeFades()
			if e.Position.Y >= float32(size.Y-34) && math.Abs(float64(float32(t.msToX(fadeIn, size.X))-e.Position.X)) <= 9 {
				t.dragMode = timelineDragFadeIn
			} else if e.Position.Y >= float32(size.Y-34) && math.Abs(float64(float32(t.msToX(max(int64(0), t.durationMs-fadeOut), size.X))-e.Position.X)) <= 9 {
				t.dragMode = timelineDragFadeOut
			} else if durationIndex := ctx.timelineActionDurationAt(e.Position.X, e.Position.Y, size, *markers); durationIndex >= 0 {
				t.checkpoint(*markers)
				t.dragIndex = durationIndex
				t.dragMode = timelineDragActionDuration
			} else if index >= 0 {
				if e.Modifiers.Contain(key.ModShift) || e.Modifiers.Contain(key.ModShortcut) {
					if t.selected[index] {
						delete(t.selected, index)
					} else {
						t.selected[index] = true
					}
				} else if !t.selected[index] {
					t.selected = map[int]bool{index: true}
				}
				t.dragIndexes = selectedTimelineIndexes(t, len(*markers))
				if len(t.dragIndexes) == 0 {
					t.dragMode = timelineDragNone
					break
				}
				t.dragMarkerTimes = make([]int64, len(t.dragIndexes))
				for i, markerIndex := range t.dragIndexes {
					t.dragMarkerTimes[i] = (*markers)[markerIndex].TimeMs
				}
				t.dragStartMs = t.xToMs(e.Position.X, size.X)
				t.dragChanged = false
				t.dragMode = timelineDragMarkers
			} else if e.Modifiers.Contain(key.ModShift) {
				t.dragMode = timelineDragRange
				t.dragStartMs = t.hoverMs
				t.dragCurrentMs = t.hoverMs
				ctx.selectTimelineRange(*markers, t.dragStartMs, t.dragCurrentMs)
			} else {
				t.selected = map[int]bool{}
				t.dragMode = timelineDragPlayhead
				t.playheadMs = t.hoverMs
			}
		case pointer.Drag:
			if e.PointerID != t.dragPointer {
				continue
			}
			switch t.dragMode {
			case timelineDragPlayhead:
				t.playheadMs = t.hoverMs
			case timelineDragRange:
				t.dragCurrentMs = t.hoverMs
				ctx.selectTimelineRange(*markers, t.dragStartMs, t.dragCurrentMs)
			case timelineDragPan:
				delta := e.Position.X - t.dragLastX
				t.viewStartMs -= int64(float64(delta) / float64(max(1, size.X)) * float64(t.viewDuration()))
				t.clampView()
				t.dragLastX = e.Position.X
			case timelineDragMarkers:
				if !t.dragChanged {
					t.checkpoint(*markers)
					t.dragChanged = true
				}
				delta := t.hoverMs - t.dragStartMs
				minTime, maxTime := t.dragMarkerTimes[0], t.dragMarkerTimes[0]
				for _, tm := range t.dragMarkerTimes {
					minTime = min(minTime, tm)
					maxTime = max(maxTime, tm)
				}
				delta = max(-minTime, min(delta, t.durationMs-maxTime))
				for i, index := range t.dragIndexes {
					(*markers)[index].TimeMs = t.dragMarkerTimes[i] + delta
				}
			case timelineDragFadeIn:
				_, fadeOut := ctx.timecodeFades()
				ctx.setTimecodeFades(min(t.durationMs, t.hoverMs), fadeOut)
			case timelineDragFadeOut:
				fadeIn, _ := ctx.timecodeFades()
				ctx.setTimecodeFades(fadeIn, min(t.durationMs, max(int64(0), t.durationMs-t.hoverMs)))
			case timelineDragActionDuration:
				if t.dragIndex >= 0 && t.dragIndex < len(*markers) {
					if duration := markerActionDuration(&(*markers)[t.dragIndex]); duration != nil {
						*duration = max(int64(0), t.hoverMs-(*markers)[t.dragIndex].TimeMs)
					}
				}
			}
		case pointer.Release, pointer.Cancel:
			if t.dragMode == timelineDragMarkers && t.dragChanged {
				sortTimecodeMarkers(markers)
				t.selected = map[int]bool{}
			}
			if (t.dragMode == timelineDragMarkers && t.dragChanged) || t.dragMode == timelineDragActionDuration {
				ctx.resetTimecodeInputs()
			}
			t.dragMode = timelineDragNone
		}
	}
}

func (ctx *CueEditUI) timelineActionDurationAt(px, py float32, size image.Point, markers []show.TimecodeMarker) int {
	t := &ctx.timeline
	for i := range markers {
		duration := markerActionDuration(&markers[i])
		if duration == nil || *duration <= 0 {
			continue
		}
		x := float32(t.msToX(markers[i].TimeMs+*duration, size.X))
		y := float32(38 + (i%4)*20)
		if math.Abs(float64(x-px)) <= 9 && math.Abs(float64(y-py)) <= 10 {
			return i
		}
	}
	return -1
}

func (ctx *CueEditUI) timelineMarkerAt(x float32, width int, markers []show.TimecodeMarker) int {
	t := &ctx.timeline
	best, distance := -1, float32(9)
	for i, m := range markers {
		mx := float32(t.msToX(m.TimeMs, width))
		d := float32(math.Abs(float64(mx - x)))
		if d <= distance {
			best, distance = i, d
		}
	}
	return best
}

func (ctx *CueEditUI) selectTimelineRange(markers []show.TimecodeMarker, a, b int64) {
	lo, hi := min(a, b), max(a, b)
	ctx.timeline.selected = map[int]bool{}
	for i, m := range markers {
		if m.TimeMs >= lo && m.TimeMs <= hi {
			ctx.timeline.selected[i] = true
		}
	}
}

func (ctx *CueEditUI) drawTimeline(th *material.Theme, gtx layout.Context, markers *[]show.TimecodeMarker, _ *show.ShowManager) layout.Dimensions {
	t := &ctx.timeline
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(190)))
	if size.X < 1 {
		size.X = 1
	}
	ctx.handleTimelineKeys(gtx, markers)
	ctx.handleTimelinePointer(gtx, size, markers)
	paint.FillShape(gtx.Ops, palette.SurfaceSunken, clip.Rect{Max: size}.Op())
	// Time grid.
	gridStep := int64(1000)
	for float64(gridStep)*float64(size.X)/float64(t.viewDuration()) < 70 {
		gridStep *= 2
	}
	first := (t.viewStartMs / gridStep) * gridStep
	for ms := first; ms <= t.viewStartMs+t.viewDuration(); ms += gridStep {
		x := t.msToX(ms, size.X)
		paint.FillShape(gtx.Ops, palette.WithAlpha(palette.Divider, 0x70), clip.Rect{Min: image.Pt(x, 0), Max: image.Pt(x+1, size.Y)}.Op())
	}
	ctx.drawWaveformBars(gtx, size)
	ctx.drawFadeZones(gtx, size)
	ctx.drawActionDurationBars(gtx, size, *markers)
	// Range selection overlay.
	if t.dragMode == timelineDragRange {
		x1, x2 := t.msToX(min(t.dragStartMs, t.dragCurrentMs), size.X), t.msToX(max(t.dragStartMs, t.dragCurrentMs), size.X)
		paint.FillShape(gtx.Ops, palette.WithAlpha(palette.Primary, 0x35), clip.Rect{Min: image.Pt(x1, 0), Max: image.Pt(x2, size.Y)}.Op())
	}
	// Markers and labels.
	for i, m := range *markers {
		x := t.msToX(m.TimeMs, size.X)
		if x < 0 || x > size.X {
			continue
		}
		c := palette.Warning
		if t.selected[i] {
			c = palette.Primary
		}
		if m.Disabled {
			c = palette.Disabled
		}
		paint.FillShape(gtx.Ops, c, clip.Rect{Min: image.Pt(x-2, 0), Max: image.Pt(x+3, size.Y)}.Op())
		ctx.drawMarkerLabel(th, gtx, m, i, x, size)
	}
	// Playhead.
	px := t.msToX(t.playheadMs, size.X)
	paint.FillShape(gtx.Ops, palette.Danger, clip.Rect{Min: image.Pt(px-1, 0), Max: image.Pt(px+2, size.Y)}.Op())
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &t.tag)
	pointer.CursorCrosshair.Add(gtx.Ops)
	area.Pop()
	return layout.Dimensions{Size: size}
}

func (ctx *CueEditUI) timecodeEditorRows(th *material.Theme, markers *[]show.TimecodeMarker) []cueEditFormRow {
	rows := []cueEditFormRow{timecodeSectionRow(th, "Clip and fades")}
	if play := ctx.cue.Play.Sound; play != nil {
		rows = append(rows,
			integerRow(th, "Clip start", ctx.page.integer["soundClipStartMs"], func(v int) { play.ClipStartMs = int64(max(0, v)) }),
			integerRow(th, "Clip end", ctx.page.integer["soundClipEndMs"], func(v int) { play.ClipEndMs = int64(max(0, v)) }),
			integerRow(th, "Fade in", ctx.page.integer["soundFadeInMs"], func(v int) { play.FadeInMs = int64(max(0, v)) }),
			integerRow(th, "Fade out", ctx.page.integer["soundFadeOutMs"], func(v int) { play.FadeOutMs = int64(max(0, v)) }),
		)
	} else if play := ctx.cue.Play.Video; play != nil {
		rows = append(rows,
			integerRow(th, "Clip start", ctx.page.integer["videoClipStartMs"], func(v int) { play.ClipStartMs = int64(max(0, v)) }),
			integerRow(th, "Clip end", ctx.page.integer["videoClipEndMs"], func(v int) { play.ClipEndMs = int64(max(0, v)) }),
			integerRow(th, "Fade in", ctx.page.integer["videoFadeInMs"], func(v int) { play.FadeInMs = int64(max(0, v)) }),
			integerRow(th, "Fade out", ctx.page.integer["videoFadeOutMs"], func(v int) { play.FadeOutMs = int64(max(0, v)) }),
		)
	} else if play := ctx.cue.Play.Image; play != nil {
		rows = append(rows,
			integerRow(th, "Duration", ctx.page.integer["imageDurationMs"], func(v int) { play.DurationMs = int64(max(0, v)) }),
			integerRow(th, "Fade in", ctx.page.integer["imageFadeInMs"], func(v int) { play.FadeInMs = int64(max(0, v)) }),
			integerRow(th, "Fade out", ctx.page.integer["imageFadeOutMs"], func(v int) { play.FadeOutMs = int64(max(0, v)) }),
		)
	}

	selected := selectedTimelineIndexes(&ctx.timeline, len(*markers))
	rows = append(rows, timecodeSectionRow(th, "Selected action"))
	if len(selected) != 1 {
		rows = append(rows, staticRow(th, "Action", "Select one timeline action to edit it"))
		return rows
	}
	index := selected[0]
	marker := &(*markers)[index]
	key := fmt.Sprintf("timecode.%d", index)
	rows = append(rows,
		integerRow(th, "At", ctx.page.integer[key+".time"], func(v int) { marker.TimeMs = int64(max(0, v)) }),
		checkboxRow(th, "", ctx.page.checkbox[key+".disabled"], func(v bool) { marker.Disabled = v }),
		dropdownRow(th, "Action", ctx.page.dropdown[key+".type"], func(selected int) {
			if selected != timecodeActionIndex(marker.Type) {
				ctx.timeline.checkpoint(*markers)
				setTimecodeActionType(marker, selected)
				ctx.resetTimecodeInputs()
			}
		}),
	)

	switch marker.Type {
	case show.CueTypeMediaControl:
		if marker.Action.MediaControl == nil {
			marker.Action.MediaControl = defaultTimecodeMediaControl()
		}
		play := marker.Action.MediaControl
		play.Target = show.MediaTarget{Kind: show.MediaTargetCurrentTrack}
		rows = append(rows, dropdownRow(th, "Track action", ctx.page.dropdown[key+".mediaAction"], func(v int) {
			play.Action = show.MediaControlAction(v)
			if mediaControlActionUsesLevel(play.Action) && play.LevelDB == nil {
				level := 0.0
				play.LevelDB = &level
			}
			if play.Action == show.MediaControlSeek && play.SeekToMs == nil {
				seek := int64(0)
				play.SeekToMs = &seek
			}
		}))
		if mediaControlActionUsesLevel(play.Action) {
			rows = append(rows, floatRow(th, "Level dB", ctx.page.float[key+".level"], func(v float64) { play.LevelDB = &v }))
		}
		if play.Action == show.MediaControlSeek {
			rows = append(rows, integerRow(th, "Seek to", ctx.page.integer[key+".seek"], func(v int) { value := int64(max(0, v)); play.SeekToMs = &value }))
		}
		rows = append(rows,
			integerRow(th, "Fade time", ctx.page.integer[key+".fade"], func(v int) { play.FadeMs = int64(max(0, v)) }),
			dropdownRow(th, "Curve", ctx.page.dropdown[key+".curve"], func(v int) { play.Curve = show.FadeCurve(v) }),
		)
	case show.CueTypeOutputControl:
		if marker.Action.OutputControl == nil {
			marker.Action = show.NewOutputControlCue().Play
		}
		play := marker.Action.OutputControl
		rows = append(rows,
			dropdownRow(th, "Output action", ctx.page.dropdown[key+".outputAction"], func(v int) { play.Action = show.OutputControlAction(v) }),
			textRow(th, "Output", ctx.page.text[key+".outputID"], func(v string) { play.OutputID = v }),
			integerRow(th, "Fade out", ctx.page.integer[key+".fadeOut"], func(v int) { play.FadeOutMs = int64(max(0, v)) }),
			integerRow(th, "Fade in", ctx.page.integer[key+".fadeIn"], func(v int) { play.FadeInMs = int64(max(0, v)) }),
			textRow(th, "Message", ctx.page.text[key+".message"], func(v string) { play.Message = v }),
		)
	case show.CueTypeRemote:
		if marker.Action.Remote == nil {
			marker.Action = show.NewRemoteCue().Play
		}
		play := marker.Action.Remote
		rows = append(rows,
			dropdownRow(th, "Protocol", ctx.page.dropdown[key+".protocol"], func(v int) { play.Protocol = show.RemoteProtocol(v) }),
			dropdownRow(th, "Remote action", ctx.page.dropdown[key+".remoteAction"], func(v int) { play.Action = show.RemoteAction(v) }),
			textRow(th, "Playback", ctx.page.text[key+".playback"], func(v string) { play.Playback = v }),
			textRow(th, "Cue number", ctx.page.text[key+".cueNumber"], func(v string) { play.CueNumber = v }),
			textRow(th, "Level", ctx.page.text[key+".remoteLevel"], func(v string) { play.Level = v }),
		)
		if play.Action == show.RemoteActionCustom {
			rows = append(rows, textRow(th, "Command", ctx.page.text[key+".custom"], func(v string) { play.Custom = v }))
		}
	}
	rows = append(rows, cueEditFormRow{layout: func(gtx layout.Context) layout.Dimensions {
		if ctx.page.button["timecodeDelete"].Clicked(gtx) {
			ctx.timeline.checkpoint(*markers)
			*markers = append((*markers)[:index], (*markers)[index+1:]...)
			ctx.timeline.selected = map[int]bool{}
			ctx.resetTimecodeInputs()
		}
		return layoutCenteredButton(th, gtx, ctx.page.button["timecodeDelete"], "Delete action", palette.Danger)
	}})
	return rows
}

func timecodeSectionRow(th *material.Theme, title string) cueEditFormRow {
	return cueEditFormRow{layout: func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(34))
		paint.FillShape(gtx.Ops, palette.SurfaceRaised, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
		label := material.Body1(th, title)
		label.Color = palette.Text
		return layout.Inset{Left: unit.Dp(10), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, label.Layout)
	}}
}

func (ctx *CueEditUI) drawWaveformBars(gtx layout.Context, size image.Point) {
	t := &ctx.timeline
	if len(t.waveSamples) == 0 || t.waveSampleRate <= 0 {
		return
	}
	_, clipStart, _, _ := ctx.timecodeMediaDetails()
	fadeIn, fadeOut := ctx.timecodeFades()
	center := size.Y / 2
	amp := float64(size.Y) * 0.40
	for x := 0; x < size.X; x++ {
		fromMs := t.viewStartMs + int64(float64(x)/float64(size.X)*float64(t.viewDuration())) + clipStart
		toMs := t.viewStartMs + int64(float64(x+1)/float64(size.X)*float64(t.viewDuration())) + clipStart
		a := int(fromMs * int64(t.waveSampleRate) / 1000)
		b := int(toMs * int64(t.waveSampleRate) / 1000)
		a = max(0, min(a, len(t.waveSamples)))
		b = max(a+1, min(max(a+1, b), len(t.waveSamples)))
		peak := float32(0)
		for i := a; i < b && i < len(t.waveSamples); i++ {
			peak = max(peak, t.waveSamples[i])
		}
		timelineMs := fromMs - clipStart
		gain := float64(1)
		if fadeIn > 0 {
			gain = min(gain, max(float64(0), float64(timelineMs)/float64(fadeIn)))
		}
		if fadeOut > 0 {
			gain = min(gain, max(float64(0), float64(t.durationMs-timelineMs)/float64(fadeOut)))
		}
		h := max(1, int(float64(peak)*amp*gain))
		paint.FillShape(gtx.Ops, palette.WithAlpha(palette.Success, 0xD0), clip.Rect{Min: image.Pt(x, center-h), Max: image.Pt(x+1, center+h)}.Op())
	}
}

func (ctx *CueEditUI) drawFadeZones(gtx layout.Context, size image.Point) {
	t := &ctx.timeline
	fadeIn, fadeOut := ctx.timecodeFades()
	zone := palette.WithAlpha(palette.Accent, 0x24)
	if fadeIn > 0 {
		x1, x2 := t.msToX(0, size.X), t.msToX(min(fadeIn, t.durationMs), size.X)
		paint.FillShape(gtx.Ops, zone, clip.Rect{Min: image.Pt(max(0, x1), 0), Max: image.Pt(min(size.X, x2), size.Y)}.Op())
	}
	if fadeOut > 0 {
		x1, x2 := t.msToX(max(int64(0), t.durationMs-fadeOut), size.X), t.msToX(t.durationMs, size.X)
		paint.FillShape(gtx.Ops, zone, clip.Rect{Min: image.Pt(max(0, x1), 0), Max: image.Pt(min(size.X, x2), size.Y)}.Op())
	}
	bottom := size.Y - 34
	inX := t.msToX(fadeIn, size.X)
	outX := t.msToX(max(int64(0), t.durationMs-fadeOut), size.X)
	handle := palette.Accent
	paint.FillShape(gtx.Ops, handle, clip.Rect{Min: image.Pt(inX-2, bottom), Max: image.Pt(inX+3, size.Y)}.Op())
	paint.FillShape(gtx.Ops, handle, clip.Rect{Min: image.Pt(outX-2, bottom), Max: image.Pt(outX+3, size.Y)}.Op())
}

func (ctx *CueEditUI) drawActionDurationBars(gtx layout.Context, size image.Point, markers []show.TimecodeMarker) {
	t := &ctx.timeline
	for i := range markers {
		duration := markerActionDuration(&markers[i])
		if duration == nil || *duration <= 0 {
			continue
		}
		x1 := t.msToX(markers[i].TimeMs, size.X)
		x2 := t.msToX(markers[i].TimeMs+*duration, size.X)
		y := 38 + (i%4)*20
		paint.FillShape(gtx.Ops, palette.WithAlpha(palette.Primary, 0xC0), clip.Rect{Min: image.Pt(x1, y-3), Max: image.Pt(x2, y+4)}.Op())
		paint.FillShape(gtx.Ops, palette.Primary, clip.Rect{Min: image.Pt(x2-2, y-8), Max: image.Pt(x2+3, y+9)}.Op())
	}
}

func timecodeActionLabel(m show.TimecodeMarker) string {
	switch m.Type {
	case show.CueTypeMediaControl:
		if m.Action.MediaControl != nil && int(m.Action.MediaControl.Action) >= 0 && int(m.Action.MediaControl.Action) < len(mediaControlActionLabels) {
			return "Track · " + mediaControlActionLabels[m.Action.MediaControl.Action]
		}
		return "Current track"
	case show.CueTypeOutputControl:
		if m.Action.OutputControl != nil && int(m.Action.OutputControl.Action) >= 0 && int(m.Action.OutputControl.Action) < len(outputControlActionLabels) {
			return "Output · " + outputControlActionLabels[m.Action.OutputControl.Action]
		}
		return "Output control"
	case show.CueTypeRemote:
		if m.Action.Remote != nil && int(m.Action.Remote.Action) >= 0 && int(m.Action.Remote.Action) < len(remoteActionLabels) {
			return "Remote · " + remoteActionLabels[m.Action.Remote.Action]
		}
		return "Remote"
	default:
		return "Action"
	}
}

func (ctx *CueEditUI) drawMarkerLabel(th *material.Theme, gtx layout.Context, m show.TimecodeMarker, index, x int, size image.Point) {
	label := timecodeActionLabel(m)
	stack := op.Offset(image.Pt(min(size.X-150, max(2, x+5)), 4+(index%4)*20)).Push(gtx.Ops)
	defer stack.Pop()
	text := material.Caption(th, label)
	text.Color = palette.WithAlpha(palette.Text, 0xEE)
	text.MaxLines = 1
	text.Layout(gtx)
}

func (ctx *CueEditUI) renderNativeTimecodeEditor(th *material.Theme, gtx layout.Context, manager *show.ShowManager, markers *[]show.TimecodeMarker) layout.Dimensions {
	ctx.ensureTimelineWaveform()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ctx.timelineToolbar(th, gtx, markers) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			_, clipStart, clipEnd, _ := ctx.timecodeMediaDetails()
			if clipEnd <= clipStart {
				clipEnd = clipStart + ctx.timeline.durationMs
			}
			fadeIn, fadeOut := ctx.timecodeFades()
			text := fmt.Sprintf("Clip %s → %s   •   Fade in %s   •   Fade out %s", formatTimelineMs(clipStart), formatTimelineMs(clipEnd), formatTimelineMs(fadeIn), formatTimelineMs(fadeOut))
			label := material.Caption(th, text)
			label.Color = palette.TextSoft
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, label.Layout)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return ctx.drawTimeline(th, gtx, markers, manager) })
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			status := "Drag actions or their fade bars; drag the bottom fade handles; wheel pans; Ctrl+wheel zooms."
			if ctx.timeline.loading {
				status = "Loading waveform…"
			} else if ctx.timeline.waveError != "" {
				status = "Timeline ready; waveform unavailable: " + ctx.timeline.waveError
			}
			label := material.Body2(th, status)
			return label.Layout(gtx)
		}),
	)
}
