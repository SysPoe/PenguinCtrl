package playback

import (
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

// NewEngine is the compatibility factory. It owns the remote dispatcher it
// creates and closes it with the engine. New composition roots should construct
// and own the remote transport themselves and use NewEngineWithRemote.
func NewEngine(manager *show.ShowManager, settings *config.Store) *Engine {
	remotePort := remote.NewDispatcher(settings)
	engine := NewEngineWithRemote(manager, settings, remotePort)
	engine.closeCompatibilityRemote = remotePort.Close
	return engine
}
