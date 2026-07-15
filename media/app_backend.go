package media

import (
	"context"

	"github.com/syspoe/cusus/playback"
)

// Backend is the media-output surface used by the application.
// TODO(micro): Backend.Close has no error return while EmergencyReset does; align Close() error handling with EmergencyResetter for consistent failure reporting
type Backend interface {
	AudioDevices() ([]AudioDevice, error)
	AudioDeviceWarning() string
	AudioMixerMetrics() []AudioMixerMetrics
	RefreshAudioDeviceStatus()
	VideoDisplays() ([]VideoDisplay, error)
	VideoOutputWarning() string
	RefreshVideoOutputStatus()
	EnsureOutputs([]string)
	SyncOutputs([]string)
	Close()
}

// EmergencyResetter is implemented by media backends that can tear down and
// recreate their decoder and hardware-audio resources while the app remains
// running.
type EmergencyResetter interface {
	EmergencyReset(context.Context) error
}

// Host is the complete media subsystem surface owned by the application. It
// keeps optional capability assertions out of show-control orchestration.
type Host interface {
	Backend
	EmergencyResetter
	Prewarm([]playback.PreloadSpec)
}
