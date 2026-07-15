package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
)

func (ctx *CueEditUI) renderGeneralTab(th *material.Theme) layout.FlexChild {
	fields := &ctx.page.general
	return ctx.renderForm(th, []cueEditFormRow{
		textRow(th, "Cue Number", fields.cueNumber, func(value string) { ctx.cue.CueNumber = value }),
		multilineRow(th, "Description", fields.description, func(value string) { ctx.cue.Description = value }),
		colourRow(th, "Color", fields.color, func(value color.NRGBA) { ctx.cue.Color = value }),
		textRow(th, "Tags", fields.tags, func(value string) { ctx.cue.Tags = splitTags(value) }),
		multilineRow(th, "Notes", fields.notes, func(value string) { ctx.cue.Notes = value }),
	})
}

func (ctx *CueEditUI) renderTimingTab(th *material.Theme) layout.FlexChild {
	fields := &ctx.page.timing
	return ctx.renderForm(th, []cueEditFormRow{
		integerRow(th, "Pre Wait MS", fields.preWaitMs, func(value int) { ctx.cue.Timing.PreWaitMs = int64(value) }),
		integerRow(th, "Post Wait MS", fields.postWaitMs, func(value int) { ctx.cue.Timing.PostWaitMs = int64(value) }),
	})
}

func (ctx *CueEditUI) renderLinkTab(th *material.Theme, manager *show.ShowManager) layout.FlexChild {
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

func (ctx *CueEditUI) renderTimecodeTab(th *material.Theme, manager *show.ShowManager) layout.FlexChild {
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
