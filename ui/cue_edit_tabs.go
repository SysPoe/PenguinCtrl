package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
)

func (ctx *CueEditUI) renderGeneralTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	return ctx.renderForm(th, []cueEditFormRow{
		textRow(th, "Cue Number", ctx.page.text["cueNumber"], func(value string) { ctx.cue.CueNumber = value }),
		multilineRow(th, "Description", ctx.page.multiline["description"], func(value string) { ctx.cue.Description = value }),
		colourRow(th, "Color", ctx.page.colour["color"], func(value color.NRGBA) { ctx.cue.Color = value }),
		textRow(th, "Tags", ctx.page.text["tags"], func(value string) { ctx.cue.Tags = splitTags(value) }),
		multilineRow(th, "Notes", ctx.page.multiline["notes"], func(value string) { ctx.cue.Notes = value }),
	})
}

func (ctx *CueEditUI) renderTimingTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	return ctx.renderForm(th, []cueEditFormRow{
		integerRow(th, "Pre Wait MS", ctx.page.integer["preWaitMs"], func(value int) { ctx.cue.Timing.PreWaitMs = int64(value) }),
		integerRow(th, "Post Wait MS", ctx.page.integer["postWaitMs"], func(value int) { ctx.cue.Timing.PostWaitMs = int64(value) }),
	})
}

