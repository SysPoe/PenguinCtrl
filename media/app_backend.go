package media

import "context"

// Backend is the media-output surface used by the application.
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
