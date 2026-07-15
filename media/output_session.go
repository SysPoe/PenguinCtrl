package media

import (
	"context"
	"errors"
	"image"
	"log"

	"github.com/syspoe/cusus/playback"
)

// stageSession owns the playback instances routed to one stage. The output
// window presents its visual players, but audio-only instances remain session
// state rather than presentation state.
type stageSession struct {
	window       *outputWindow
	players      map[string]*Player
	heldFrame    image.Image
	lastSequence uint64
}

func newStageSession(window *outputWindow) stageSession {
	return stageSession{window: window, players: make(map[string]*Player)}
}

func (session *stageSession) handleEvent(event playback.Event) {
	if event.Action == "resync" {
		events, sequence := session.window.controller.port.OutputSnapshot(session.window.id)
		for _, snapshotEvent := range events {
			session.applyEvent(snapshotEvent)
		}
		session.lastSequence = sequence
		return
	}
	if event.Sequence > 0 && event.Sequence <= session.lastSequence {
		return
	}
	session.applyEvent(event)
	if event.Sequence > session.lastSequence {
		session.lastSequence = event.Sequence
	}
}

func (session *stageSession) applyEvent(event playback.Event) {
	switch event.Action {
	case "sync":
		session.reconcile(event.Instances)
	case "play":
		session.start(event.Instance)
	case "remove":
		for _, id := range event.InstanceIDs {
			if player := session.players[id]; player != nil {
				if session.window.route().IdleBehavior == "hold" {
					session.heldFrame = player.Frame()
				}
				player.Close(false)
				delete(session.players, id)
			}
		}
	case "control":
		if event.Control == "stop-all" {
			session.heldFrame = nil
			session.closePlayers(false)
			return
		}
		for _, id := range event.InstanceIDs {
			if player := session.players[id]; player != nil {
				player.Control(event)
			}
		}
	case "output":
		session.window.handleOutputControl(event)
	}
}

func (session *stageSession) reconcile(instances []playback.Instance) {
	desired := make(map[string]playback.Instance, len(instances))
	for _, instance := range instances {
		desired[instance.ID] = instance
	}
	for id, player := range session.players {
		if _, keep := desired[id]; keep {
			continue
		}
		player.Close(false)
		delete(session.players, id)
	}
	for _, instance := range instances {
		if session.players[instance.ID] == nil {
			session.start(&instance)
		}
	}
}

func (session *stageSession) start(instance *playback.Instance) {
	if instance == nil || session.players[instance.ID] != nil {
		return
	}
	window := session.window
	backend := window.controller.runtime.backend()
	if backend == nil {
		window.controller.port.HandleOutputError(instance.ID, errors.New("media backend is unavailable"))
		return
	}
	player := NewPlayerWithBackend(
		*instance,
		window.controller.settings,
		backend,
		window.window,
		func(report string) { window.controller.port.HandleOutputReport(instance.ID, report) },
		func(durationMs int64) { window.controller.port.HandleOutputDuration(instance.ID, durationMs) },
		func(err error) { window.controller.port.HandleOutputError(instance.ID, err) },
	)
	session.players[instance.ID] = player
	player.goOwned(func(context.Context) {
		if err := player.Start(); err != nil {
			log.Printf("play %s: %v", instance.Source, err)
			window.controller.port.HandleOutputError(instance.ID, err)
		}
	})
}

func (session *stageSession) visualPlayers() []*Player {
	visible := make([]*Player, 0, len(session.players))
	for _, player := range session.players {
		if player.MediaType() == playback.MediaTypeVideo || player.MediaType() == playback.MediaTypeImage {
			visible = append(visible, player)
		}
	}
	return visible
}

func (session *stageSession) setVisibleDecoders(visible []*Player) {
	shown := make(map[*Player]struct{}, len(visible))
	for _, player := range visible {
		shown[player] = struct{}{}
	}
	for _, player := range session.players {
		_, isVisible := shown[player]
		player.SetDecodeVisible(isVisible)
	}
}

func (session *stageSession) closePlayers(fade bool) {
	for id, player := range session.players {
		player.Close(fade)
		delete(session.players, id)
	}
}
