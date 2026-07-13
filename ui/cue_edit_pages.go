package ui

import (
	"gioui.org/layout"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
)

func (ctx *CueEditUI) drawBody(th *material.Theme, gtx layout.Context, manager *show.ShowManager) layout.FlexChild {
	ctx.ensurePageInputs()
	if ctx.focusFirstInput {
		ctx.focusActiveTab()
		ctx.focusFirstInput = false
	}

	switch ctx.activeTab {
	case tabGeneral:
		return ctx.renderGeneralTab(th, gtx)
	case tabTiming:
		return ctx.renderTimingTab(th, gtx)
	case tabLink:
		return ctx.renderLinkTab(th, gtx, manager)
	case tabMedia:
		return ctx.renderMediaTab(th, gtx)
	case tabTimecode:
		return ctx.renderTimecodeTab(th, gtx, manager)
	case tabRemote:
		return ctx.renderRemoteTab(th, gtx)
	case tabWait:
		return ctx.renderWaitTab(th, gtx, manager)
	case tabMediaCtrl:
		return ctx.renderMediaCtrlTab(th, gtx, manager)
	case tabOutputCtrl:
		return ctx.renderOutputCtrlTab(th, gtx)
	}
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{}
	})
}

func (ctx *CueEditUI) focusActiveTab() {
	switch ctx.activeTab {
	case tabGeneral:
		ctx.page.text["cueNumber"].Focus()
	case tabTiming:
		ctx.page.integer["preWaitMs"].Focus()
	case tabLink:
		ctx.page.dropdown["linkMode"].Focus()
	case tabMedia:
		switch ctx.cType {
		case show.CueTypeSound:
			ctx.page.text["soundFile"].Focus()
		case show.CueTypeVideo:
			ctx.page.text["videoFile"].Focus()
		case show.CueTypeImage:
			ctx.page.text["imageFile"].Focus()
		}
	case tabRemote:
		ctx.page.dropdown["remoteProtocol"].Focus()
	case tabWait:
		ctx.page.dropdown["waitKind"].Focus()
	case tabMediaCtrl:
		ctx.page.dropdown["mediaCtrlAction"].Focus()
	case tabOutputCtrl:
		ctx.page.dropdown["outputCtrlAction"].Focus()
	}
}

type cueEditFormRow struct {
	label  string
	layout func(gtx layout.Context) layout.Dimensions
}

var (
	cueLinkModeLabels = []string{
		"Manual",
		"Start Advance",
		"Start Play",
		"Fade In Advance",
		"Fade In Play",
		"Fade Out Advance",
		"Fade Out Play",
		"End Advance",
		"End Play",
	}
	cueTargetKindLabels = []string{
		"None",
		"Next",
		"Previous",
		"Cue ID",
	}
	remoteProtocolLabels = []string{
		"Auto",
		"OSC",
		"ERC",
	}
	remoteActionLabels = []string{
		"None",
		"Go",
		"Go to",
		"Back",
		"Release",
		"Level",
		"Activate",
		"Flash",
		"Custom",
	}
	waitKindLabels = []string{
		"Duration",
		"Media Start",
		"Media End",
		"Fade In Complete",
		"Fade Out Complete",
		"Instance Stopped",
		"All Audio Stopped",
		"All Video Stopped",
		"All Media Stopped",
	}
	mediaTargetKindLabels = []string{
		"Cue ID",
		"Instance ID",
		"All Audio",
		"All Video",
		"All Media",
		"Output ID",
		"Current Track",
		"Cue Group",
	}
	mediaControlActionLabels = []string{
		"Fade To",
		"Fade Out",
		"Stop",
		"Pause",
		"Resume",
		"Seek",
		"Set Volume",
		"Mute",
		"Unmute",
	}
	fadeCurveLabels = []string{
		"Linear",
		"Equal Power",
	}
	outputControlActionLabels = []string{
		"Blackout",
		"Clear",
		"Test Pattern",
		"Identify",
		"Reopen",
		"Fullscreen",
		"Exit Fullscreen",
	}
	timecodeActionLabels = []string{
		"Current track",
		"Output control",
		"Remote",
	}
	videoFileExtensions = []string{".mp4", ".mov", ".mkv", ".webm", ".avi"}
	soundFileExtensions = []string{".wav", ".mp3", ".flac", ".ogg", ".aiff", ".aif", ".m4a", ".opus"}
	imageFileExtensions = []string{".png", ".jpg", ".jpeg", ".webp", ".gif"}
)

func (ctx *CueEditUI) ensurePageInputs() {
	if ctx.page.initialized && ctx.page.cueID == ctx.cue.ID {
		return
	}

	ctx.ensureCuePlay()
	ctx.cType = ctx.cue.Type
	ctx.activeTab = tabGeneral
	ctx.page = newCueEditPageState(ctx.cue)
}

func (ctx *CueEditUI) ensureCuePlay() {
	switch ctx.cue.Type {
	case show.CueTypeSound:
		if ctx.cue.Play.Sound == nil {
			ctx.cue.Play.Sound = show.NewSoundCue().Play.Sound
		}
	case show.CueTypeVideo:
		if ctx.cue.Play.Video == nil {
			ctx.cue.Play.Video = show.NewVideoCue().Play.Video
		}
	case show.CueTypeImage:
		if ctx.cue.Play.Image == nil {
			ctx.cue.Play.Image = show.NewImageCue().Play.Image
		}
	case show.CueTypeRemote:
		if ctx.cue.Play.Remote == nil {
			ctx.cue.Play.Remote = show.NewRemoteCue().Play.Remote
		}
	case show.CueTypeWait:
		if ctx.cue.Play.Wait == nil {
			ctx.cue.Play.Wait = show.NewWaitCue().Play.Wait
		}
	case show.CueTypeMediaControl:
		if ctx.cue.Play.MediaControl == nil {
			ctx.cue.Play.MediaControl = show.NewMediaControlCue().Play.MediaControl
		}
	case show.CueTypeOutputControl:
		if ctx.cue.Play.OutputControl == nil {
			ctx.cue.Play.OutputControl = show.NewOutputControlCue().Play.OutputControl
		}
	}
}
