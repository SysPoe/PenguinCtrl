package ui

import (
	"fmt"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/palette"
	"github.com/syspoe/cusus/ui/input"
)

func (p *SettingsPage) header(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.H5(th, "Settings")
					label.Color = palette.Opaque(th.Fg)
					return layoutStableText(gtx, label.Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := stableBody2(th, p.status)
					if p.statusError {
						label.Color = palette.Danger
					}
					return layoutStableText(gtx, label.Layout)
				}),
			)
		}),
		settingsHeaderButton(th, &p.reopenOutputs, "Reopen output windows", th.ContrastBg),
		settingsHeaderButton(th, &p.supportBundle, "Create support bundle", th.ContrastBg),
		settingsHeaderButton(th, &p.reload, "Reload", th.ContrastBg),
		settingsHeaderButton(th, &p.save, "Save", palette.Primary),
	)
}

func settingsHeaderButton(th *material.Theme, clickable *widget.Clickable, label string, background color.NRGBA) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layoutButton(th, gtx, clickable, label, background)
		})
	})
}

func (p *SettingsPage) defaultsSection(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return settingsSection(th, gtx, "Defaults", []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "Default playback", p.defaultPlayback.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "Default media output", p.defaultMediaOutput.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "FFmpeg executable", p.ffmpegPath.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return pairedSettingsFields(th, gtx, "Cache quota / free reserve (GB)", p.cacheQuotaGB.Layout, p.cacheReserveGB.Layout)
		},
	})
}

func (p *SettingsPage) audioSection(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return settingsSection(th, gtx, "Audio devices", []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "Playback", p.playbackAudioDevice.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "Playback loss policy", p.playbackAudioRecovery.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "Playback backup", p.playbackBackupAudioDevice.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "Preview", p.previewAudioDevice.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "Preview loss policy", p.previewAudioRecovery.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return settingsField(th, gtx, "Preview backup", p.previewBackupAudioDevice.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layoutButton(th, gtx, &p.refreshAudioDevices, "Refresh audio devices", th.ContrastBg)
		},
	})
}

func (p *SettingsPage) timecodeSection(th *material.Theme, gtx layout.Context) layout.Dimensions {
	return settingsSection(th, gtx, "External timecode", []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return pairedSettingsFields(th, gtx, "Source / discontinuity policy", p.timecodeSource.Layout, p.timecodePolicy.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return pairedSettingsFields(th, gtx, "OSC address / frame rate", p.timecodeListenAddress.Layout, p.timecodeFrameRate.Layout)
		},
	})
}

func (p *SettingsPage) redundancySection(th *material.Theme, gtx layout.Context) layout.Dimensions {
	status := "Save redundancy settings to start heartbeat and interlock supervision"
	if p.redundancyStatus != nil {
		status = p.redundancyStatus()
	}
	return settingsSection(th, gtx, "Warm-spare redundancy", []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutStableText(gtx, stableBody1(th, status).Layout)
			})
		},
		func(gtx layout.Context) layout.Dimensions {
			return pairedSettingsFields(th, gtx, "Role / unique node ID", p.redundancyRole.Layout, p.redundancyNodeID.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return pairedSettingsFields(th, gtx, "Heartbeat listen / peer", p.redundancyListenAddress.Layout, p.redundancyPeerAddress.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return pairedSettingsFields(th, gtx, "Shared key / interlock path", p.redundancySharedKey.Layout, p.redundancyInterlockPath.Layout)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutButton(th, gtx, &p.takeAuthority, "Take command authority", palette.Primary)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layoutButton(th, gtx, &p.releaseAuthority, "Release for handoff", th.ContrastBg)
					})
				}),
			)
		},
	})
}

func (p *SettingsPage) refreshAudioDeviceList() {
	if p.audioDeviceProvider == nil {
		return
	}
	devices, err := p.audioDeviceProvider()
	if err != nil {
		p.status, p.statusError = err.Error(), true
		return
	}
	p.audioDevices = devices
	settings := p.store.Snapshot()
	playbackID, previewID := settings.PlaybackAudioDevice, settings.PreviewAudioDevice
	playbackBackupID, previewBackupID := settings.PlaybackBackupAudioDevice, settings.PreviewBackupAudioDevice
	if p.playbackAudioDevice != nil {
		playbackID = selectedDropdownValue(p.playbackAudioDevice)
	}
	if p.previewAudioDevice != nil {
		previewID = selectedDropdownValue(p.previewAudioDevice)
	}
	if p.playbackBackupAudioDevice != nil {
		playbackBackupID = selectedDropdownValue(p.playbackBackupAudioDevice)
	}
	if p.previewBackupAudioDevice != nil {
		previewBackupID = selectedDropdownValue(p.previewBackupAudioDevice)
	}
	p.playbackAudioDevice = newAudioDeviceDropdown(devices, playbackID)
	p.previewAudioDevice = newAudioDeviceDropdown(devices, previewID)
	p.playbackBackupAudioDevice = newAudioDeviceDropdown(devices, playbackBackupID)
	p.previewBackupAudioDevice = newAudioDeviceDropdown(devices, previewBackupID)
	p.status, p.statusError = fmt.Sprintf("Found %d audio device(s)", len(devices)), false
}

