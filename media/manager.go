package media

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/widget"
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
)

type Manager struct {
	engine            *playback.Engine
	settings          *config.Store
	mu                sync.Mutex
	backendMu         sync.RWMutex
	resetMu           sync.Mutex
	windows           map[string]*outputWindow
	desired           map[string]struct{}
	closed            bool
	ctx               context.Context
	cancel            context.CancelFunc
	workers           sync.WaitGroup
	audio             *AudioSystem
	decoder           *FFmpegBackend
	audioStatusMu     sync.Mutex
	lastAudioCheck    time.Time
	audioDeviceStatus string
	audioDevices      []AudioDevice
	audioDevicesErr   error
	audioRefresh      chan struct{}
	displaysMu        sync.RWMutex
	displays          []VideoDisplay
	displaysErr       error
	displayRefresh    chan struct{}
	displaySignature  string
	displayStatusMu   sync.Mutex
	lastDisplayCheck  time.Time
	videoOutputStatus string
}

func NewManager(engine *playback.Engine, settings *config.Store) *Manager {
	audioSystem, err := NewAudioSystem(settings)
	if err != nil {
		log.Printf("initialize audio output: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{engine: engine, settings: settings, windows: map[string]*outputWindow{}, desired: map[string]struct{}{}, audio: audioSystem, ctx: ctx, cancel: cancel, audioRefresh: make(chan struct{}, 1), displayRefresh: make(chan struct{}, 1), audioDeviceStatus: "Checking audio output devices…"}
	manager.decoder = NewFFmpegBackend(settings, audioSystem)
	manager.refreshDisplays(true)
	manager.workers.Add(2)
	go func() { defer manager.workers.Done(); manager.monitorDisplays() }()
	go func() { defer manager.workers.Done(); manager.monitorAudioDevices() }()
	return manager
}

func (m *Manager) Prewarm(instances []playback.Instance) {
	requests := make([]PlaybackRequest, 0, len(instances))
	for _, instance := range instances {
		requests = append(requests, PlaybackRequest{
			Instance: instance,
			Position: time.Duration(max(int64(0), instance.ClipStartMs)) * time.Millisecond,
		})
	}
	m.backendMu.RLock()
	defer m.backendMu.RUnlock()
	if m.decoder != nil {
		m.decoder.Prewarm(requests)
	}
}

func (m *Manager) AudioDevices() ([]AudioDevice, error) {
	m.audioStatusMu.Lock()
	defer m.audioStatusMu.Unlock()
	return append([]AudioDevice(nil), m.audioDevices...), m.audioDevicesErr
}

func (m *Manager) AudioMixerMetrics() []AudioMixerMetrics {
	m.backendMu.RLock()
	defer m.backendMu.RUnlock()
	if m.audio == nil {
		return nil
	}
	return m.audio.Metrics()
}

// AudioDeviceWarning returns a cached warning for selected devices that are no
// longer present. Empty device IDs intentionally follow Windows' default route
// and therefore do not depend on one particular endpoint remaining connected.
func (m *Manager) AudioDeviceWarning() string {
	m.audioStatusMu.Lock()
	defer m.audioStatusMu.Unlock()
	return m.audioDeviceStatus
}

func (m *Manager) RefreshAudioDeviceStatus() {
	select {
	case m.audioRefresh <- struct{}{}:
	default:
	}
}

func (m *Manager) monitorAudioDevices() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		m.refreshAudioDevices()
		select {
		case <-m.ctx.Done():
			return
		case <-m.audioRefresh:
		case <-ticker.C:
		}
	}
}

