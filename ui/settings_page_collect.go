package ui

import (
	"fmt"
	"strings"

	"github.com/syspoe/cusus/config"
)

func (model *settingsDefaultsModel) collect(settings *config.Settings) error {
	settings.FFmpegPath = strings.TrimSpace(model.ffmpegPath.Value)
	settings.DefaultPlayback = strings.TrimSpace(model.defaultPlayback.Value)
	settings.DefaultMediaOutput = strings.TrimSpace(model.defaultMediaOutput.Value)
	settings.CacheQuotaGB, settings.CacheReserveGB = model.cacheQuotaGB.Value, model.cacheReserveGB.Value
	return nil
}

func (model *settingsTimecodeModel) collect(settings *config.Settings) error {
	settings.TimecodeSource = selectedDropdownValue(model.timecodeSource)
	settings.TimecodePolicy = selectedDropdownValue(model.timecodePolicy)
	settings.TimecodeListenAddress = strings.TrimSpace(model.timecodeListenAddress.Value)
	settings.TimecodeFrameRate = model.timecodeFrameRate.Value
	return nil
}

func (model *settingsRedundancyModel) collect(settings *config.Settings) error {
	settings.RedundancyRole = selectedDropdownValue(model.redundancyRole)
	settings.RedundancyNodeID = strings.TrimSpace(model.redundancyNodeID.Value)
	settings.RedundancyListenAddress = strings.TrimSpace(model.redundancyListenAddress.Value)
	settings.RedundancyPeerAddress = strings.TrimSpace(model.redundancyPeerAddress.Value)
	settings.RedundancySharedKey = strings.TrimSpace(model.redundancySharedKey.Value)
	settings.RedundancyInterlockPath = strings.TrimSpace(model.redundancyInterlockPath.Value)
	if settings.RedundancyRole == config.RedundancyOff {
		return nil
	}
	if settings.RedundancyNodeID == "" || settings.RedundancyListenAddress == "" || settings.RedundancyPeerAddress == "" || settings.RedundancyInterlockPath == "" {
		return fmt.Errorf("redundancy requires node ID, heartbeat addresses, and a shared interlock path")
	}
	if len(settings.RedundancySharedKey) < config.MinimumRedundancySharedKeyLength {
		return fmt.Errorf("redundancy shared key must contain at least %d characters", config.MinimumRedundancySharedKeyLength)
	}
	return nil
}

func (model *settingsAudioModel) collect(settings *config.Settings) error {
	settings.PlaybackAudioDevice = selectedDropdownValue(model.playbackAudioDevice)
	settings.PlaybackAudioRecovery = selectedDropdownValue(model.playbackAudioRecovery)
	settings.PlaybackBackupAudioDevice = selectedDropdownValue(model.playbackBackupAudioDevice)
	settings.PreviewAudioDevice = selectedDropdownValue(model.previewAudioDevice)
	settings.PreviewAudioRecovery = selectedDropdownValue(model.previewAudioRecovery)
	settings.PreviewBackupAudioDevice = selectedDropdownValue(model.previewBackupAudioDevice)
	if settings.PlaybackAudioRecovery == config.AudioRecoveryNamedBackup && settings.PlaybackBackupAudioDevice == "" {
		return fmt.Errorf("playback named-backup policy requires a backup device")
	}
	if settings.PreviewAudioRecovery == config.AudioRecoveryNamedBackup && settings.PreviewBackupAudioDevice == "" {
		return fmt.Errorf("preview named-backup policy requires a backup device")
	}
	return nil
}

func (model *settingsVideoModel) collect(settings *config.Settings) error {
	settings.VideoOutputs = make([]config.VideoOutput, 0, len(model.videoOutputs))
	stages := make(map[string]struct{}, len(model.videoOutputs))
	for _, fields := range model.videoOutputs {
		stage := strings.TrimSpace(fields.stage.Value)
		if stage == "" {
			return fmt.Errorf("video stage names cannot be empty")
		}
		if _, exists := stages[stage]; exists {
			return fmt.Errorf("duplicate video stage: %s", stage)
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
	return nil
}

func (model *settingsTargetsModel) collect(settings *config.Settings) error {
	settings.RemoteSuccessPolicy = selectedDropdownValue(model.remoteSuccessPolicy)
	settings.RemoteTargets = make([]config.RemoteTarget, 0, len(model.targets))
	for _, target := range model.targets {
		settings.RemoteTargets = append(settings.RemoteTargets, config.RemoteTarget{
			Name: strings.TrimSpace(target.name.Value), Host: strings.TrimSpace(target.host.Value),
			OSCPort: target.oscPort.Value, ERCPort: target.ercPort.Value, HealthPort: target.healthPort.Value, AckPort: target.ackPort.Value,
		})
	}
	return nil
}

func (model *settingsVariablesModel) collect(settings *config.Settings) error {
	settings.Variables = make(map[string]string, len(model.variables))
	for _, variable := range model.variables {
		name := strings.TrimSpace(variable.name.Value)
		if name == "" {
			return fmt.Errorf("variable names cannot be empty")
		}
		if !config.ValidVariableName(name) {
			return fmt.Errorf("invalid variable name: %s", name)
		}
		if name == "defaultPlayback" || name == "defaultMediaOutput" || name == "cueNumber" {
			return fmt.Errorf("%s is a built-in variable", name)
		}
		if _, exists := settings.Variables[name]; exists {
			return fmt.Errorf("duplicate variable: %s", name)
		}
		settings.Variables[name] = variable.value.Value
	}
	return nil
}
