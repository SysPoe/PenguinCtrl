package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
)

// TODO(micro): gtx is unused across tab renderers; drop the param or use it if layout needs constraints
func (ctx *CueEditUI) renderGeneralTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	fields := &ctx.page.general
	return ctx.renderForm(th, []cueEditFormRow{
		textRow(th, "Cue Number", fields.cueNumber, func(value string) { ctx.cue.CueNumber = value }),
		multilineRow(th, "Description", fields.description, func(value string) { ctx.cue.Description = value }),
		colourRow(th, "Color", fields.color, func(value color.NRGBA) { ctx.cue.Color = value }),
		textRow(th, "Tags", fields.tags, func(value string) { ctx.cue.Tags = splitTags(value) }),
		multilineRow(th, "Notes", fields.notes, func(value string) { ctx.cue.Notes = value }),
	})
}

func (ctx *CueEditUI) renderTimingTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	fields := &ctx.page.timing
	return ctx.renderForm(th, []cueEditFormRow{
		integerRow(th, "Pre Wait MS", fields.preWaitMs, func(value int) { ctx.cue.Timing.PreWaitMs = int64(value) }),
		integerRow(th, "Post Wait MS", fields.postWaitMs, func(value int) { ctx.cue.Timing.PostWaitMs = int64(value) }),
	})
}

func (ctx *CueEditUI) renderLinkTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	fields := &ctx.page.link
	rows := []cueEditFormRow{
		dropdownRow(th, "Mode", fields.mode, func(selected int) {
			ctx.cue.Link.Mode = show.CueLinkMode(selected)
			if ctx.cue.Link.Mode != show.CueLinkManual && ctx.cue.Link.Target.Kind == show.CueTargetNone {
				ctx.cue.Link.Target.Kind = show.CueTargetNext
				fields.targetKind.Selected = int(show.CueTargetNext)
			}
		}),
		dropdownRow(th, "Target", fields.targetKind, func(selected int) {
			ctx.cue.Link.Target.Kind = show.CueTargetKind(selected)
		}),
	}
	if ctx.cue.Link.Target.Kind == show.CueTargetCue {
		rows = append(rows, ctx.cueTargetDropdownRow(th, "Target Cue", &fields.targetCue, manager, &ctx.cue.Link.Target.CueID))
	}
	return ctx.renderForm(th, rows)
}

// TODO(macro): Media/Remote/Wait/MediaCtrl/OutputCtrl tabs are hand-written field lists that mirror show.Cue play payloads and partially duplicate timecode marker action forms. Generate tab schemas from typed field groups (or shared action-form builders) so cue-type coverage and marker editors stay in lockstep.
func (ctx *CueEditUI) renderMediaTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	rows := []cueEditFormRow{}
	fields := ctx.page.media
	if play := ctx.cue.Play.Sound; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", "audio", fields.file, fields.projectFile, fields.fileBrowse, soundFileExtensions, func(value string) {
				ctx.setTimecodeMediaSource(&play.File, &play.ClipEndMs, fields.clipEndMs, value)
			}),
			textRow(th, "Output ID", fields.outputID, func(value string) { play.OutputID = value }),
		)
		rows = append(rows, ctx.mediaRangeRows(th, mediaTabRangeLabels, false)...)
		rows = append(rows, floatRow(th, "Level dB", fields.levelDB, func(value float64) { play.LevelDB = value }))
	}
	if play := ctx.cue.Play.Video; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", "video", fields.file, fields.projectFile, fields.fileBrowse, videoFileExtensions, func(value string) {
				ctx.setTimecodeMediaSource(&play.File, &play.ClipEndMs, fields.clipEndMs, value)
			}),
			textRow(th, "Output ID", fields.outputID, func(value string) { play.OutputID = value }),
		)
		rows = append(rows, ctx.mediaRangeRows(th, mediaTabRangeLabels, false)...)
		rows = append(rows, floatRow(th, "Level dB", fields.levelDB, func(value float64) { play.LevelDB = value }))
	}
	if play := ctx.cue.Play.Image; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", "image", fields.file, fields.projectFile, fields.fileBrowse, imageFileExtensions, func(value string) { play.File = value }),
			textRow(th, "Output ID", fields.outputID, func(value string) { play.OutputID = value }),
		)
		rows = append(rows, ctx.mediaRangeRows(th, mediaTabRangeLabels, false)...)
	}
	if len(rows) == 0 {
		rows = append(rows, staticRow(th, "Media", "No media settings for this cue type."))
	}
	return ctx.renderForm(th, rows)
}

