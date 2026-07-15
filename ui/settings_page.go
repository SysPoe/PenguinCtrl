package ui

import (
	"sort"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/ui/input"
)

type remoteTargetFields struct {
	name       *input.Text
	host       *input.Text
	oscPort    *input.Integer
	ercPort    *input.Integer
	healthPort *input.Integer
	ackPort    *input.Integer
	remove     widget.Clickable
}

type variableFields struct {
	name   *input.Text
	value  *input.Text
	remove widget.Clickable
}

type AudioDevice struct {
	ID        string
	Name      string
	IsDefault bool
}

type VideoDisplay struct {
	ID      string
	Name    string
	Primary bool
}

type videoOutputFields struct {
	stage            *input.Text
	display          *input.Dropdown
	fullscreen       *input.Checkbox
	x                *input.Integer
	y                *input.Integer
	width            *input.Integer
	height           *input.Integer
	resolutionW      *input.Integer
	resolutionH      *input.Integer
	scaling          *input.Dropdown
	idle             *input.Dropdown
	testGrid         *input.Checkbox
	safeArea         *input.Integer
	layers           *input.Integer
	expectedRefresh  *input.Integer
	alwaysOnTop      *input.Checkbox
	lockedFullscreen *input.Checkbox
	hideCursor       *input.Checkbox
	displayConfirmed *input.Checkbox
	remove           widget.Clickable
}

// TODO(macro): Bind per-domain settings view models through the same typed
// validation boundary used by config.Store. This flat widget mirror plus the
// manual load/save mapping lets persistence, validation, and UI fields drift as
// the settings schema grows.
// TODO(macro): SettingsPage is a flat god-struct: every section's fields, device providers,
// action clickables, and host callbacks sit on one type with monolithic load/save. Split
// into section models (Defaults, Audio, VideoOutputs, Timecode, Redundancy, Targets,
// Variables) that each load/save their config slice, and keep SettingsPage as a section
// host + status chrome.
type SettingsPage struct {
	store                     *config.Store
	initialized               bool
	list                      layout.List
	ffmpegPath                *input.Text
	defaultPlayback           *input.Text
	defaultMediaOutput        *input.Text
	cacheQuotaGB              *input.Integer
	cacheReserveGB            *input.Integer
	timecodeSource            *input.Dropdown
	timecodePolicy            *input.Dropdown
	timecodeListenAddress     *input.Text
	timecodeFrameRate         *input.Float
	redundancyRole            *input.Dropdown
	redundancyNodeID          *input.Text
	redundancyListenAddress   *input.Text
	redundancyPeerAddress     *input.Text
	redundancySharedKey       *input.Text
	redundancyInterlockPath   *input.Text
	remoteSuccessPolicy       *input.Dropdown
	playbackAudioDevice       *input.Dropdown
	playbackAudioRecovery     *input.Dropdown
	playbackBackupAudioDevice *input.Dropdown
	previewAudioDevice        *input.Dropdown
	previewAudioRecovery      *input.Dropdown
	previewBackupAudioDevice  *input.Dropdown
	audioDevices              []AudioDevice
	audioDeviceProvider       func() ([]AudioDevice, error)
	videoDisplays             []VideoDisplay
	videoDisplayProvider      func() ([]VideoDisplay, error)
	videoOutputs              []*videoOutputFields
	targets                   []*remoteTargetFields
	variables                 []*variableFields
	addTarget                 widget.Clickable
	addVariable               widget.Clickable
	save                      widget.Clickable
	reload                    widget.Clickable
	reopenOutputs             widget.Clickable
	supportBundle             widget.Clickable
	refreshAudioDevices       widget.Clickable
	refreshDisplays           widget.Clickable
	addVideoOutput            widget.Clickable
	takeAuthority             widget.Clickable
	releaseAuthority          widget.Clickable
	status                    string
	statusError               bool
	onSaved                   func()
	onReopenOutputs           func()
	onSupportBundle           func() (string, error)
	redundancyStatus          func() string
	onTakeAuthority           func() error
	onReleaseAuthority        func() error
}

func (p *SettingsPage) SetOnSaved(callback func()) { p.onSaved = callback }

func (p *SettingsPage) SetOnReopenOutputs(callback func()) { p.onReopenOutputs = callback }

func (p *SettingsPage) SetOnSupportBundle(callback func() (string, error)) {
	p.onSupportBundle = callback
}

func (p *SettingsPage) SetRedundancyControl(status func() string, takeAuthority, releaseAuthority func() error) {
	p.redundancyStatus = status
	p.onTakeAuthority = takeAuthority
	p.onReleaseAuthority = releaseAuthority
}

func (p *SettingsPage) SetAudioDeviceProvider(provider func() ([]AudioDevice, error)) {
	p.audioDeviceProvider = provider
	p.refreshAudioDeviceList()
}

func (p *SettingsPage) SetVideoDisplayProvider(provider func() ([]VideoDisplay, error)) {
	p.videoDisplayProvider = provider
	p.refreshVideoDisplayList()
}

