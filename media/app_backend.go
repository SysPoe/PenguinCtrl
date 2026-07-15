package media

import "context"

// Backend is the media-output surface used by the application.
// TODO(macro): Backend, EmergencyResetter, and the optional video-routing /
// Prewarm capability asserts used from package main are three partial ports for
// one subsystem. Publish one intentional media.Host (or composition of ports)
// covering devices, outputs, emergency reset, routing, and prewarm so main does
// not type-assert ad-hoc interfaces onto *Manager.
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