type mediaRangeLabels struct {
	clipStart string
	clipEnd   string
	fadeIn    string
	fadeOut   string
	duration  string
}

var (
	mediaTabRangeLabels = mediaRangeLabels{
		clipStart: "Clip Start MS",
		clipEnd:   "Clip End MS",
		fadeIn:    "Fade In MS",
		fadeOut:   "Fade Out MS",
		duration:  "Duration MS",
	}
	timecodeRangeLabels = mediaRangeLabels{
		clipStart: "Clip start",
		clipEnd:   "Clip end",
		fadeIn:    "Fade in",
		fadeOut:   "Fade out",
		duration:  "Duration",
	}
)

func (ctx *CueEditUI) mediaRangeRows(th *material.Theme, labels mediaRangeLabels, nonNegative bool) []cueEditFormRow {
	fields := ctx.page.media
	if fields == nil {
		return nil
	}
	value := func(v int) int64 {
		if nonNegative {
			return int64(max(0, v))
		}
		return int64(v)
	}
	if play := ctx.cue.Play.Sound; play != nil {
		return []cueEditFormRow{
			integerRow(th, labels.clipStart, fields.clipStartMs, func(v int) { ctx.setTimecodeClipStart(int64(v)) }),
			integerRow(th, labels.clipEnd, fields.clipEndMs, func(v int) { ctx.setTimecodeClipEnd(int64(v)) }),
			integerRow(th, labels.fadeIn, fields.fadeInMs, func(v int) { play.FadeInMs = value(v) }),
			integerRow(th, labels.fadeOut, fields.fadeOutMs, func(v int) { play.FadeOutMs = value(v) }),
		}
	}
	if play := ctx.cue.Play.Video; play != nil {
		return []cueEditFormRow{
			integerRow(th, labels.clipStart, fields.clipStartMs, func(v int) { ctx.setTimecodeClipStart(int64(v)) }),
			integerRow(th, labels.clipEnd, fields.clipEndMs, func(v int) { ctx.setTimecodeClipEnd(int64(v)) }),
			integerRow(th, labels.fadeIn, fields.fadeInMs, func(v int) { play.FadeInMs = value(v) }),
			integerRow(th, labels.fadeOut, fields.fadeOutMs, func(v int) { play.FadeOutMs = value(v) }),
		}
	}
	if play := ctx.cue.Play.Image; play != nil {
		return []cueEditFormRow{
			integerRow(th, labels.duration, fields.durationMs, func(v int) { play.DurationMs = value(v) }),
			integerRow(th, labels.fadeIn, fields.fadeInMs, func(v int) { play.FadeInMs = value(v) }),
			integerRow(th, labels.fadeOut, fields.fadeOutMs, func(v int) { play.FadeOutMs = value(v) }),
		}
	}
	return nil
}

func (ctx *CueEditUI) renderTimecodeTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	markers := cueTimecodeMarkers(&ctx.cue)
	if markers == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Timecode", "This cue type does not support timecode markers.")})
	}

	if sortTimecodeMarkers(markers) {
		ctx.timeline.selected = map[int]bool{}
		ctx.resetTimecodeInputs()
	}

	rows := ctx.timecodeEditorRows(th, markers)
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ctx.renderNativeTimecodeEditor(th, gtx, manager, markers)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return ctx.layoutFormRows(th, gtx, rows)
				}),
			)
		})
	})
}

