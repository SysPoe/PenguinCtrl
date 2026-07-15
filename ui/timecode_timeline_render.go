package ui

import (
	"fmt"
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/show"
)

// TODO(micro): unused manager param; drop from signature and call site in renderNativeTimecodeEditor.
func (ctx *CueEditUI) drawTimeline(th *material.Theme, gtx layout.Context, markers *[]show.TimecodeMarker, _ *show.ShowManager) layout.Dimensions {
	t := &ctx.timeline
	// TODO(micro): 190 timeline height is a magic Dp; name timelineHeightDp const.
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
	ctx.drawClipHandles(gtx, size)
	ctx.drawActionDurationBars(gtx, size, *markers)
	// Range selection overlay.
	if t.dragMode == timelineDragRange {
		x1 := t.msToX(ctx.timelineCueToTrackMs(min(t.dragStartMs, t.dragCurrentMs)), size.X)
		x2 := t.msToX(ctx.timelineCueToTrackMs(max(t.dragStartMs, t.dragCurrentMs)), size.X)
		paint.FillShape(gtx.Ops, palette.WithAlpha(palette.Primary, 0x35), clip.Rect{Min: image.Pt(x1, 0), Max: image.Pt(x2, size.Y)}.Op())
	}
	// Markers and labels.
	for i, m := range *markers {
		x := t.msToX(ctx.timelineCueToTrackMs(m.TimeMs), size.X)
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
	px := t.msToX(ctx.timelineCueToTrackMs(t.playheadMs), size.X)
	paint.FillShape(gtx.Ops, palette.Danger, clip.Rect{Min: image.Pt(px-1, 0), Max: image.Pt(px+2, size.Y)}.Op())
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &t.tag)
	pointer.CursorCrosshair.Add(gtx.Ops)
	area.Pop()
	return layout.Dimensions{Size: size}
}

