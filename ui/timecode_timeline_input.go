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
)

const (
	timelineMinZoom       = 1.0
	timelineMaxZoom       = 64.0
	timelineZoomFactor    = 1.12
	timelineScrollDivisor = 50.0
)

func (editor *timecodeEditor) copySelection(gtx layout.Context) {
	copied := editor.model.copySelection()
	if len(copied) == 0 {
		return
	}
	payload, err := json.Marshal(timecodeClipboard{Format: "cusus-timecode-markers", Markers: copied})
	if err != nil {
		return
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(string(payload)))})
}

func (editor *timecodeEditor) pasteMarkers(pasted []show.TimecodeMarker) {
	if len(pasted) == 0 {
		return
	}
	editor.model.paste(pasted, editor.hoverMs, editor.cueDuration())
	editor.resetInputs()
	editor.syncMarkers()
	editor.updateDuration()
}

func (editor *timecodeEditor) handleKeys(gtx layout.Context) {
	t := editor
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
				t.model.selectAll()
			case "C":
				editor.copySelection(gtx)
			case "V":
				gtx.Execute(clipboard.ReadCmd{Tag: &t.tag})
			case "Z":
				if e.Modifiers.Contain(key.ModShift) {
					editor.redoEdit()
				} else {
					editor.undoEdit()
				}
			case "Y":
				editor.redoEdit()
			case key.NameDeleteBackward, key.NameDeleteForward:
				editor.deleteSelectedMarkers()
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
				editor.pasteMarkers(payload.Markers)
			}
		}
	}
}

func (editor *timecodeEditor) deleteSelectedMarkers() {
	if editor.model.deleteSelected() {
		editor.resetInputs()
		editor.syncMarkers()
	}
}

