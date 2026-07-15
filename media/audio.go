package media

import (
	"errors"
	"io"
	"time"

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

// AudioSystem is the stable facade over device topology, endpoint mixers, and
// PCM source construction. Each collaborator owns its own lifecycle and locks.
type AudioSystem struct {
	topology *audioDeviceTopology
	mixers   *audioMixerRegistry
	factory  *audioPlayerFactory
}

func NewAudioSystem(settings *config.Store) (*AudioSystem, error) {
	topology, err := newAudioDeviceTopology()
	if err != nil {
		return nil, err
	}
	mixers := newAudioMixerRegistry(topology)
	return &AudioSystem{
		topology: topology,
		mixers:   mixers,
		factory:  &audioPlayerFactory{settings: settings, mixers: mixers},
	}, nil
}

func (a *AudioSystem) Devices() ([]AudioDevice, error) {
	if a == nil || a.topology == nil {
		return nil, errors.New("audio system is unavailable")
	}
	return a.topology.devices()
}

// NewPlayer is the compatibility construction path for callers that want an
// immediately started source. Playback uses NewPreparedPlayer for A/V sync.
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
	if a == nil || a.factory == nil {
		return nil, errors.New("audio output is unavailable")
	}
	return a.factory.preparedPlayer(reader, preview)
}

func (a *AudioSystem) newPreparedPlayer(reader io.Reader, deviceID, recoveryPolicy, backupID string) (*devicePlayer, error) {
	if a == nil || a.factory == nil {
		return nil, errors.New("audio output is unavailable")
	}
	return a.factory.preparedPlayerForRoute(reader, deviceID, recoveryPolicy, backupID)
}

func (a *AudioSystem) Close() {
	if a == nil {
		return
	}
	if a.mixers != nil {
		a.mixers.close()
	}
	if a.topology != nil {
		a.topology.close()
	}
}

func (a *AudioSystem) Metrics() []AudioMixerMetrics {
	if a == nil || a.mixers == nil {
		return nil
	}
	return a.mixers.metrics()
}
