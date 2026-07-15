package media

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/syspoe/cusus/config"
)

const (
	audioSampleRate = 48000
	audioChannels   = 2
	audioRingBytes  = audioSampleRate * audioChannels * 2 * 2 // two seconds of S16 stereo
	audioPrebuffer  = 4096
)

// AudioDevice is a selectable hardware playback device. An empty ID means the
// operating system's current default device.
type AudioDevice struct {
	ID        string
	Name      string
	IsDefault bool
}

type AudioMixerMetrics struct {
	EndpointID     string
	ActiveSources  int
	Recovering     bool
	Failed         bool
	RecoveryCount  uint64
	LastCallback   time.Time
	MaxCallback    time.Duration
	TotalUnderruns uint64
}

// TODO(macro): AudioSystem owns malgo context, per-endpoint mixers, device
// enumeration, and prepared-player factory, while endpointMixer type starts here
// and devicePlayer is declared in audio_mixer.go with methods in audio_player.go.
// Re-cut files so system / mixer / source each have a single home, and keep the
// realtime callback types free of FFmpeg recovery hooks (see devicePlayer.recovery).
type AudioSystem struct {
	settings *config.Store
	context  *malgo.AllocatedContext
	mu       sync.Mutex
	mixers   map[string]*endpointMixer
	closed   bool
}

func NewAudioSystem(settings *config.Store) (*AudioSystem, error) {
	context, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initialize audio system: %w", err)
	}
	return &AudioSystem{settings: settings, context: context, mixers: map[string]*endpointMixer{}}, nil
}

func (a *AudioSystem) Devices() ([]AudioDevice, error) {
	if a == nil || a.context == nil {
		// TODO(micro): Use errors.New for this static message instead of fmt.Errorf.
		return nil, fmt.Errorf("audio system is unavailable")
	}
	devices, err := a.context.Devices(malgo.Playback)
	if err != nil {
		return nil, fmt.Errorf("list audio devices: %w", err)
	}
	result := make([]AudioDevice, 0, len(devices))
	for i := range devices {
		name := strings.TrimSpace(devices[i].Name())
		if name == "" {
			name = "Unnamed audio device"
		}
		result = append(result, AudioDevice{ID: devices[i].ID.String(), Name: name, IsDefault: devices[i].IsDefault != 0})
	}
	return result, nil
}

// TODO(micro): NewPlayer only wraps NewPreparedPlayer+Start and is unused outside tests; keep one public construction path or move the convenience wrapper next to tests.
func (a *AudioSystem) NewPlayer(reader io.Reader, preview bool) (*devicePlayer, error) {
	player, err := a.NewPreparedPlayer(reader, preview)
	if err != nil {
		return nil, err
	}
	if err := player.Start(); err != nil {
		_ = player.Close()
		return nil, err
	}
	return player, nil
}

// NewPreparedPlayer opens a device without starting its callback so audio and
// video can be buffered before the shared clock begins.
func (a *AudioSystem) NewPreparedPlayer(reader io.Reader, preview bool) (*devicePlayer, error) {
	if a == nil || a.context == nil {
		// TODO(micro): Use errors.New for this static message instead of fmt.Errorf.
		return nil, fmt.Errorf("audio output is unavailable")
	}
	deviceID, recoveryPolicy, backupID := config.AudioRoute(a.settings.Snapshot(), preview)
	return a.newPreparedPlayer(reader, deviceID, recoveryPolicy, backupID)
}

func (a *AudioSystem) newPreparedPlayer(reader io.Reader, deviceID, recoveryPolicy, backupID string) (*devicePlayer, error) {
	mixer, err := a.mixer(deviceID)
	if err != nil {
		return nil, err
	}

	player := &devicePlayer{
		reader: reader, ring: newPCMRing(audioRingBytes), done: make(chan struct{}),
		ready: make(chan struct{}), stopped: make(chan struct{}), mixer: mixer,
		recoveryPolicy: recoveryPolicy, backupDeviceID: backupID,
	}
	player.volume.Store(1)
	go player.fillRing()
	return player, nil
}

func (a *AudioSystem) mixer(deviceID string) (*endpointMixer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.context == nil {
		return nil, errors.New("audio system is closed")
	}
	if mixer := a.mixers[deviceID]; mixer != nil {
		return mixer, nil
	}
	mixer := &endpointMixer{system: a, deviceID: deviceID}
	mixer.sources.Store([]*devicePlayer{})
	if err := mixer.openDeviceLocked(); err != nil {
		return nil, err
	}
	a.mixers[deviceID] = mixer
	return mixer, nil
}

func (a *AudioSystem) deviceConfig(deviceID string) (malgo.DeviceConfig, error) {
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = audioChannels
	deviceConfig.SampleRate = audioSampleRate
	if deviceID == "" {
		return deviceConfig, nil
	}
	devices, err := a.context.Devices(malgo.Playback)
	if err != nil {
		return deviceConfig, fmt.Errorf("list audio devices: %w", err)
	}
	for i := range devices {
		if devices[i].ID.String() == deviceID {
			deviceConfig.Playback.DeviceID = devices[i].ID.Pointer()
			return deviceConfig, nil
		}
	}
	return deviceConfig, errors.New("selected audio device is unavailable")
}

func (a *AudioSystem) Close() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	// TODO(micro): Close and Metrics both snapshot mixers under a.mu; extract a lockedMixers() helper.
	mixers := make([]*endpointMixer, 0, len(a.mixers))
	for _, mixer := range a.mixers {
		mixers = append(mixers, mixer)
	}
	a.mixers = nil
	a.mu.Unlock()
	for _, mixer := range mixers {
		mixer.close()
	}
	if a.context != nil {
		_ = a.context.Uninit()
		a.context.Free()
		a.context = nil
	}
}

func (a *AudioSystem) Metrics() []AudioMixerMetrics {
	a.mu.Lock()
	// TODO(micro): same mixer snapshot as Close(); call shared lockedMixers() helper.
	mixers := make([]*endpointMixer, 0, len(a.mixers))
	for _, mixer := range a.mixers {
		mixers = append(mixers, mixer)
	}
	a.mu.Unlock()
	result := make([]AudioMixerMetrics, 0, len(mixers))
	for _, mixer := range mixers {
		sources := mixer.sources.Load().([]*devicePlayer)
		metrics := AudioMixerMetrics{
			EndpointID: mixer.deviceID, ActiveSources: len(sources), Recovering: mixer.recovering.Load(),
			Failed: mixer.failed.Load(), RecoveryCount: mixer.recoveries.Load(),
			MaxCallback: time.Duration(mixer.callbackMax.Load()),
		}
		if at := mixer.callbackAt.Load(); at > 0 {
			metrics.LastCallback = time.Unix(0, at)
		}
		for _, source := range sources {
			metrics.TotalUnderruns += source.Underruns()
		}
		result = append(result, metrics)
	}
	return result
}

type endpointMixer struct {
	system      *AudioSystem
	deviceID    string
	mu          sync.Mutex
	device      *malgo.Device
	sources     atomic.Value // immutable []*devicePlayer, read without locks by callback
	started     bool
	closed      bool
	recovering  atomic.Bool
	failed      atomic.Bool
	recoveries  atomic.Uint64
	callbackAt  atomic.Int64
	callbackMax atomic.Int64
}
