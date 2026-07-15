package ui

import (
	"fmt"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/config"
)

const settingsAudioSectionIndex = 2

func (p *SettingsPage) Layout(th *material.Theme, gtx layout.Context) layout.Dimensions {
	if !p.initialized {
		p.load()
	}
	p.handleClicks(gtx)
	sections := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions { return p.header(th, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.defaultsSection(th, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.audioSection(th, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.videoOutputsSection(th, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.timecodeSection(th, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.redundancySection(th, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.targetsSection(th, gtx) },
		func(gtx layout.Context) layout.Dimensions { return p.variablesSection(th, gtx) },
	}
	return p.list.Layout(gtx, len(sections), func(gtx layout.Context, index int) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(14), Bottom: unit.Dp(6)}.Layout(gtx, sections[index])
	})
}

func (p *SettingsPage) handleClicks(gtx layout.Context) {
	for {
		event, ok := gtx.Event(key.Filter{Name: "S", Required: key.ModShortcut})
		if !ok {
			break
		}
		if event, ok := event.(key.Event); ok && event.State == key.Press {
			p.saveSettings()
		}
	}
	if p.reload.Clicked(gtx) {
		p.load()
	}
	if p.reopenOutputs.Clicked(gtx) && p.onReopenOutputs != nil {
		p.onReopenOutputs()
		p.status, p.statusError = "Output windows reopened", false
	}
	if p.supportBundle.Clicked(gtx) && p.onSupportBundle != nil {
		path, err := p.onSupportBundle()
		if err != nil {
			p.status, p.statusError = "Support bundle failed: "+err.Error(), true
		} else {
			p.status, p.statusError = "Support bundle saved: "+path, false
		}
	}
	if p.takeAuthority.Clicked(gtx) && p.onTakeAuthority != nil {
		if err := p.onTakeAuthority(); err != nil {
			p.status, p.statusError = "Take authority failed: "+err.Error(), true
		} else {
			p.status, p.statusError = "Command authority acquired", false
		}
	}
	if p.releaseAuthority.Clicked(gtx) && p.onReleaseAuthority != nil {
		if err := p.onReleaseAuthority(); err != nil {
			p.status, p.statusError = "Release authority failed: "+err.Error(), true
		} else {
			p.status, p.statusError = "Command authority released", false
		}
	}
	if p.refreshAudioDevices.Clicked(gtx) {
		p.refreshAudioDeviceList()
	}
	if p.refreshDisplays.Clicked(gtx) {
		p.refreshVideoDisplayList()
	}
	if p.addVideoOutput.Clicked(gtx) {
		index := len(p.videoOutputs) + 1
		p.videoOutputs = append(p.videoOutputs, newVideoOutputFields(config.DefaultVideoOutput(fmt.Sprintf("stage-%d", index)), p.videoDisplays))
	}
	if p.addTarget.Clicked(gtx) {
		p.targets = append(p.targets, newRemoteTargetFields(config.RemoteTarget{Name: fmt.Sprintf("Target %d", len(p.targets)+1), Host: "127.0.0.1", OSCPort: 8000, ERCPort: 6553}))
	}
	if p.addVariable.Clicked(gtx) {
		p.variables = append(p.variables, newVariableFields("", ""))
	}
	for index := len(p.targets) - 1; index >= 0; index-- {
		if p.targets[index].remove.Clicked(gtx) {
			p.targets = append(p.targets[:index], p.targets[index+1:]...)
		}
	}
	for index := len(p.variables) - 1; index >= 0; index-- {
		if p.variables[index].remove.Clicked(gtx) {
			p.variables = append(p.variables[:index], p.variables[index+1:]...)
		}
	}
	for index := len(p.videoOutputs) - 1; index >= 0; index-- {
		if p.videoOutputs[index].remove.Clicked(gtx) {
			p.videoOutputs = append(p.videoOutputs[:index], p.videoOutputs[index+1:]...)
		}
	}
	if p.save.Clicked(gtx) {
		p.saveSettings()
	}
}

func (p *SettingsPage) saveSettings() {
	settings := p.store.Snapshot()
	collectors := []func(*config.Settings) error{
		p.settingsDefaultsModel.collect,
		p.settingsAudioModel.collect,
		p.settingsVideoModel.collect,
		p.settingsTimecodeModel.collect,
		p.settingsRedundancyModel.collect,
		p.settingsTargetsModel.collect,
		p.settingsVariablesModel.collect,
	}
	for _, collect := range collectors {
		if err := collect(&settings); err != nil {
			p.status, p.statusError = err.Error(), true
			return
		}
	}
	if err := p.store.Update(settings); err != nil {
		p.status, p.statusError = err.Error(), true
		return
	}
	p.status, p.statusError = "Settings saved", false
	if p.onSaved != nil {
		p.onSaved()
	}
}
