package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/syspoe/cusus/internal/atomicfile"
)

type RemoteTarget struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	OSCPort    int    `json:"oscPort"`
	ERCPort    int    `json:"ercPort"`
	HealthPort int    `json:"healthPort,omitempty"`
	AckPort    int    `json:"ackPort,omitempty"`
}

type VideoOutput struct {
	Stage            string `json:"stage"`
	DisplayID        string `json:"displayId,omitempty"`
	Fullscreen       bool   `json:"fullscreen"`
	X                int    `json:"x,omitempty"`
	Y                int    `json:"y,omitempty"`
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	ResolutionWidth  int    `json:"resolutionWidth"`
	ResolutionHeight int    `json:"resolutionHeight"`
	Scaling          string `json:"scaling"`
	IdleBehavior     string `json:"idleBehavior"`
	TestGrid         bool   `json:"testGrid,omitempty"`
	SafeAreaPercent  int    `json:"safeAreaPercent,omitempty"`
	Layers           int    `json:"layers"`
	ExpectedRefresh  int    `json:"expectedRefresh,omitempty"`
	AlwaysOnTop      bool   `json:"alwaysOnTop,omitempty"`
	LockedFullscreen bool   `json:"lockedFullscreen,omitempty"`
	HideCursor       bool   `json:"hideCursor,omitempty"`
	DisplayConfirmed bool   `json:"displayConfirmed,omitempty"`
}