func (m *Manager) refreshAudioDevices() {
	var devices []AudioDevice
	var metrics []AudioMixerMetrics
	var err error
	m.backendMu.RLock()
	if m.audio == nil {
		err = fmt.Errorf("audio output is unavailable")
	} else {
		devices, err = m.audio.Devices()
		metrics = m.audio.Metrics()
	}
	m.backendMu.RUnlock()
	status := audioDeviceWarning(m.settings.Snapshot(), devices, err)
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
	m.audioStatusMu.Lock()
	m.audioDevices, m.audioDevicesErr, m.audioDeviceStatus, m.lastAudioCheck = devices, err, status, time.Now()
	m.audioStatusMu.Unlock()
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

func (m *Manager) EnsureOutputs(outputIDs []string) {
	outputIDs = m.outputIDsWithConfiguredStages(outputIDs)
	m.mu.Lock()
	for _, outputID := range outputIDs {
		m.desired[outputID] = struct{}{}
	}
	m.mu.Unlock()
	for _, outputID := range outputIDs {
		m.ensureOutput(outputID)
	}
}

func (m *Manager) SyncOutputs(outputIDs []string) {
	outputIDs = m.outputIDsWithConfiguredStages(outputIDs)
	desired := make(map[string]struct{}, len(outputIDs))
	for _, outputID := range outputIDs {
		desired[outputID] = struct{}{}
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.desired = desired
	var stale []*outputWindow
	for outputID, output := range m.windows {
		if _, keep := desired[outputID]; !keep {
			stale = append(stale, output)
		}
	}
	m.mu.Unlock()
	for _, outputID := range outputIDs {
		m.ensureOutput(outputID)
	}
	for _, output := range stale {
		if output.window != nil {
			output.window.Perform(system.ActionClose)
		}
	}
}

func (m *Manager) ensureOutput(outputID string) {
	if outputID == "" {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	if _, exists := m.windows[outputID]; exists {
		m.mu.Unlock()
		return
	}
	output := &outputWindow{id: outputID, manager: m, players: map[string]*Player{}}
	m.windows[outputID] = output
	m.mu.Unlock()
	go output.run()
}

func (m *Manager) removed(outputID string) {
	m.mu.Lock()
	delete(m.windows, outputID)
	m.mu.Unlock()
}

func (m *Manager) shouldRecoverOutput(outputID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	_, desired := m.desired[outputID]
	return desired
}

func (m *Manager) Close() {
	m.resetMu.Lock()
	defer m.resetMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.cancel()
	m.desired = map[string]struct{}{}
	windows := make([]*outputWindow, 0, len(m.windows))
	for _, output := range m.windows {
		windows = append(windows, output)
	}
	m.mu.Unlock()
	m.workers.Wait()
	for _, output := range windows {
		if output.window != nil {
			output.window.Perform(system.ActionClose)
		}
	}
	m.backendMu.Lock()
	defer m.backendMu.Unlock()
	if m.decoder != nil {
		m.decoder.Close()
	}
	if m.audio != nil {
		m.audio.Close()
	}
}

// EmergencyReset force-closes every decoder and hardware-audio source, creates
// fresh backend resources, and restarts output windows. It is intentionally
// stronger than STOP ALL and can recover output-local players that have fallen
// out of sync with the engine registry.
func (m *Manager) EmergencyReset(ctx context.Context) error {
	m.resetMu.Lock()
	defer m.resetMu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("media manager is closed")
	}
	windows := make([]*outputWindow, 0, len(m.windows))
	for _, output := range m.windows {
		windows = append(windows, output)
	}
	m.mu.Unlock()

	m.backendMu.Lock()
	if m.decoder != nil {
		m.decoder.Close()
	}
	if m.audio != nil {
		m.audio.Close()
	}
	audioSystem, audioErr := NewAudioSystem(m.settings)
	m.audio = audioSystem
	m.decoder = NewFFmpegBackend(m.settings, audioSystem)
	m.backendMu.Unlock()

	for _, output := range windows {
		if output.window != nil {
			output.window.Perform(system.ActionClose)
		}
	}
	m.RefreshAudioDeviceStatus()
	if audioErr != nil {
		return fmt.Errorf("reinitialize audio output: %w", audioErr)
	}
	return nil
}

func (m *Manager) playbackBackend() PlaybackBackend {
	m.backendMu.RLock()
	defer m.backendMu.RUnlock()
	return m.decoder
}

type outputWindow struct {
	id              string
	manager         *Manager
	window          *app.Window
	players         map[string]*Player
	clickable       widget.Clickable
	fullscreen      bool
	blackout        bool
	test            bool
	identify        bool
	identifyMessage string
	reopening       bool
	transition      *outputTransition
	nativeHandle    uintptr
	routed          bool
	displayMissing  bool
	lastGeometry    [4]int
	heldFrame       image.Image
	routeMu         sync.Mutex
	lastSequence    uint64
	geometryUpdates chan [4]int
}

func (m *Manager) outputIDsWithConfiguredStages(outputIDs []string) []string {
	seen := make(map[string]struct{}, len(outputIDs))
	result := make([]string, 0, len(outputIDs)+len(m.settings.Snapshot().VideoOutputs))
	for _, outputID := range outputIDs {
		if outputID != "" {
			if _, exists := seen[outputID]; !exists {
				seen[outputID], result = struct{}{}, append(result, outputID)
			}
		}
	}
	for _, output := range m.settings.Snapshot().VideoOutputs {
		if _, exists := seen[output.Stage]; !exists {
			seen[output.Stage], result = struct{}{}, append(result, output.Stage)
		}
	}
	return result
}

type outputTransition struct {
	event   playback.Event
	stage   string
	started time.Time
}