func newAudioRecoveryDropdown(policy string) *input.Dropdown {
	return enumDropdown([]input.DropdownItem{
		{Label: "Fail closed", Value: config.AudioRecoveryFailClosed},
		{Label: "Follow Windows default", Value: config.AudioRecoveryFollowDefault},
		{Label: "Switch to named backup", Value: config.AudioRecoveryNamedBackup},
	}, policy)
}

func newAudioDeviceDropdown(devices []AudioDevice, selectedID string) *input.Dropdown {
	items := []input.DropdownItem{{Label: "Windows default routing", Value: ""}}
	selected := 0
	found := selectedID == ""
	for _, device := range devices {
		label := device.Name
		if device.IsDefault {
			label += " (default)"
		}
		items = append(items, input.DropdownItem{Label: label, Value: device.ID})
		if device.ID == selectedID {
			selected, found = len(items)-1, true
		}
	}
	if !found {
		items = append(items, input.DropdownItem{Label: "Unavailable device", Value: selectedID})
		selected = len(items) - 1
	}
	return input.NewDropdown(items, selected)
}

func newVideoOutputFields(output config.VideoOutput, displays []VideoDisplay) *videoOutputFields {
	return &videoOutputFields{
		stage:      input.NewText("Stage name", output.Stage),
		display:    newVideoDisplayDropdown(displays, output.DisplayID),
		fullscreen: input.NewCheckbox("Fullscreen on launch", output.Fullscreen),
		x:          input.NewInteger("X", output.X), y: input.NewInteger("Y", output.Y),
		width: input.NewInteger("Window width", output.Width), height: input.NewInteger("Window height", output.Height),
		resolutionW: input.NewInteger("Output width", output.ResolutionWidth), resolutionH: input.NewInteger("Output height", output.ResolutionHeight),
		scaling:          enumDropdown([]input.DropdownItem{{Label: "Contain (letterbox)", Value: "contain"}, {Label: "Cover (crop)", Value: "cover"}, {Label: "Stretch", Value: "stretch"}, {Label: "Native pixels", Value: "native"}}, output.Scaling),
		idle:             enumDropdown([]input.DropdownItem{{Label: "Black", Value: "black"}, {Label: "Hold last frame", Value: "hold"}}, output.IdleBehavior),
		testGrid:         input.NewCheckbox("Overlay test grid", output.TestGrid),
		safeArea:         input.NewInteger("Safe area %", output.SafeAreaPercent),
		layers:           input.NewInteger("Maximum layers", output.Layers),
		expectedRefresh:  input.NewOptionalInteger("Expected refresh Hz", output.ExpectedRefresh),
		alwaysOnTop:      input.NewCheckbox("Keep output always on top", output.AlwaysOnTop),
		lockedFullscreen: input.NewCheckbox("Lock fullscreen mode", output.LockedFullscreen),
		hideCursor:       input.NewCheckbox("Hide cursor over output", output.HideCursor),
		displayConfirmed: input.NewCheckbox("Operator confirmed display mapping", output.DisplayConfirmed),
	}
}

func enumDropdown(items []input.DropdownItem, value string) *input.Dropdown {
	selected := 0
	for i, item := range items {
		if item.Value == value {
			selected = i
			break
		}
	}
	return input.NewDropdown(items, selected)
}

func newVideoDisplayDropdown(displays []VideoDisplay, selectedID string) *input.Dropdown {
	items := []input.DropdownItem{{Label: "Primary display (automatic)", Value: ""}}
	selected, found := 0, selectedID == ""
	for _, display := range displays {
		label := display.Name
		if display.Primary {
			label += " (primary)"
		}
		items = append(items, input.DropdownItem{Label: label, Value: display.ID})
		if display.ID == selectedID {
			selected, found = len(items)-1, true
		}
	}
	if !found {
		items = append(items, input.DropdownItem{Label: "Disconnected display", Value: selectedID})
		selected = len(items) - 1
	}
	return input.NewDropdown(items, selected)
}
