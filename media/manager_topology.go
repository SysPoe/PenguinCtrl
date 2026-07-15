package media

import (
	"context"
	"sync"
	"time"

	"github.com/syspoe/cusus/config"
)

const devicePollInterval = 2 * time.Second

// deviceTopology owns hardware inventory workers and their cached operator
// status. It observes mediaRuntime but never resets or closes it.
type deviceTopology struct {
	settings *config.Store
	runtime  *mediaRuntime
	ctx      context.Context
	cancel   context.CancelFunc
	workers  sync.WaitGroup

	audioMu           sync.Mutex
	audioDeviceStatus string
	audioDevices      []AudioDevice
	audioDevicesErr   error
	audioRefresh      chan struct{}

	displaysMu       sync.RWMutex
	displays         []VideoDisplay
	displaysErr      error
	displayRefresh   chan struct{}
	displaySignature string
	displayStatusMu  sync.Mutex
	lastDisplayCheck time.Time
	videoStatus      string

	onDisplaysChanged func()
}

func newDeviceTopology(settings *config.Store, runtime *mediaRuntime) *deviceTopology {
	ctx, cancel := context.WithCancel(context.Background())
	return &deviceTopology{
		settings: settings, runtime: runtime, ctx: ctx, cancel: cancel,
		audioRefresh: make(chan struct{}, 1), displayRefresh: make(chan struct{}, 1),
		audioDeviceStatus: "Checking audio output devices…",
	}
}

func (topology *deviceTopology) start() {
	topology.refreshDisplays(true)
	topology.workers.Add(2)
	go func() { defer topology.workers.Done(); topology.monitorDisplays() }()
	go func() { defer topology.workers.Done(); topology.monitorAudioDevices() }()
}

func (topology *deviceTopology) close() {
	topology.cancel()
	topology.workers.Wait()
}

func (topology *deviceTopology) audioDevicesSnapshot() ([]AudioDevice, error) {
	topology.audioMu.Lock()
	defer topology.audioMu.Unlock()
	return append([]AudioDevice(nil), topology.audioDevices...), topology.audioDevicesErr
}

func (topology *deviceTopology) audioWarning() string {
	topology.audioMu.Lock()
	defer topology.audioMu.Unlock()
	return topology.audioDeviceStatus
}

func (topology *deviceTopology) refreshAudioStatus() {
	select {
	case topology.audioRefresh <- struct{}{}:
	default:
	}
}

func (topology *deviceTopology) monitorAudioDevices() {
	ticker := time.NewTicker(devicePollInterval)
	defer ticker.Stop()
	for {
		topology.refreshAudioDevices()
		select {
		case <-topology.ctx.Done():
			return
		case <-topology.audioRefresh:
		case <-ticker.C:
		}
	}
}

func (topology *deviceTopology) refreshAudioDevices() {
	devices, metrics, err := topology.runtime.audioSnapshot()
	status := audioDeviceWarning(topology.settings.Snapshot(), devices, err)
	if status == "" {
		for _, mixer := range metrics {
			if mixer.Failed {
				status = "An audio endpoint could not be recovered. Active cues on that route are offline."
				break
			}
			if mixer.Recovering {
				status = "An audio endpoint stopped unexpectedly. CuSus is reconnecting with bounded retry."
				break
			}
		}
	}
	topology.audioMu.Lock()
	topology.audioDevices, topology.audioDevicesErr, topology.audioDeviceStatus = devices, err, status
	topology.audioMu.Unlock()
}

func audioDeviceWarning(settings config.Settings, devices []AudioDevice, err error) string {
	if err != nil {
		return "Audio device detection failed: " + err.Error()
	}
	if len(devices) == 0 {
		return "No Windows audio output device is available. Playback and preview audio are offline."
	}
	available := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		available[device.ID] = struct{}{}
	}
	_, playbackAvailable := available[settings.PlaybackAudioDevice]
	_, previewAvailable := available[settings.PreviewAudioDevice]
	playbackMissing := settings.PlaybackAudioDevice != "" && !playbackAvailable
	previewMissing := settings.PreviewAudioDevice != "" && !previewAvailable
	switch {
	case playbackMissing && previewMissing && settings.PlaybackAudioDevice == settings.PreviewAudioDevice:
		return "The selected playback and preview audio device is disconnected."
	case playbackMissing && previewMissing:
		return "The selected playback and preview audio devices are disconnected."
	case playbackMissing:
		return "The selected playback audio device is disconnected."
	case previewMissing:
		return "The selected preview audio device is disconnected."
	default:
		return ""
	}
}
