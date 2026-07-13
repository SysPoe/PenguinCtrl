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

func (m *endpointMixer) openDeviceLocked() error {
	config, err := m.system.deviceConfig(m.deviceID)
	if err != nil {
		return err
	}
	callbacks := malgo.DeviceCallbacks{Data: m.mix, Stop: m.deviceStopped}
	device, err := malgo.InitDevice(m.system.context.Context, config, callbacks)
	if err != nil {
		return fmt.Errorf("open audio device: %w", err)
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
	backoff := 250 * time.Millisecond
	for attempt := 0; attempt < 6; attempt++ {
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
		backoff = min(4*time.Second, backoff*2)
	}
	failed := false
	for _, source := range m.sources.Load().([]*devicePlayer) {
		if !m.failover(source) {
			failed = true
			source.stoppedOnce.Do(func() { close(source.stopped) })
		}
	}
	m.failed.Store(failed)
	if !failed {
		m.recoveries.Add(1)
	}
}

func (m *endpointMixer) failover(source *devicePlayer) bool {
	targetID, allowed := fallbackDeviceID(source.recoveryPolicy, source.backupDeviceID)
	if !allowed {
		return false
	}
	if targetID == m.deviceID || (source.recoveryPolicy == config.AudioRecoveryNamedBackup && targetID == "") {
		return false
	}
	return source.recoverTo(targetID)
}

func fallbackDeviceID(policy, backupID string) (string, bool) {
	switch policy {
	case config.AudioRecoveryFollowDefault:
		return "", true
	case config.AudioRecoveryNamedBackup:
		return strings.TrimSpace(backupID), strings.TrimSpace(backupID) != ""
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

type devicePlayer struct {
	reader         io.Reader
	mixer          *endpointMixer
	recoveryPolicy string
	backupDeviceID string
	volume         atomic.Uint64
	ring           *pcmRing
	done           chan struct{}
	ready          chan struct{}
	readyOnce      sync.Once
	stopped        chan struct{}
	stoppedOnce    sync.Once
	intentional    atomic.Bool
	eof            atomic.Bool
	underruns      atomic.Uint64
	mu             sync.Mutex
	closed         bool
	started        bool
	recovery       func(string) error
	renderedFrames atomic.Uint64
}
