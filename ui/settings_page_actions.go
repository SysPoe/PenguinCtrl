package ui

import (
	"fmt"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/config"
)

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
		p.videoOutputs = append(p.videoOutputs, newVideoOutputFields(config.VideoOutput{
			// TODO(micro): default video stage geometry/resolution are magic; pull from config defaults or named consts.
			Stage: fmt.Sprintf("stage-%d", index), Width: 960, Height: 540,
			ResolutionWidth: 1920, ResolutionHeight: 1080,
			Scaling: "contain", IdleBehavior: "black", Layers: 1,
		}, p.videoDisplays))
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

// TODO(macro): saveSettings is a single cross-section serializer/validator that knows every field and hard-codes section order for ShowAudioDevices via list.Position.First. Per-section Collect()/Validate() would keep scroll targets, validation, and config mapping inside section boundaries.
func (p *SettingsPage) saveSettings() {
	settings := p.store.Snapshot()
	settings.FFmpegPath = strings.TrimSpace(p.ffmpegPath.Value)
	settings.DefaultPlayback = strings.TrimSpace(p.defaultPlayback.Value)
	settings.DefaultMediaOutput = strings.TrimSpace(p.defaultMediaOutput.Value)
	settings.CacheQuotaGB, settings.CacheReserveGB = p.cacheQuotaGB.Value, p.cacheReserveGB.Value
	settings.TimecodeSource = selectedDropdownValue(p.timecodeSource)
	settings.TimecodePolicy = selectedDropdownValue(p.timecodePolicy)
	settings.TimecodeListenAddress = strings.TrimSpace(p.timecodeListenAddress.Value)
	settings.TimecodeFrameRate = p.timecodeFrameRate.Value
	settings.RedundancyRole = selectedDropdownValue(p.redundancyRole)
	settings.RedundancyNodeID = strings.TrimSpace(p.redundancyNodeID.Value)
	settings.RedundancyListenAddress = strings.TrimSpace(p.redundancyListenAddress.Value)
	settings.RedundancyPeerAddress = strings.TrimSpace(p.redundancyPeerAddress.Value)
	settings.RedundancySharedKey = strings.TrimSpace(p.redundancySharedKey.Value)
	settings.RedundancyInterlockPath = strings.TrimSpace(p.redundancyInterlockPath.Value)
	if settings.RedundancyRole != config.RedundancyOff {
		if settings.RedundancyNodeID == "" || settings.RedundancyListenAddress == "" || settings.RedundancyPeerAddress == "" || settings.RedundancyInterlockPath == "" {
			p.status, p.statusError = "Redundancy requires node ID, heartbeat addresses, and a shared interlock path", true
			return
		}
		// TODO(micro): 16-char key min is a magic policy number; share with config validation const.
		if len(settings.RedundancySharedKey) < 16 {
			p.status, p.statusError = "Redundancy shared key must contain at least 16 characters", true
			return
		}
	}
	settings.PlaybackAudioDevice = selectedDropdownValue(p.playbackAudioDevice)
	settings.PlaybackAudioRecovery = selectedDropdownValue(p.playbackAudioRecovery)
	settings.PlaybackBackupAudioDevice = selectedDropdownValue(p.playbackBackupAudioDevice)
	settings.PreviewAudioDevice = selectedDropdownValue(p.previewAudioDevice)
	settings.PreviewAudioRecovery = selectedDropdownValue(p.previewAudioRecovery)
	settings.PreviewBackupAudioDevice = selectedDropdownValue(p.previewBackupAudioDevice)
	if settings.PlaybackAudioRecovery == config.AudioRecoveryNamedBackup && settings.PlaybackBackupAudioDevice == "" {
		p.status, p.statusError = "Playback named-backup policy requires a backup device", true
		return
	}
	if settings.PreviewAudioRecovery == config.AudioRecoveryNamedBackup && settings.PreviewBackupAudioDevice == "" {
		p.status, p.statusError = "Preview named-backup policy requires a backup device", true
		return
	}
	settings.VideoOutputs = make([]config.VideoOutput, 0, len(p.videoOutputs))
	stages := make(map[string]struct{}, len(p.videoOutputs))
	for _, fields := range p.videoOutputs {
		stage := strings.TrimSpace(fields.stage.Value)
		if stage == "" {
			p.status, p.statusError = "Video stage names cannot be empty", true
			return
		}
		if _, exists := stages[stage]; exists {
			p.status, p.statusError = "Duplicate video stage: "+stage, true
			return
		}
		stages[stage] = struct{}{}
		settings.VideoOutputs = append(settings.VideoOutputs, config.VideoOutput{
			Stage: stage, DisplayID: selectedDropdownValue(fields.display), Fullscreen: fields.fullscreen.Checked,
			X: fields.x.Value, Y: fields.y.Value, Width: fields.width.Value, Height: fields.height.Value,
			ResolutionWidth: fields.resolutionW.Value, ResolutionHeight: fields.resolutionH.Value,
			Scaling: selectedDropdownValue(fields.scaling), IdleBehavior: selectedDropdownValue(fields.idle),
			TestGrid: fields.testGrid.Checked, SafeAreaPercent: fields.safeArea.Value, Layers: fields.layers.Value,
			ExpectedRefresh: fields.expectedRefresh.Value, AlwaysOnTop: fields.alwaysOnTop.Checked, LockedFullscreen: fields.lockedFullscreen.Checked,
			HideCursor: fields.hideCursor.Checked, DisplayConfirmed: fields.displayConfirmed.Checked,
		})
	}
	settings.RemoteTargets = make([]config.RemoteTarget, 0, len(p.targets))
	settings.RemoteSuccessPolicy = selectedDropdownValue(p.remoteSuccessPolicy)
	for _, target := range p.targets {
		settings.RemoteTargets = append(settings.RemoteTargets, config.RemoteTarget{
			Name: strings.TrimSpace(target.name.Value), Host: strings.TrimSpace(target.host.Value),
			OSCPort: target.oscPort.Value, ERCPort: target.ercPort.Value, HealthPort: target.healthPort.Value, AckPort: target.ackPort.Value,
		})
	}
	settings.Variables = map[string]string{}
	for _, variable := range p.variables {
		name := strings.TrimSpace(variable.name.Value)
		if name == "" {
			p.status, p.statusError = "Variable names cannot be empty", true
			return
		}
		if !config.ValidVariableName(name) {
			p.status, p.statusError = "Invalid variable name: "+name, true
			return
		}
		if name == "defaultPlayback" || name == "defaultMediaOutput" || name == "cueNumber" {
			p.status, p.statusError = name+" is a built-in variable", true
			return
		}
		if _, exists := settings.Variables[name]; exists {
			p.status, p.statusError = "Duplicate variable: "+name, true
			return
		}
		settings.Variables[name] = variable.value.Value
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
