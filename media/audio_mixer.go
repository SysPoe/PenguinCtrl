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
	audioRecoveryInitialBackoff = 250 * time.Millisecond
	audioRecoveryMaxBackoff     = 4 * time.Second
	audioRecoveryAttempts       = 6
)

type audioSourceRoute struct {
	policy   string
	backupID string
	recover  func(string) error
}

// audioMixerRegistry owns endpoint mixers and non-realtime failover metadata.
// devicePlayer carries only PCM callback/lifecycle state.
type audioMixerRegistry struct {
	topology *audioDeviceTopology
	mu       sync.Mutex
	mixers   map[string]*endpointMixer
	routes   map[*devicePlayer]audioSourceRoute
	closed   bool
}

func newAudioMixerRegistry(topology *audioDeviceTopology) *audioMixerRegistry {
	return &audioMixerRegistry{
		topology: topology,
		mixers:   make(map[string]*endpointMixer),
		routes:   make(map[*devicePlayer]audioSourceRoute),
	}
}

func (registry *audioMixerRegistry) newSource(reader io.Reader, deviceID, recoveryPolicy, backupID string) (*devicePlayer, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed || registry.topology == nil {
		return nil, errors.New("audio system is closed")
	}
	mixer := registry.mixers[deviceID]
	if mixer == nil {
		mixer = &endpointMixer{registry: registry, topology: registry.topology, deviceID: deviceID}
		mixer.sources.Store([]*devicePlayer{})
		if err := mixer.openDeviceLocked(); err != nil {
			return nil, err
		}
		registry.mixers[deviceID] = mixer
	}
	player := newDevicePlayer(reader, mixer, registry)
	registry.routes[player] = audioSourceRoute{policy: recoveryPolicy, backupID: backupID}
	go player.fillRing()
	return player, nil
}

func (registry *audioMixerRegistry) setRecoveryHandler(player *devicePlayer, handler func(string) error) {
	registry.mu.Lock()
	route, ok := registry.routes[player]
	if ok && !registry.closed {
		route.recover = handler
		registry.routes[player] = route
	}
	registry.mu.Unlock()
}

func (registry *audioMixerRegistry) removeSource(player *devicePlayer) {
	registry.mu.Lock()
	delete(registry.routes, player)
	registry.mu.Unlock()
}

func (registry *audioMixerRegistry) failoverSource(player *devicePlayer, currentDeviceID string) bool {
	player.mu.Lock()
	closed := player.closed
	player.mu.Unlock()
	if closed {
		return true
	}
	registry.mu.Lock()
	route, ok := registry.routes[player]
	registry.mu.Unlock()
	if !ok {
		return false
	}
	targetID, allowed := fallbackDeviceID(route.policy, route.backupID)
	if !allowed || targetID == currentDeviceID || (route.policy == config.AudioRecoveryNamedBackup && targetID == "") {
		return false
	}
	return route.recover != nil && route.recover(targetID) == nil
}

func (registry *audioMixerRegistry) lockedMixers() []*endpointMixer {
	mixers := make([]*endpointMixer, 0, len(registry.mixers))
	for _, mixer := range registry.mixers {
		mixers = append(mixers, mixer)
	}
	return mixers
}

func (registry *audioMixerRegistry) metrics() []AudioMixerMetrics {
	registry.mu.Lock()
	mixers := registry.lockedMixers()
	registry.mu.Unlock()
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

func (registry *audioMixerRegistry) close() {
	registry.mu.Lock()
	if registry.closed {
		registry.mu.Unlock()
		return
	}
	registry.closed = true
	mixers := registry.lockedMixers()
	registry.mixers = nil
	registry.routes = nil
	registry.mu.Unlock()
	for _, mixer := range mixers {
		mixer.close()
	}
}

type endpointMixer struct {
	registry    *audioMixerRegistry
	topology    *audioDeviceTopology
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

func (m *endpointMixer) openDeviceLocked() error {
	if m.topology == nil {
		return errors.New("audio device topology is unavailable")
	}
	callbacks := malgo.DeviceCallbacks{Data: m.mix, Stop: m.deviceStopped}
	device, err := m.topology.openPlaybackDevice(m.deviceID, callbacks)
	if err != nil {
		return err
	}
	m.device = device
	return nil
}

func (m *endpointMixer) add(player *devicePlayer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.device == nil {
		return errors.New("audio mixer is unavailable")
	}
	current := m.sources.Load().([]*devicePlayer)
	for _, source := range current {
		if source == player {
			return nil
		}
	}
	next := append(append([]*devicePlayer(nil), current...), player)
	m.sources.Store(next)
	if !m.started {
		if err := m.device.Start(); err != nil {
			m.sources.Store(current)
			return fmt.Errorf("start audio device: %w", err)
		}
		m.started = true
	}
	return nil
}

func (m *endpointMixer) remove(player *devicePlayer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.sources.Load().([]*devicePlayer)
	next := make([]*devicePlayer, 0, len(current))
	for _, source := range current {
		if source != player {
			next = append(next, source)
		}
	}
	m.sources.Store(next)
}

func (m *endpointMixer) mix(output, _ []byte, _ uint32) {
	started := time.Now()
	clear(output)
	for _, source := range m.sources.Load().([]*devicePlayer) {
		source.mixInto(output)
	}
	now := time.Now()
	m.callbackAt.Store(now.UnixNano())
	duration := now.Sub(started).Nanoseconds()
	for previous := m.callbackMax.Load(); duration > previous && !m.callbackMax.CompareAndSwap(previous, duration); previous = m.callbackMax.Load() {
	}
}

func (m *endpointMixer) deviceStopped() {
	m.mu.Lock()
	intentional := m.closed
	m.started = false
	m.mu.Unlock()
	if intentional || !m.recovering.CompareAndSwap(false, true) {
		return
	}
	go m.recover()
}

func (m *endpointMixer) recover() {
	defer m.recovering.Store(false)
	backoff := audioRecoveryInitialBackoff
	for range audioRecoveryAttempts {
		time.Sleep(backoff)
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return
		}
		old := m.device
		m.device = nil
		m.mu.Unlock()
		if old != nil {
			old.Uninit()
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return
		}
		err := m.openDeviceLocked()
		if err == nil && len(m.sources.Load().([]*devicePlayer)) > 0 {
			err = m.device.Start()
			m.started = err == nil
		}
		m.mu.Unlock()
		if err == nil {
			m.failed.Store(false)
			m.recoveries.Add(1)
			return
		}
		backoff = min(audioRecoveryMaxBackoff, backoff*2)
	}
	failed := false
	for _, source := range m.sources.Load().([]*devicePlayer) {
		if m.registry == nil || !m.registry.failoverSource(source, m.deviceID) {
			failed = true
			source.stoppedOnce.Do(func() { close(source.stopped) })
		}
	}
	m.failed.Store(failed)
	if !failed {
		m.recoveries.Add(1)
	}
}

func fallbackDeviceID(policy, backupID string) (string, bool) {
	switch policy {
	case config.AudioRecoveryFollowDefault:
		return "", true
	case config.AudioRecoveryNamedBackup:
		backupID = strings.TrimSpace(backupID)
		return backupID, backupID != ""
	default:
		return "", false
	}
}

func (m *endpointMixer) close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	device := m.device
	m.device = nil
	m.mu.Unlock()
	if device != nil {
		device.Uninit()
	}
}