// ShowAudioDevices refreshes and scrolls directly to the audio routing controls.
func (p *SettingsPage) ShowAudioDevices() {
	p.refreshAudioDeviceList()
	// TODO(micro): magic section index 2 for audio; name a const matching Layout section order.
	p.list.Position.First = 2
	p.list.Position.Offset = 0
}

func NewSettingsPage(store *config.Store) *SettingsPage {
	page := &SettingsPage{store: store, list: layout.List{Axis: layout.Vertical}}
	page.load()
	return page
}

func (p *SettingsPage) load() {
	settings := p.store.Snapshot()
	p.ffmpegPath = input.NewText("FFmpeg executable", settings.FFmpegPath)
	p.defaultPlayback = input.NewText("Default playback", settings.DefaultPlayback)
	p.defaultMediaOutput = input.NewText("Default media output", settings.DefaultMediaOutput)
	p.cacheQuotaGB = input.NewInteger("Cache quota GB", settings.CacheQuotaGB)
	p.cacheReserveGB = input.NewInteger("Reserved free GB", settings.CacheReserveGB)
	p.timecodeSource = enumDropdown([]input.DropdownItem{{Label: "Internal / manual", Value: config.TimecodeInternal}, {Label: "LTC adapter", Value: config.TimecodeLTC}, {Label: "MTC quarter-frame", Value: config.TimecodeMTC}, {Label: "OSC input", Value: config.TimecodeOSC}}, settings.TimecodeSource)
	p.timecodePolicy = enumDropdown([]input.DropdownItem{{Label: "Hold for operator", Value: config.TimecodeHold}, {Label: "Chase external source", Value: config.TimecodeChase}, {Label: "Immediate resync", Value: config.TimecodeResync}}, settings.TimecodePolicy)
	p.timecodeListenAddress = input.NewText("OSC listen address", settings.TimecodeListenAddress)
	p.timecodeFrameRate = input.NewFloat("Frame rate", settings.TimecodeFrameRate)
	p.redundancyRole = enumDropdown([]input.DropdownItem{{Label: "Disabled", Value: config.RedundancyOff}, {Label: "Primary", Value: config.RedundancyPrimary}, {Label: "Warm standby", Value: config.RedundancyStandby}}, settings.RedundancyRole)
	p.redundancyNodeID = input.NewText("Unique node ID", settings.RedundancyNodeID)
	p.redundancyListenAddress = input.NewText("Heartbeat listen address", settings.RedundancyListenAddress)
	p.redundancyPeerAddress = input.NewText("Peer heartbeat address", settings.RedundancyPeerAddress)
	p.redundancySharedKey = input.NewText("Shared authentication key", settings.RedundancySharedKey)
	p.redundancyInterlockPath = input.NewText("Shared interlock path", settings.RedundancyInterlockPath)
	p.remoteSuccessPolicy = enumDropdown([]input.DropdownItem{{Label: "Require every target", Value: config.RemoteSuccessAll}, {Label: "Any redundant target", Value: config.RemoteSuccessAny}}, settings.RemoteSuccessPolicy)
	p.playbackAudioDevice = newAudioDeviceDropdown(p.audioDevices, settings.PlaybackAudioDevice)
	p.playbackAudioRecovery = newAudioRecoveryDropdown(settings.PlaybackAudioRecovery)
	p.playbackBackupAudioDevice = newAudioDeviceDropdown(p.audioDevices, settings.PlaybackBackupAudioDevice)
	p.previewAudioDevice = newAudioDeviceDropdown(p.audioDevices, settings.PreviewAudioDevice)
	p.previewAudioRecovery = newAudioRecoveryDropdown(settings.PreviewAudioRecovery)
	p.previewBackupAudioDevice = newAudioDeviceDropdown(p.audioDevices, settings.PreviewBackupAudioDevice)
	p.videoOutputs = make([]*videoOutputFields, 0, len(settings.VideoOutputs))
	for _, output := range settings.VideoOutputs {
		p.videoOutputs = append(p.videoOutputs, newVideoOutputFields(output, p.videoDisplays))
	}
	p.targets = make([]*remoteTargetFields, 0, len(settings.RemoteTargets))
	for _, target := range settings.RemoteTargets {
		p.targets = append(p.targets, newRemoteTargetFields(target))
	}
	p.variables = make([]*variableFields, 0, len(settings.Variables))
	keys := make([]string, 0, len(settings.Variables))
	for key := range settings.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		p.variables = append(p.variables, newVariableFields(key, settings.Variables[key]))
	}
	p.initialized = true
	p.status = "Loaded"
	p.statusError = false
}

func newRemoteTargetFields(target config.RemoteTarget) *remoteTargetFields {
	return &remoteTargetFields{
		name: input.NewText("Name", target.Name), host: input.NewText("Host", target.Host),
		oscPort: input.NewOptionalInteger("OSC port", target.OSCPort), ercPort: input.NewOptionalInteger("ERC port", target.ERCPort),
		healthPort: input.NewOptionalInteger("Health TCP", target.HealthPort), ackPort: input.NewOptionalInteger("Ack relay TCP", target.AckPort),
	}
}

func newVariableFields(name, value string) *variableFields {
	return &variableFields{name: input.NewText("Variable name", name), value: input.NewText("Value", value)}
}