func (editor *timecodeEditor) toolbar(th *material.Theme, gtx layout.Context) layout.Dimensions {
	t := editor
	if t.add.Clicked(gtx) {
		t.model.add(t.playheadMs)
		editor.resetInputs()
		editor.syncMarkers()
		editor.updateDuration()
	}
	if t.preview.Clicked(gtx) {
		editor.togglePreview()
	}
	if t.undo.Clicked(gtx) {
		editor.undoEdit()
	}
	if t.redo.Clicked(gtx) {
		editor.redoEdit()
	}
	button := func(click *widget.Clickable, label string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, material.Button(th, click, label).Layout)
		})
	}
	status := fmt.Sprintf("%s  |  %d action(s), %d selected", formatTimelineMs(t.playheadMs), len(t.model.markers), len(t.model.selectedIndexes()))
	previewLabel := "Play preview"
	if t.previewing {
		previewLabel = "Pause preview"
	}
	children := []layout.FlexChild{button(&t.add, "+ Action")}
	if cue := editor.cue(); cue != nil && cue.Play.Sound != nil {
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

func (editor *timecodeEditor) handlePointer(gtx layout.Context, size image.Point) {
	t := editor
	markers := &t.model.markers
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &t.tag, Kinds: pointer.Press | pointer.Drag | pointer.Move | pointer.Release | pointer.Cancel | pointer.Scroll, ScrollX: pointer.ScrollRange{Min: -10000, Max: 10000}, ScrollY: pointer.ScrollRange{Min: -10000, Max: 10000}})
		if !ok {
			break
		}
		e := ev.(pointer.Event)
		hoverTrackMs := t.xToMs(e.Position.X, size.X)
		t.hoverMs = editor.trackToCueMs(hoverTrackMs)
		switch e.Kind {
		case pointer.Scroll:
			if e.Modifiers.Contain(key.ModShortcut) {
				anchor := hoverTrackMs
				oldDur := t.viewDuration()
				frac := float64(anchor-t.viewStartMs) / float64(oldDur)
				t.zoom = math.Max(timelineMinZoom, math.Min(timelineMaxZoom, t.zoom*math.Pow(timelineZoomFactor, float64(-e.Scroll.Y)/timelineScrollDivisor)))
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
			index := editor.markerAt(e.Position.X, size.X, *markers)
			if e.Buttons.Contain(pointer.ButtonTertiary) {
				t.dragMode = timelineDragPan
				break
			}
			clipStart, clipEnd, hasClip := editor.clipRange()
			clipDuration := max(int64(0), clipEnd-clipStart)
			fadeIn, fadeOut := editor.fades()
			if hasClip && clipDuration > 0 && e.Position.Y <= 34 && math.Abs(float64(float32(t.msToX(clipStart, size.X))-e.Position.X)) <= 10 {
				t.dragClipStartMs, t.dragClipEndMs, t.dragViewMs = clipStart, clipEnd, t.viewDuration()
				t.dragMode = timelineDragClipStart
			} else if hasClip && clipDuration > 0 && e.Position.Y <= 34 && math.Abs(float64(float32(t.msToX(clipEnd, size.X))-e.Position.X)) <= 10 {
				t.dragClipStartMs, t.dragClipEndMs, t.dragViewMs = clipStart, clipEnd, t.viewDuration()
				t.dragMode = timelineDragClipEnd
			} else if e.Position.Y >= float32(size.Y-34) && math.Abs(float64(float32(t.msToX(editor.cueToTrackMs(fadeIn), size.X))-e.Position.X)) <= 9 {
				t.dragMode = timelineDragFadeIn
			} else if e.Position.Y >= float32(size.Y-34) && math.Abs(float64(float32(t.msToX(editor.cueToTrackMs(max(int64(0), editor.cueDuration()-fadeOut)), size.X))-e.Position.X)) <= 9 {
				t.dragMode = timelineDragFadeOut
			} else if durationIndex := editor.actionDurationAt(e.Position.X, e.Position.Y, size, *markers); durationIndex >= 0 {
				t.model.checkpoint()
				t.dragIndex = durationIndex
				t.dragMode = timelineDragActionDuration
			} else if index >= 0 {
				switch {
				case e.Modifiers.Contain(key.ModShift) || e.Modifiers.Contain(key.ModShortcut):
					t.model.toggleSelection(index)
				case !t.model.selected[index]:
					t.model.selectOnly(index)
				}
				t.dragIndexes = t.model.selectedIndexes()
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
				t.model.selectRange(t.dragStartMs, t.dragCurrentMs)
			} else {
				t.model.clearSelection()
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
				t.model.selectRange(t.dragStartMs, t.dragCurrentMs)
			case timelineDragPan:
				delta := e.Position.X - t.dragLastX
				t.viewStartMs -= int64(float64(delta) / float64(max(1, size.X)) * float64(t.viewDuration()))
				t.clampView()
				t.dragLastX = e.Position.X
			case timelineDragClipStart:
				delta := int64(float64(e.Position.X-t.dragStartX) / float64(max(1, size.X)) * float64(t.dragViewMs))
				editor.setClipStart(t.dragClipStartMs + delta)
			case timelineDragClipEnd:
				delta := int64(float64(e.Position.X-t.dragStartX) / float64(max(1, size.X)) * float64(t.dragViewMs))
				editor.setClipEnd(t.dragClipEndMs + delta)
			case timelineDragMarkers:
				if !t.dragChanged {
					t.model.checkpoint()
					t.dragChanged = true
				}
				delta := t.hoverMs - t.dragStartMs
				minTime, maxTime := t.dragMarkerTimes[0], t.dragMarkerTimes[0]
				for _, tm := range t.dragMarkerTimes {
					minTime = min(minTime, tm)
					maxTime = max(maxTime, tm)
				}
				delta = max(-minTime, min(delta, editor.cueDuration()-maxTime))
				for i, index := range t.dragIndexes {
					t.model.setMarkerTime(index, t.dragMarkerTimes[i]+delta)
				}
			case timelineDragFadeIn:
				_, fadeOut := editor.fades()
				editor.setFades(min(editor.cueDuration(), t.hoverMs), fadeOut)
			case timelineDragFadeOut:
				fadeIn, _ := editor.fades()
				editor.setFades(fadeIn, min(editor.cueDuration(), max(int64(0), editor.cueDuration()-t.hoverMs)))
			case timelineDragActionDuration:
				if t.dragIndex >= 0 && t.dragIndex < len(t.model.markers) {
					t.model.setActionDuration(t.dragIndex, t.hoverMs-t.model.markers[t.dragIndex].TimeMs)
				}
			}
		case pointer.Release, pointer.Cancel:
			if t.dragMode == timelineDragMarkers && t.dragChanged {
				t.model.normalize()
				t.model.clearSelection()
			}
			if (t.dragMode == timelineDragMarkers && t.dragChanged) || t.dragMode == timelineDragActionDuration {
				editor.resetInputs()
			}
			t.dragMode = timelineDragNone
			editor.syncMarkers()
		}
	}
}

func (editor *timecodeEditor) actionDurationAt(px, py float32, size image.Point, markers []show.TimecodeMarker) int {
	t := editor
	for i := range markers {
		duration := markerActionDuration(&markers[i])
		if duration == nil || *duration <= 0 {
			continue
		}
		x := float32(t.msToX(editor.cueToTrackMs(markers[i].TimeMs+*duration), size.X))
		y := float32(38 + (i%4)*20)
		if math.Abs(float64(x-px)) <= 9 && math.Abs(float64(y-py)) <= 10 {
			return i
		}
	}
	return -1
}

func (editor *timecodeEditor) markerAt(x float32, width int, markers []show.TimecodeMarker) int {
	t := editor
	best, distance := -1, float32(9)
	for i, m := range markers {
		mx := float32(t.msToX(editor.cueToTrackMs(m.TimeMs), width))
		d := float32(math.Abs(float64(mx - x)))
		if d <= distance {
			best, distance = i, d
		}
	}
	return best
}
