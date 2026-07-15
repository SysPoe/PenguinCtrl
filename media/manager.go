package media

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/playback"
)

// Manager is the stable application facade over independently owned media
// runtime, device-topology, and output-window lifecycles.
type Manager struct {
	lifecycleMu sync.Mutex
	closed      bool
	runtime     *mediaRuntime
	topology    *deviceTopology
	outputs     *outputController
}

var _ Host = (*Manager)(nil)

func NewManager(engine *playback.Engine, settings *config.Store) *Manager {
	runtime, _ := newMediaRuntime(settings)
	topology := newDeviceTopology(settings, runtime)
	outputs := newOutputController(engine, settings, runtime, topology)
	topology.onDisplaysChanged = outputs.refreshRoutes
	topology.start()
	return &Manager{runtime: runtime, topology: topology, outputs: outputs}
}

func (m *Manager) Prewarm(specs []playback.PreloadSpec) { m.runtime.prewarm(specs) }

func (m *Manager) AudioDevices() ([]AudioDevice, error) { return m.topology.audioDevicesSnapshot() }

func (m *Manager) AudioMixerMetrics() []AudioMixerMetrics { return m.runtime.mixerMetrics() }

func (m *Manager) AudioDeviceWarning() string { return m.topology.audioWarning() }

func (m *Manager) RefreshAudioDeviceStatus() { m.topology.refreshAudioStatus() }

func (m *Manager) VideoDisplays() ([]VideoDisplay, error) { return m.topology.videoDisplays() }

func (m *Manager) VideoOutputWarning() string { return m.topology.videoWarning() }

func (m *Manager) RefreshVideoOutputStatus() { m.topology.refreshVideoStatus() }

func (m *Manager) EnsureOutputs(outputIDs []string) { m.outputs.ensure(outputIDs) }

func (m *Manager) SyncOutputs(outputIDs []string) { m.outputs.sync(outputIDs) }

func (m *Manager) Close() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	m.topology.close()
	m.outputs.close()
	m.runtime.close()
	return nil
}

// EmergencyReset force-closes every decoder and hardware-audio source, creates
// fresh backend resources, and restarts output windows.
func (m *Manager) EmergencyReset(ctx context.Context) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if m.closed {
		return errors.New("media manager is closed")
	}
	audioErr := m.runtime.reset()
	m.outputs.restart()
	m.topology.refreshAudioStatus()
	if audioErr != nil {
		return fmt.Errorf("reinitialize audio output: %w", audioErr)
	}
	return nil
}
