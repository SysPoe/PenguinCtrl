package media

import (
	"io"

	"github.com/syspoe/cusus/config"
)

// audioPlayerFactory owns settings-to-route selection and source construction.
type audioPlayerFactory struct {
	settings *config.Store
	mixers   *audioMixerRegistry
}

func (factory *audioPlayerFactory) preparedPlayer(reader io.Reader, preview bool) (*devicePlayer, error) {
	deviceID, recoveryPolicy, backupID := config.AudioRoute(factory.settings.Snapshot(), preview)
	return factory.preparedPlayerForRoute(reader, deviceID, recoveryPolicy, backupID)
}

func (factory *audioPlayerFactory) preparedPlayerForRoute(reader io.Reader, deviceID, recoveryPolicy, backupID string) (*devicePlayer, error) {
	return factory.mixers.newSource(reader, deviceID, recoveryPolicy, backupID)
}
