package ui

import (
	"gioui.org/layout"
	"gioui.org/widget/material"
)

func (ctx *CueEditUI) DrawBody(th *material.Theme, gtx layout.Context) layout.FlexChild {
	switch ctx.activeTab {
	case TabGeneral:
		return ctx.renderGeneralTab(th, gtx)
	case TabTiming:
		return ctx.renderTimingTab(th, gtx)
	case TabLink:
		return ctx.renderLinkTab(th, gtx)
	case TabMedia:
		return ctx.renderMediaTab(th, gtx)
	case TabTimecode:
		return ctx.renderTimecodeTab(th, gtx)
	case TabRemote:
		return ctx.renderRemoteTab(th, gtx)
	case TabWait:
		return ctx.renderWaitTab(th, gtx)
	case TabMediaCtrl:
		return ctx.renderMediaCtrlTab(th, gtx)
	case TabOutputCtrl:
		return ctx.renderOutputCtrlTab(th, gtx)
	}
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{}
	})
}

func (ctx *CueEditUI) renderGeneralTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	/*
		- CueNumber: text
		- Title: text
		- Description: multiline text
		- Disabled: checkbox/toggle
		- HexColor: text or color picker
		- Tags: tag list / tokenized text
		- Notes: multiline text
	*/
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return material.Body1(th, "General Tab").Layout(gtx)
	})
}

func (ctx *CueEditUI) renderTimingTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	/*
		- PreWaitMS: int
		- PostWaitMS: int
	*/
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return material.Body1(th, "Timing Tab").Layout(gtx)
	})
}

func (ctx *CueEditUI) renderLinkTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	/*
		- Mode: dropdown
		- Target.Kind: dropdown
		- Target.CueID: cue picker / dropdown when kind is CueTargetCue
	*/
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return material.Body1(th, "Link Tab").Layout(gtx)
	})
}

func (ctx *CueEditUI) renderMediaTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	/*
		Sound:
			- File: file picker / text
			- ClipStartMS: int
			- ClipEndMS: int
			- FadeInMS: int
			- FadeOutMS: int
			- LevelDB: float or slider
		Video
			- File: file picker / text
			- OutputID: dropdown or text
			- ClipStartMS: int
			- ClipEndMS: int
			- FadeInMS: int
			- FadeOutMS: int
			- LevelDB: float or slider
		Image
			- File: file picker / text
			- OutputID: dropdown or text
			- FadeInMS: int
			- FadeOutMS: int
			- DurationMS: int
	*/
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return material.Body1(th, "Media Tab").Layout(gtx)
	})
}

func (ctx *CueEditUI) renderTimecodeTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	// Ugh
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return material.Body1(th, "Timecode Tab").Layout(gtx)
	})
}

func (ctx *CueEditUI) renderRemoteTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	/*
		- Protocol: dropdown
		- Action: dropdown
		- Playback: text
		- CueNumber: text
		- Level: text
	*/
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return material.Body1(th, "Remote Tab").Layout(gtx)
	})
}

func (ctx *CueEditUI) renderWaitTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	/*
		- Kind: dropdown
		- DurationMS: int for WaitDuration
		- Target.Kind: dropdown for cue-based waits
		- Target.CueID: cue picker / dropdown
		- Media.Kind: dropdown for media-based waits
		- Media.CueID: cue picker / dropdown when targeting a cue
		- Media.InstanceID: text
		- Media.OutputID: dropdown or text
	*/
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return material.Body1(th, "Wait Tab").Layout(gtx)
	})
}

func (ctx *CueEditUI) renderMediaCtrlTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	/*
		- Action: dropdown
		- Target.Kind: dropdown
		- Target.CueID: cue picker / dropdown
		- Target.InstanceID: text
		- Target.OutputID: dropdown or text
		- LevelDB: float or slider, only for volume/fade actions
		- SeekToMS: int, only for seek
		- FadeMS: int
		- Curve: dropdown
	*/
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return material.Body1(th, "Media Ctrl Tab").Layout(gtx)
	})
}

func (ctx *CueEditUI) renderOutputCtrlTab(th *material.Theme, gtx layout.Context) layout.FlexChild {
	/*
		- Action: dropdown
		- OutputID: dropdown or text
		- FadeOutMS: int
		- FadeInMS: int
		- Message: text
	*/
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return material.Body1(th, "Output Ctrl Tab").Layout(gtx)
	})
}