type WindowPlacement struct {
	X      int `json:"x,omitempty"`
	Y      int `json:"y,omitempty"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// TODO(macro): Split this persistence DTO into versioned domain settings
// (media, routing, timecode, redundancy, and operator UI), with one migration
// boundary preserving the on-disk JSON contract. The flat aggregate makes every
// new option fan out through defaults, normalization, UI binding, and consumers.
// TODO(macro): Decompose the Settings blob by domain — one struct owns ffmpeg,
// audio recovery, video stages, template variables, remote targets, cache
// quotas, operator window geometry, timecode, and redundancy secrets, so every
// subsystem snapshots/clones/hashes unrelated machine policy. Group nested
// sub-settings (Media, Audio, Outputs, Remote, Timecode, Redundancy, UI).
type Settings struct {
	FFmpegPath                string            `json:"ffmpegPath"`
	DefaultPlayback           string            `json:"defaultPlayback"`
	DefaultMediaOutput        string            `json:"defaultMediaOutput"`
	PlaybackAudioDevice       string            `json:"playbackAudioDevice,omitempty"`
	PlaybackAudioRecovery     string            `json:"playbackAudioRecovery,omitempty"`
	PlaybackBackupAudioDevice string            `json:"playbackBackupAudioDevice,omitempty"`
	PreviewAudioDevice        string            `json:"previewAudioDevice,omitempty"`
	PreviewAudioRecovery      string            `json:"previewAudioRecovery,omitempty"`
	PreviewBackupAudioDevice  string            `json:"previewBackupAudioDevice,omitempty"`
	VideoOutputs              []VideoOutput     `json:"videoOutputs,omitempty"`
	Variables                 map[string]string `json:"variables"`
	RemoteTargets             []RemoteTarget    `json:"remoteTargets"`
	RemoteSuccessPolicy       string            `json:"remoteSuccessPolicy,omitempty"`
	CacheQuotaGB              int               `json:"cacheQuotaGb,omitempty"`
	CacheReserveGB            int               `json:"cacheReserveGb,omitempty"`
	OperatorWindow            WindowPlacement   `json:"operatorWindow"`
	TimecodeSource            string            `json:"timecodeSource,omitempty"`
	TimecodePolicy            string            `json:"timecodePolicy,omitempty"`
	TimecodeListenAddress     string            `json:"timecodeListenAddress,omitempty"`
	TimecodeFrameRate         float64           `json:"timecodeFrameRate,omitempty"`
	RedundancyRole            string            `json:"redundancyRole,omitempty"`
	RedundancyNodeID          string            `json:"redundancyNodeId,omitempty"`
	RedundancyListenAddress   string            `json:"redundancyListenAddress,omitempty"`
	RedundancyPeerAddress     string            `json:"redundancyPeerAddress,omitempty"`
	RedundancySharedKey       string            `json:"redundancySharedKey,omitempty"`
	RedundancyInterlockPath   string            `json:"redundancyInterlockPath,omitempty"`
}

const (
	AudioRecoveryFailClosed    = "fail-closed"
	AudioRecoveryFollowDefault = "follow-default"
	AudioRecoveryNamedBackup   = "named-backup"
	RemoteSuccessAll           = "all"
	RemoteSuccessAny           = "any"
	TimecodeInternal           = "internal"
	TimecodeLTC                = "ltc"
	TimecodeMTC                = "mtc"
	TimecodeOSC                = "osc"
	TimecodeHold               = "hold"
	TimecodeChase              = "chase"
	TimecodeResync             = "resync"
	RedundancyOff              = "off"
	RedundancyPrimary          = "primary"
	RedundancyStandby          = "standby"
	minimumVideoWidth          = 320
	minimumVideoHeight         = 180
	defaultVideoWidth          = 960
	defaultVideoHeight         = 540
	defaultResolutionWidth     = 1920
	defaultResolutionHeight    = 1080
	minimumSafeAreaPercent     = 0
	maximumSafeAreaPercent     = 20
	minimumOutputLayers        = 1
	maximumOutputLayers        = 8
	minimumCacheQuotaGB        = 1
	maximumCacheQuotaGB        = 500
	minimumCacheReserveGB      = 1
	maximumCacheReserveGB      = 100
)

func Defaults() Settings {
	return Settings{
		FFmpegPath:              "ffmpeg",
		DefaultPlayback:         "1",
		DefaultMediaOutput:      "main",
		PlaybackAudioRecovery:   AudioRecoveryFailClosed,
		PreviewAudioRecovery:    AudioRecoveryFailClosed,
		VideoOutputs:            []VideoOutput{{Stage: "main", Fullscreen: true, Width: defaultVideoWidth, Height: defaultVideoHeight, ResolutionWidth: defaultResolutionWidth, ResolutionHeight: defaultResolutionHeight, Scaling: "contain", IdleBehavior: "black", Layers: minimumOutputLayers}},
		Variables:               map[string]string{},
		RemoteTargets:           []RemoteTarget{{Name: "Local console", Host: "127.0.0.1", OSCPort: 8000, ERCPort: 6553}},
		RemoteSuccessPolicy:     RemoteSuccessAll,
		CacheQuotaGB:            20,
		CacheReserveGB:          5,
		OperatorWindow:          WindowPlacement{X: 80, Y: 80, Width: 1300, Height: 720},
		TimecodeSource:          TimecodeInternal,
		TimecodePolicy:          TimecodeHold,
		TimecodeListenAddress:   "127.0.0.1:9001",
		TimecodeFrameRate:       30,
		RedundancyRole:          RedundancyOff,
		RedundancyNodeID:        defaultNodeID(),
		RedundancyListenAddress: "127.0.0.1:9012",
		RedundancyPeerAddress:   "127.0.0.1:9013",
	}
}

func defaultNodeID() string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "cusus-node"
	}
	return strings.TrimSpace(name)
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(dir, "CuSus", "settings.json"), nil
}

type Store struct {
	mu       sync.RWMutex
	path     string
	settings Settings
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	store := &Store{path: path, settings: Defaults()}
	if err := atomicfile.Recover(path); err != nil {
		return nil, fmt.Errorf("recover settings: %w", err)
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := store.saveLocked(store.settings); err != nil {
			return nil, err
		}
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	settings := Defaults()
	if err := json.Unmarshal(raw, &settings); err != nil {
		backupRaw, backupErr := os.ReadFile(atomicfile.BackupPath(path))
		backupSettings := Defaults()
		if backupErr != nil || json.Unmarshal(backupRaw, &backupSettings) != nil {
			return nil, fmt.Errorf("decode settings: %w", err)
		}
		corruptPath := fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixMilli())
		if renameErr := os.Rename(path, corruptPath); renameErr != nil {
			return nil, fmt.Errorf("preserve corrupt settings: %w", renameErr)
		}
		if renameErr := os.Rename(atomicfile.BackupPath(path), path); renameErr != nil {
			_ = os.Rename(corruptPath, path)
			return nil, fmt.Errorf("restore valid settings backup: %w", renameErr)
		}
		settings = backupSettings
	}
	store.settings = normalize(settings)
	return store, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) Snapshot() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.settings)
}

func (s *Store) Update(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name := range settings.Variables {
		if !ValidVariableName(name) {
			return fmt.Errorf("invalid variable name %q", name)
		}
		if name == "defaultPlayback" || name == "defaultMediaOutput" || name == "cueNumber" {
			return fmt.Errorf("%s is a built-in variable", name)
		}
	}
	candidate := normalize(settings)
	if err := s.saveLocked(candidate); err != nil {
		return err
	}
	s.settings = candidate
	return nil
}

func (s *Store) UpdateVideoOutputGeometry(stage string, x, y, width, height int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	candidate := clone(s.settings)
	for i := range candidate.VideoOutputs {
		output := &candidate.VideoOutputs[i]
		if output.Stage == stage {
			output.X, output.Y = x, y
			if width > 0 && height > 0 {
				output.Width, output.Height = width, height
			}
			if err := s.saveLocked(candidate); err != nil {
				return err
			}
			s.settings = candidate
			return nil
		}
	}
	return fmt.Errorf("video output stage %q is not configured", stage)
}

func (s *Store) saveLocked(settings Settings) error {
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := atomicfile.Write(s.path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("persist settings: %w", err)
	}
	return nil
}

func normalize(in Settings) Settings {
	if strings.TrimSpace(in.FFmpegPath) == "" {
		in.FFmpegPath = "ffmpeg"
	}
	if strings.TrimSpace(in.DefaultPlayback) == "" {
		in.DefaultPlayback = "1"
	}
	if strings.TrimSpace(in.DefaultMediaOutput) == "" {
		in.DefaultMediaOutput = "main"
	}
	in.PlaybackAudioDevice = strings.TrimSpace(in.PlaybackAudioDevice)
	in.PreviewAudioDevice = strings.TrimSpace(in.PreviewAudioDevice)
	in.PlaybackBackupAudioDevice = strings.TrimSpace(in.PlaybackBackupAudioDevice)
	in.PreviewBackupAudioDevice = strings.TrimSpace(in.PreviewBackupAudioDevice)
	in.PlaybackAudioRecovery = normalizeAudioRecovery(in.PlaybackAudioRecovery)
	in.PreviewAudioRecovery = normalizeAudioRecovery(in.PreviewAudioRecovery)
	seenStages := make(map[string]struct{}, len(in.VideoOutputs))
	outputs := make([]VideoOutput, 0, len(in.VideoOutputs)+1)
	for _, output := range in.VideoOutputs {
		output.Stage, output.DisplayID = strings.TrimSpace(output.Stage), strings.TrimSpace(output.DisplayID)
		if output.Stage == "" {
			continue
		}
		if _, exists := seenStages[output.Stage]; exists {
			continue
		}
		seenStages[output.Stage] = struct{}{}
		if output.Width < minimumVideoWidth {
			output.Width = defaultVideoWidth
		}
		if output.Height < minimumVideoHeight {
			output.Height = defaultVideoHeight
		}
		if output.ResolutionWidth < 1 {
			output.ResolutionWidth = defaultResolutionWidth
		}
		if output.ResolutionHeight < 1 {
			output.ResolutionHeight = defaultResolutionHeight
		}
		switch output.Scaling {
		case "contain", "cover", "stretch", "native":
		default:
			output.Scaling = "contain"
		}
		if output.IdleBehavior != "hold" {
			output.IdleBehavior = "black"
		}
		output.SafeAreaPercent = min(maximumSafeAreaPercent, max(minimumSafeAreaPercent, output.SafeAreaPercent))
		output.Layers = min(maximumOutputLayers, max(minimumOutputLayers, output.Layers))
		if output.ExpectedRefresh < 0 || output.ExpectedRefresh > 1000 {
			output.ExpectedRefresh = 0
		}
		if output.LockedFullscreen {
			output.Fullscreen = true
		}
		outputs = append(outputs, output)
	}
	if _, exists := seenStages[in.DefaultMediaOutput]; !exists {
		outputs = append(outputs, VideoOutput{Stage: in.DefaultMediaOutput, Fullscreen: true, Width: defaultVideoWidth, Height: defaultVideoHeight, ResolutionWidth: defaultResolutionWidth, ResolutionHeight: defaultResolutionHeight, Scaling: "contain", IdleBehavior: "black", Layers: minimumOutputLayers})
	}
	in.VideoOutputs = outputs
	if in.Variables == nil {
		in.Variables = map[string]string{}
	}
	for i := range in.RemoteTargets {
		target := &in.RemoteTargets[i]
		target.Name = strings.TrimSpace(target.Name)
		target.Host = strings.TrimSpace(target.Host)
		if target.Host == "" {
			target.Host = "127.0.0.1"
		}
		if target.OSCPort < 0 || target.OSCPort > 65535 {
			target.OSCPort = 0
		}
		if target.ERCPort < 0 || target.ERCPort > 65535 {
			target.ERCPort = 0
		}
		if target.HealthPort < 0 || target.HealthPort > 65535 {
			target.HealthPort = 0
		}
		if target.AckPort < 0 || target.AckPort > 65535 {
			target.AckPort = 0
		}
	}
	if in.RemoteSuccessPolicy != RemoteSuccessAny {
		in.RemoteSuccessPolicy = RemoteSuccessAll
	}
	in.CacheQuotaGB = min(maximumCacheQuotaGB, max(minimumCacheQuotaGB, in.CacheQuotaGB))
	in.CacheReserveGB = min(maximumCacheReserveGB, max(minimumCacheReserveGB, in.CacheReserveGB))
	if in.OperatorWindow.Width <= 0 && in.OperatorWindow.Height <= 0 {
		in.OperatorWindow = Defaults().OperatorWindow
	}
	in.OperatorWindow.Width = min(7680, max(480, in.OperatorWindow.Width))
	in.OperatorWindow.Height = min(4320, max(320, in.OperatorWindow.Height))
	switch in.TimecodeSource {
	case TimecodeLTC, TimecodeMTC, TimecodeOSC:
	default:
		in.TimecodeSource = TimecodeInternal
	}
	switch in.TimecodePolicy {
	case TimecodeChase, TimecodeResync:
	default:
		in.TimecodePolicy = TimecodeHold
	}
	if strings.TrimSpace(in.TimecodeListenAddress) == "" {
		in.TimecodeListenAddress = "127.0.0.1:9001"
	}
	if in.TimecodeFrameRate != 24 && in.TimecodeFrameRate != 25 && in.TimecodeFrameRate != 29.97 && in.TimecodeFrameRate != 30 {
		in.TimecodeFrameRate = 30
	}
	switch in.RedundancyRole {
	case RedundancyPrimary, RedundancyStandby:
	default:
		in.RedundancyRole = RedundancyOff
	}
	in.RedundancyNodeID = strings.TrimSpace(in.RedundancyNodeID)
	if in.RedundancyNodeID == "" {
		in.RedundancyNodeID = defaultNodeID()
	}
	in.RedundancyListenAddress = strings.TrimSpace(in.RedundancyListenAddress)
	if in.RedundancyListenAddress == "" {
		in.RedundancyListenAddress = "127.0.0.1:9012"
	}
	in.RedundancyPeerAddress = strings.TrimSpace(in.RedundancyPeerAddress)
	if in.RedundancyPeerAddress == "" {
		in.RedundancyPeerAddress = "127.0.0.1:9013"
	}
	in.RedundancySharedKey = strings.TrimSpace(in.RedundancySharedKey)
	in.RedundancyInterlockPath = strings.TrimSpace(in.RedundancyInterlockPath)
	return in
}

func normalizeAudioRecovery(policy string) string {
	policy = strings.TrimSpace(policy)
	switch policy {
	case AudioRecoveryFollowDefault, AudioRecoveryNamedBackup:
		return policy
	default:
		return AudioRecoveryFailClosed
	}
}

func AudioRoute(settings Settings, preview bool) (deviceID, policy, backupID string) {
	if preview {
		return settings.PreviewAudioDevice, normalizeAudioRecovery(settings.PreviewAudioRecovery), settings.PreviewBackupAudioDevice
	}
	return settings.PlaybackAudioDevice, normalizeAudioRecovery(settings.PlaybackAudioRecovery), settings.PlaybackBackupAudioDevice
}

func clone(in Settings) Settings {
	out := in
	out.Variables = make(map[string]string, len(in.Variables))
	for key, value := range in.Variables {
		out.Variables[key] = value
	}
	out.RemoteTargets = append([]RemoteTarget(nil), in.RemoteTargets...)
	out.VideoOutputs = append([]VideoOutput(nil), in.VideoOutputs...)
	return out
}

func VideoOutputFor(settings Settings, stage string) VideoOutput {
	for _, output := range settings.VideoOutputs {
		if output.Stage == stage {
			return output
		}
	}
	fallback := Defaults().VideoOutputs[0]
	fallback.Stage = stage
	return fallback
}

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

func ValidVariableName(name string) bool { return variableNamePattern.MatchString(name) }
