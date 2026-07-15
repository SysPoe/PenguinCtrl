package media

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/gen2brain/malgo"
)

// audioDeviceTopology exclusively owns the malgo context and all device
// enumeration/configuration. Mixers receive opened devices, never the context.
type audioDeviceTopology struct {
	mu      sync.Mutex
	context *malgo.AllocatedContext
	closed  bool
}

func newAudioDeviceTopology() (*audioDeviceTopology, error) {
	context, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initialize audio system: %w", err)
	}
	return &audioDeviceTopology{context: context}, nil
}

func (topology *audioDeviceTopology) devices() ([]AudioDevice, error) {
	topology.mu.Lock()
	defer topology.mu.Unlock()
	if topology.closed || topology.context == nil {
		return nil, errors.New("audio system is unavailable")
	}
	devices, err := topology.context.Devices(malgo.Playback)
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

func (topology *audioDeviceTopology) openPlaybackDevice(deviceID string, callbacks malgo.DeviceCallbacks) (*malgo.Device, error) {
	topology.mu.Lock()
	defer topology.mu.Unlock()
	if topology.closed || topology.context == nil {
		return nil, errors.New("audio system is closed")
	}
	deviceConfig, err := topology.deviceConfigLocked(deviceID)
	if err != nil {
		return nil, err
	}
	device, err := malgo.InitDevice(topology.context.Context, deviceConfig, callbacks)
	if err != nil {
		return nil, fmt.Errorf("open audio device: %w", err)
	}
	return device, nil
}

func (topology *audioDeviceTopology) deviceConfigLocked(deviceID string) (malgo.DeviceConfig, error) {
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = audioChannels
	deviceConfig.SampleRate = audioSampleRate
	if deviceID == "" {
		return deviceConfig, nil
	}
	devices, err := topology.context.Devices(malgo.Playback)
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

func (topology *audioDeviceTopology) close() {
	topology.mu.Lock()
	if topology.closed {
		topology.mu.Unlock()
		return
	}
	topology.closed = true
	context := topology.context
	topology.context = nil
	topology.mu.Unlock()
	if context != nil {
		_ = context.Uninit()
		context.Free()
	}
}