func (ctx *CueEditUI) renderLinkTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	rows := []cueEditFormRow{
		dropdownRow(th, "Mode", ctx.page.dropdown["linkMode"], func(selected int) {
			ctx.cue.Link.Mode = show.CueLinkMode(selected)
			if ctx.cue.Link.Mode != show.CueLinkManual && ctx.cue.Link.Target.Kind == show.CueTargetNone {
				ctx.cue.Link.Target.Kind = show.CueTargetNext
				ctx.page.dropdown["linkTargetKind"].Selected = int(show.CueTargetNext)
			}
		}),
		dropdownRow(th, "Target", ctx.page.dropdown["linkTargetKind"], func(selected int) {
			ctx.cue.Link.Target.Kind = show.CueTargetKind(selected)
		}),
	}
	if ctx.cue.Link.Target.Kind == show.CueTargetCue {
		rows = append(rows, ctx.cueTargetDropdownRow(th, "Target Cue", "linkTargetCue", manager, &ctx.cue.Link.Target.CueID))
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderMediaTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	rows := []cueEditFormRow{}
	if play := ctx.cue.Play.Sound; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", "audio", ctx.page.text["soundFile"], ctx.page.dropdown["soundProjectFile"], ctx.page.button["soundFileBrowse"], soundFileExtensions, func(value string) {
				ctx.setTimecodeMediaSource(&play.File, &play.ClipEndMs, "soundClipEndMs", value)
			}),
			textRow(th, "Output ID", ctx.page.text["soundOutputID"], func(value string) { play.OutputID = value }),
			integerRow(th, "Clip Start MS", ctx.page.integer["soundClipStartMs"], func(value int) { ctx.setTimecodeClipStart(int64(value)) }),
			integerRow(th, "Clip End MS", ctx.page.integer["soundClipEndMs"], func(value int) { ctx.setTimecodeClipEnd(int64(value)) }),
			integerRow(th, "Fade In MS", ctx.page.integer["soundFadeInMs"], func(value int) { play.FadeInMs = int64(value) }),
			integerRow(th, "Fade Out MS", ctx.page.integer["soundFadeOutMs"], func(value int) { play.FadeOutMs = int64(value) }),
			floatRow(th, "Level dB", ctx.page.float["soundLevelDB"], func(value float64) { play.LevelDB = value }),
		)
	}
	if play := ctx.cue.Play.Video; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", "video", ctx.page.text["videoFile"], ctx.page.dropdown["videoProjectFile"], ctx.page.button["videoFileBrowse"], videoFileExtensions, func(value string) {
				ctx.setTimecodeMediaSource(&play.File, &play.ClipEndMs, "videoClipEndMs", value)
			}),
			textRow(th, "Output ID", ctx.page.text["videoOutputID"], func(value string) { play.OutputID = value }),
			integerRow(th, "Clip Start MS", ctx.page.integer["videoClipStartMs"], func(value int) { ctx.setTimecodeClipStart(int64(value)) }),
			integerRow(th, "Clip End MS", ctx.page.integer["videoClipEndMs"], func(value int) { ctx.setTimecodeClipEnd(int64(value)) }),
			integerRow(th, "Fade In MS", ctx.page.integer["videoFadeInMs"], func(value int) { play.FadeInMs = int64(value) }),
			integerRow(th, "Fade Out MS", ctx.page.integer["videoFadeOutMs"], func(value int) { play.FadeOutMs = int64(value) }),
			floatRow(th, "Level dB", ctx.page.float["videoLevelDB"], func(value float64) { play.LevelDB = value }),
		)
	}
	if play := ctx.cue.Play.Image; play != nil {
		rows = append(rows,
			ctx.fileRow(th, "File", "image", ctx.page.text["imageFile"], ctx.page.dropdown["imageProjectFile"], ctx.page.button["imageFileBrowse"], imageFileExtensions, func(value string) { play.File = value }),
			textRow(th, "Output ID", ctx.page.text["imageOutputID"], func(value string) { play.OutputID = value }),
			integerRow(th, "Fade In MS", ctx.page.integer["imageFadeInMs"], func(value int) { play.FadeInMs = int64(value) }),
			integerRow(th, "Fade Out MS", ctx.page.integer["imageFadeOutMs"], func(value int) { play.FadeOutMs = int64(value) }),
			integerRow(th, "Duration MS", ctx.page.integer["imageDurationMs"], func(value int) { play.DurationMs = int64(value) }),
		)
	}
	if len(rows) == 0 {
		rows = append(rows, staticRow(th, "Media", "No media settings for this cue type."))
	}
	return ctx.renderForm(th, rows)
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
	rows := []cueEditFormRow{
		dropdownRow(th, "Protocol", ctx.page.dropdown["remoteProtocol"], func(selected int) { play.Protocol = show.RemoteProtocol(selected) }),
		dropdownRow(th, "Action", ctx.page.dropdown["remoteAction"], func(selected int) { play.Action = show.RemoteAction(selected) }),
		textRow(th, "Playback", ctx.page.text["remotePlayback"], func(value string) { play.Playback = value }),
		textRow(th, "Cue Number", ctx.page.text["remoteCueNumber"], func(value string) { play.CueNumber = value }),
		textRow(th, "Level", ctx.page.text["remoteLevel"], func(value string) { play.Level = value }),
	}
	if play.Action == show.RemoteActionCustom {
		rows = append(rows, textRow(th, "Custom Command", ctx.page.text["remoteCustom"], func(value string) { play.Custom = value }))
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderWaitTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	play := ctx.cue.Play.Wait
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Wait", "No wait settings for this cue type.")})
	}

	rows := []cueEditFormRow{
		dropdownRow(th, "Kind", ctx.page.dropdown["waitKind"], func(selected int) { play.Kind = show.WaitKind(selected) }),
	}
	if play.Kind == show.WaitDuration {
		rows = append(rows, integerRow(th, "Duration MS", ctx.page.integer["waitDurationMs"], func(value int) { play.DurationMs = int64(value) }))
	} else {
		// TODO(micro): Collapse this nested branch to "else if" to reduce indentation.
		if waitKindUsesMediaTarget(play.Kind) {
			rows = ctx.appendMediaTargetRows(rows, th, manager, "waitMedia", &play.Media)
		}
	}
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderMediaCtrlTab(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	play := ctx.cue.Play.MediaControl
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Media Control", "No media control settings for this cue type.")})
	}

	rows := []cueEditFormRow{
		dropdownRow(th, "Action", ctx.page.dropdown["mediaCtrlAction"], func(selected int) {
			play.Action = show.MediaControlAction(selected)
			syncMediaControlOptionals(play, ctx.page)
		}),
	}
	rows = ctx.appendMediaTargetRows(rows, th, manager, "mediaCtrl", &play.Target)
	if mediaControlActionUsesLevel(play.Action) {
		rows = append(rows, floatRow(th, "Level dB", ctx.page.float["mediaCtrlLevelDB"], func(value float64) { play.LevelDB = &value }))
	}
	if play.Action == show.MediaControlSeek {
		rows = append(rows, integerRow(th, "Seek To MS", ctx.page.integer["mediaCtrlSeekToMs"], func(value int) { play.SeekToMs = ptr(int64(value)) }))
	}
	rows = append(rows,
		integerRow(th, "Fade MS", ctx.page.integer["mediaCtrlFadeMs"], func(value int) { play.FadeMs = int64(value) }),
		dropdownRow(th, "Curve", ctx.page.dropdown["mediaCtrlCurve"], func(selected int) { play.Curve = show.FadeCurve(selected) }),
	)
	return ctx.renderForm(th, rows)
}

func (ctx *CueEditUI) renderOutputCtrlTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	play := ctx.cue.Play.OutputControl
	if play == nil {
		return ctx.renderForm(th, []cueEditFormRow{staticRow(th, "Output Control", "No output control settings for this cue type.")})
	}
	return ctx.renderForm(th, []cueEditFormRow{
		dropdownRow(th, "Action", ctx.page.dropdown["outputCtrlAction"], func(selected int) { play.Action = show.OutputControlAction(selected) }),
		textRow(th, "Output ID", ctx.page.text["outputCtrlOutputID"], func(value string) { play.OutputID = value }),
		integerRow(th, "Fade Out MS", ctx.page.integer["outputCtrlFadeOutMs"], func(value int) { play.FadeOutMs = int64(value) }),
		integerRow(th, "Fade In MS", ctx.page.integer["outputCtrlFadeInMs"], func(value int) { play.FadeInMs = int64(value) }),
		textRow(th, "Message", ctx.page.text["outputCtrlMessage"], func(value string) { play.Message = value }),
	})
}
