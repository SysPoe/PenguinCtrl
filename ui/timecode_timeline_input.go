package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/ui/input"
)

// TODO(micro): Remove these unused pass-through constructors and the input import they keep alive.
// TODO(micro): unused wrappers — delete or call input.NewInteger/NewCheckbox directly at use sites
func inputInteger(label string, value int) *input.Integer    { return input.NewInteger(label, value) }
// TODO(micro): unused wrapper — delete or call input.NewCheckbox directly at use sites
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
	// TODO(micro): ignored json.Marshal error; check err before writing clipboard.
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
	base := min(max(int64(0), t.hoverMs), max(int64(0), ctx.timecodeCueDuration()-maximumOffset))
	// TODO(micro): selection map cleared here and again after append; drop this first clear.
	t.selected = map[int]bool{}
	for _, marker := range pasted {
		marker.TimeMs = base + marker.TimeMs - minimum
		*markers = append(*markers, marker)
	}
	sortTimecodeMarkers(markers)
	t.selected = map[int]bool{}
	ctx.resetTimecodeInputs()
	ctx.updateTimelineDuration()
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
		ctx.updateTimelineDuration()
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
		hoverTrackMs := t.xToMs(e.Position.X, size.X)
		t.hoverMs = ctx.timelineTrackToCueMs(hoverTrackMs)
		switch e.Kind {
		case pointer.Scroll:
			if e.Modifiers.Contain(key.ModShortcut) {
				anchor := hoverTrackMs
				oldDur := t.viewDuration()
				frac := float64(anchor-t.viewStartMs) / float64(oldDur)
				// TODO(micro): zoom max 64, factor 1.12, scroll divisor 50 are magic; name timeline zoom consts.
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
			clipStart, clipEnd, hasClip := ctx.timecodeClipRange()
			clipDuration := max(int64(0), clipEnd-clipStart)
			fadeIn, fadeOut := ctx.timecodeFades()
			if hasClip && clipDuration > 0 && e.Position.Y <= 34 && math.Abs(float64(float32(t.msToX(clipStart, size.X))-e.Position.X)) <= 10 {
				t.dragClipStartMs, t.dragClipEndMs, t.dragViewMs = clipStart, clipEnd, t.viewDuration()
				t.dragMode = timelineDragClipStart
			} else if hasClip && clipDuration > 0 && e.Position.Y <= 34 && math.Abs(float64(float32(t.msToX(clipEnd, size.X))-e.Position.X)) <= 10 {
				t.dragClipStartMs, t.dragClipEndMs, t.dragViewMs = clipStart, clipEnd, t.viewDuration()
				t.dragMode = timelineDragClipEnd
			} else if e.Position.Y >= float32(size.Y-34) && math.Abs(float64(float32(t.msToX(ctx.timelineCueToTrackMs(fadeIn), size.X))-e.Position.X)) <= 9 {
				t.dragMode = timelineDragFadeIn
			} else if e.Position.Y >= float32(size.Y-34) && math.Abs(float64(float32(t.msToX(ctx.timelineCueToTrackMs(max(int64(0), ctx.timecodeCueDuration()-fadeOut)), size.X))-e.Position.X)) <= 9 {
				t.dragMode = timelineDragFadeOut
			} else if durationIndex := ctx.timelineActionDurationAt(e.Position.X, e.Position.Y, size, *markers); durationIndex >= 0 {
				t.checkpoint(*markers)
				t.dragIndex = durationIndex
				t.dragMode = timelineDragActionDuration
			} else if index >= 0 {
				// TODO(micro): Flatten this selection toggle into a switch to make modifier, existing-selection, and replacement cases explicit.
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
				t.dragStartMs = t.hoverMs
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
			case timelineDragClipStart:
				delta := int64(float64(e.Position.X-t.dragStartX) / float64(max(1, size.X)) * float64(t.dragViewMs))
				ctx.setTimecodeClipStart(t.dragClipStartMs + delta)
			case timelineDragClipEnd:
				delta := int64(float64(e.Position.X-t.dragStartX) / float64(max(1, size.X)) * float64(t.dragViewMs))
				ctx.setTimecodeClipEnd(t.dragClipEndMs + delta)
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
				delta = max(-minTime, min(delta, ctx.timecodeCueDuration()-maxTime))
				for i, index := range t.dragIndexes {
					(*markers)[index].TimeMs = t.dragMarkerTimes[i] + delta
				}
			case timelineDragFadeIn:
				_, fadeOut := ctx.timecodeFades()
				ctx.setTimecodeFades(min(ctx.timecodeCueDuration(), t.hoverMs), fadeOut)
			case timelineDragFadeOut:
				fadeIn, _ := ctx.timecodeFades()
				ctx.setTimecodeFades(fadeIn, min(ctx.timecodeCueDuration(), max(int64(0), ctx.timecodeCueDuration()-t.hoverMs)))
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
		x := float32(t.msToX(ctx.timelineCueToTrackMs(markers[i].TimeMs+*duration), size.X))
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
		mx := float32(t.msToX(ctx.timelineCueToTrackMs(m.TimeMs), width))
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