// TODO(macro): Timecode tab re-binds clip/fade/duration fields that the Media tab already owns, plus a parallel MediaControl/Remote/OutputControl form for markers. Share one media-range binder and one action-form builder so Media and Timecode stop dual-editing the same cue fields through separate integer widgets.
func (ctx *CueEditUI) timecodeEditorRows(th *material.Theme, markers *[]show.TimecodeMarker) []cueEditFormRow {
	rows := []cueEditFormRow{timecodeSectionRow(th, "Clip and fades")}
	if play := ctx.cue.Play.Sound; play != nil {
		rows = append(rows,
			integerRow(th, "Clip start", ctx.page.integer["soundClipStartMs"], func(v int) { ctx.setTimecodeClipStart(int64(v)) }),
			integerRow(th, "Clip end", ctx.page.integer["soundClipEndMs"], func(v int) { ctx.setTimecodeClipEnd(int64(v)) }),
			integerRow(th, "Fade in", ctx.page.integer["soundFadeInMs"], func(v int) { play.FadeInMs = int64(max(0, v)) }),
			integerRow(th, "Fade out", ctx.page.integer["soundFadeOutMs"], func(v int) { play.FadeOutMs = int64(max(0, v)) }),
		)
	} else if play := ctx.cue.Play.Video; play != nil {
		rows = append(rows,
			integerRow(th, "Clip start", ctx.page.integer["videoClipStartMs"], func(v int) { ctx.setTimecodeClipStart(int64(v)) }),
			integerRow(th, "Clip end", ctx.page.integer["videoClipEndMs"], func(v int) { ctx.setTimecodeClipEnd(int64(v)) }),
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
	_, clipStart, clipEnd, _ := ctx.timecodeMediaDetails()
	clipDuration := max(int64(0), clipEnd-clipStart)
	fadeIn, fadeOut := ctx.timecodeFades()
	center := size.Y / 2
	amp := float64(size.Y) * 0.40
	for x := 0; x < size.X; x++ {
		fromMs := t.viewStartMs + int64(float64(x)/float64(size.X)*float64(t.viewDuration()))
		toMs := t.viewStartMs + int64(float64(x+1)/float64(size.X)*float64(t.viewDuration()))
		a := int(fromMs * int64(t.waveSampleRate) / 1000)
		b := int(toMs * int64(t.waveSampleRate) / 1000)
		a = max(0, min(a, len(t.waveSamples)))
		b = max(a+1, min(max(a+1, b), len(t.waveSamples)))
		peak := float32(0)
		for i := a; i < b && i < len(t.waveSamples); i++ {
			peak = max(peak, t.waveSamples[i])
		}
		timelineMs := fromMs - clipStart
		gain := float64(0.25)
		if timelineMs >= 0 && timelineMs <= clipDuration {
			gain = 1
			if fadeIn > 0 {
				gain = min(gain, max(float64(0), float64(timelineMs)/float64(fadeIn)))
			}
			if fadeOut > 0 {
				gain = min(gain, max(float64(0), float64(clipDuration-timelineMs)/float64(fadeOut)))
			}
		}
		h := max(1, int(float64(peak)*amp*gain))
		// TODO(micro): waveform alpha 0xD0 and 0.40 amp are magic; name waveformAlpha/waveformAmplitude consts.
		paint.FillShape(gtx.Ops, palette.WithAlpha(palette.Success, 0xD0), clip.Rect{Min: image.Pt(x, center-h), Max: image.Pt(x+1, center+h)}.Op())
	}
}

func (ctx *CueEditUI) drawFadeZones(gtx layout.Context, size image.Point) {
	t := &ctx.timeline
	fadeIn, fadeOut := ctx.timecodeFades()
	clipStart, clipEnd, ok := ctx.timecodeClipRange()
	if !ok {
		clipStart, clipEnd = 0, ctx.timecodeCueDuration()
	}
	clipDuration := max(int64(0), clipEnd-clipStart)
	zone := palette.WithAlpha(palette.Accent, 0x24)
	if fadeIn > 0 {
		x1, x2 := t.msToX(clipStart, size.X), t.msToX(clipStart+min(fadeIn, clipDuration), size.X)
		paint.FillShape(gtx.Ops, zone, clip.Rect{Min: image.Pt(max(0, x1), 0), Max: image.Pt(min(size.X, x2), size.Y)}.Op())
	}
	if fadeOut > 0 {
		x1, x2 := t.msToX(clipStart+max(int64(0), clipDuration-fadeOut), size.X), t.msToX(clipEnd, size.X)
		paint.FillShape(gtx.Ops, zone, clip.Rect{Min: image.Pt(max(0, x1), 0), Max: image.Pt(min(size.X, x2), size.Y)}.Op())
	}
	bottom := size.Y - 34
	inX := t.msToX(clipStart+fadeIn, size.X)
	outX := t.msToX(clipStart+max(int64(0), clipDuration-fadeOut), size.X)
	handle := palette.Accent
	paint.FillShape(gtx.Ops, handle, clip.Rect{Min: image.Pt(inX-2, bottom), Max: image.Pt(inX+3, size.Y)}.Op())
	paint.FillShape(gtx.Ops, handle, clip.Rect{Min: image.Pt(outX-2, bottom), Max: image.Pt(outX+3, size.Y)}.Op())
}

func (ctx *CueEditUI) drawClipHandles(gtx layout.Context, size image.Point) {
	startMs, endMs, ok := ctx.timecodeClipRange()
	if !ok || endMs <= startMs {
		return
	}
	t := &ctx.timeline
	startX := t.msToX(startMs, size.X)
	endX := t.msToX(endMs, size.X)
	height := min(34, size.Y)
	paint.FillShape(gtx.Ops, palette.Success, clip.Rect{Min: image.Pt(startX-3, 0), Max: image.Pt(startX+4, height)}.Op())
	paint.FillShape(gtx.Ops, palette.Warning, clip.Rect{Min: image.Pt(endX-3, 0), Max: image.Pt(endX+4, height)}.Op())
}

func (ctx *CueEditUI) drawActionDurationBars(gtx layout.Context, size image.Point, markers []show.TimecodeMarker) {
	t := &ctx.timeline
	for i := range markers {
		duration := markerActionDuration(&markers[i])
		if duration == nil || *duration <= 0 {
			continue
		}
		x1 := t.msToX(ctx.timelineCueToTrackMs(markers[i].TimeMs), size.X)
		x2 := t.msToX(ctx.timelineCueToTrackMs(markers[i].TimeMs+*duration), size.X)
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
			status := "Drag the top start/end handles, actions or fade bars; bottom handles adjust fades; wheel pans; Ctrl+wheel zooms."
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
