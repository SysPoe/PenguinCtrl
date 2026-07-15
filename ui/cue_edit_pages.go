package ui

import (
	"gioui.org/layout"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/show"
)

func (ctx *CueEditUI) drawBody(th *material.Theme, manager *show.ShowManager) layout.FlexChild {
	ctx.ensurePageInputs()
	if ctx.tabs.focusFirstInput {
		ctx.tabs.focus(&ctx.page)
		ctx.tabs.focusFirstInput = false
	}

	switch ctx.tabs.active {
	case tabGeneral:
		return ctx.renderGeneralTab(th)
	case tabTiming:
		return ctx.renderTimingTab(th)
	case tabLink:
		return ctx.renderLinkTab(th, manager)
	case tabMedia:
		return ctx.renderMediaTab(th)
	case tabTimecode:
		return ctx.renderTimecodeTab(th, manager)
	case tabRemote:
		return ctx.renderRemoteTab(th)
	case tabWait:
		return ctx.renderWaitTab(th, manager)
	case tabMediaCtrl:
		return ctx.renderMediaCtrlTab(th, manager)
	case tabOutputCtrl:
		return ctx.renderOutputCtrlTab(th)
	}
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{}
	})
}
