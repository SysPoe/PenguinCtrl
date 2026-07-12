package media

// Backend is the media-output surface used by the application.
type Backend interface {
	AudioDevices() ([]AudioDevice, error)
	AudioDeviceWarning() string
	RefreshAudioDeviceStatus()
	VideoDisplays() ([]VideoDisplay, error)
	VideoOutputWarning() string
	RefreshVideoOutputStatus()
	EnsureOutputs([]string)
	SyncOutputs([]string)
}