func cueTimecodeMarkers(cue *show.Cue) *[]show.TimecodeMarker {
	if cue == nil {
		return nil
	}
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			return &cue.Play.Sound.Timecode
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			return &cue.Play.Video.Timecode
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			return &cue.Play.Image.Timecode
		}
	}
	return nil
}

func (ctx *CueEditUI) renderRemoteTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	play := ctx.cue.Play.Remote
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Remote", "No remote settings for this cue type.")})
	}
	fields := ctx.page.remote
	rows := []cueEditFormRow{
		dropdownRow(th, "Protocol", fields.protocol, func(selected int) { play.Protocol = show.RemoteProtocol(selected) }),
		dropdownRow(th, "Action", fields.action, func(selected int) { play.Action = show.RemoteAction(selected) }),
		textRow(th, "Playback", fields.playback, func(value string) { play.Playback = value }),
		textRow(th, "Cue Number", fields.cueNumber, func(value string) { play.CueNumber = value }),
		textRow(th, "Level", fields.level, func(value string) { play.Level = value }),
	}
	if play.Action == show.RemoteActionCustom {
		rows = append(rows, textRow(th, "Custom Command", fields.custom, func(value string) { play.Custom = value }))
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderWaitTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	play := ctx.cue.Play.Wait
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Wait", "No wait settings for this cue type.")})
	}
	fields := ctx.page.wait

	rows := []cueEditFormRow{
		dropdownRow(th, "Kind", fields.kind, func(selected int) { play.Kind = show.WaitKind(selected) }),
	}
	if play.Kind == show.WaitDuration {
		rows = append(rows, integerRow(th, "Duration MS", fields.durationMs, func(value int) { play.DurationMs = int64(value) }))
	} else {
		// TODO(micro): Collapse this nested branch to "else if" to reduce indentation.
		if waitKindUsesMediaTarget(play.Kind) {
			rows = ctx.appendMediaTargetRows(rows, th, manager, &fields.target, &play.Media)
		}
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderMediaCtrlTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	play := ctx.cue.Play.MediaControl
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Media Control", "No media control settings for this cue type.")})
	}
	fields := ctx.page.mediaControl

	rows := []cueEditFormRow{
		dropdownRow(th, "Action", fields.action, func(selected int) {
			play.Action = show.MediaControlAction(selected)
			syncMediaControlOptionals(play, fields)
		}),
	}
	rows = ctx.appendMediaTargetRows(rows, th, manager, &fields.target, &play.Target)
	if mediaControlActionUsesLevel(play.Action) {
		rows = append(rows, floatRow(th, "Level dB", fields.levelDB, func(value float64) { play.LevelDB = &value }))
	}
	if play.Action == show.MediaControlSeek {
		rows = append(rows, integerRow(th, "Seek To MS", fields.seekToMs, func(value int) { play.SeekToMs = ptr(int64(value)) }))
	}
	rows = append(rows,
		integerRow(th, "Fade MS", fields.fadeMs, func(value int) { play.FadeMs = int64(value) }),
		dropdownRow(th, "Curve", fields.curve, func(selected int) { play.Curve = show.FadeCurve(selected) }),
	)
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderOutputCtrlTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	play := ctx.cue.Play.OutputControl
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Output Control", "No output control settings for this cue type.")})
	}
	fields := ctx.page.outputControl
	return ctx.renderForm(th, []cueEditFormRow{
		dropdownRow(th, "Action", fields.action, func(selected int) { play.Action = show.OutputControlAction(selected) }),
		textRow(th, "Output ID", fields.outputID, func(value string) { play.OutputID = value }),
		integerRow(th, "Fade Out MS", fields.fadeOutMs, func(value int) { play.FadeOutMs = int64(value) }),
		integerRow(th, "Fade In MS", fields.fadeInMs, func(value int) { play.FadeInMs = int64(value) }),
		textRow(th, "Message", fields.message, func(value string) { play.Message = value }),
	})
}
