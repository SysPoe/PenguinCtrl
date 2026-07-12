package media

// Backend is the media-output surface used by the application.
type Backend interface {
	AudioDevices() ([]AudioDevice, error)
	AudioDeviceWarning() string
	RefreshAudioDeviceStatus()
	EnsureOutputs([]string)
	SyncOutputs([]string)
}
