package playback

import (
	"sync"

	"github.com/syspoe/cusus/show"
)

type desiredOutputSnapshot struct {
	visual    Event
	window    Event
	hasVisual bool
	hasWindow bool
}

// desiredOutputState owns the persistent non-media mutations needed to rebuild
// an output after resync or window recreation.
type desiredOutputState struct {
	mu      sync.RWMutex
	visuals map[string]Event
	windows map[string]Event
}

func newDesiredOutputState() *desiredOutputState {
	return &desiredOutputState{visuals: map[string]Event{}, windows: map[string]Event{}}
}

func (state *desiredOutputState) rememberVisual(outputID string, event Event) {
	state.mu.Lock()
	state.visuals[outputID] = event
	state.mu.Unlock()
}

func (state *desiredOutputState) rememberWindow(outputID string, event Event) {
	state.mu.Lock()
	state.windows[outputID] = event
	state.mu.Unlock()
}

func (state *desiredOutputState) snapshot(outputID string) desiredOutputSnapshot {
	state.mu.RLock()
	defer state.mu.RUnlock()
	visual, hasVisual := state.visuals[outputID]
	window, hasWindow := state.windows[outputID]
	return desiredOutputSnapshot{visual: visual, window: window, hasVisual: hasVisual, hasWindow: hasWindow}
}

// outputCoordinator combines incremental publication with the authoritative
// desired-state snapshots used for subscriptions and resyncs.
type outputCoordinator struct {
	bus     *outputBus
	desired *desiredOutputState
	runtime *runtimeState
}

func newOutputCoordinator(runtime *runtimeState) *outputCoordinator {
	return &outputCoordinator{bus: newOutputBus(), desired: newDesiredOutputState(), runtime: runtime}
}

func (coordinator *outputCoordinator) publish(event outputEvent) { coordinator.bus.publish(event) }

func (coordinator *outputCoordinator) subscribe(outputID string) chan Event {
	return coordinator.bus.subscribe(outputID)
}

func (coordinator *outputCoordinator) unsubscribe(outputID string, events chan Event) {
	coordinator.bus.unsubscribe(outputID, events)
}

func (coordinator *outputCoordinator) subscribeSnapshot(outputID string) (<-chan Event, func()) {
	events, release := coordinator.bus.subscribePaused(outputID)
	snapshot, _ := coordinator.snapshot(outputID)
	for _, event := range snapshot {
		events <- event
	}
	release()
	return events, func() { coordinator.bus.unsubscribe(outputID, events) }
}

func (coordinator *outputCoordinator) snapshot(outputID string) ([]Event, uint64) {
	sequence := coordinator.bus.currentSequence()
	instances := coordinator.runtime.matching(show.MediaTarget{Kind: show.MediaTargetOutput, OutputID: outputID})
	desired := coordinator.desired.snapshot(outputID)
	snapshots := make([]MediaSnapshot, 0, len(instances))
	for _, instance := range instances {
		snapshots = append(snapshots, snapshotMedia(instance))
	}
	syncEvent := syncOutputEvent{outputID: outputID, instances: snapshots}.compatibilityEvent()
	syncEvent.Sequence = sequence
	events := []Event{syncEvent}
	if desired.hasVisual {
		desired.visual.Sequence = sequence
		events = append(events, desired.visual)
	}
	if desired.hasWindow {
		desired.window.Sequence = sequence
		events = append(events, desired.window)
	}
	return events, sequence
}

func (coordinator *outputCoordinator) setOnResync(callback func(string, uint64, int)) {
	coordinator.bus.setOnResync(callback)
}

func (coordinator *outputCoordinator) resyncCount() uint64 { return coordinator.bus.resyncCount() }

func (coordinator *outputCoordinator) rememberVisual(outputID string, event Event) {
	coordinator.desired.rememberVisual(outputID, event)
}

func (coordinator *outputCoordinator) rememberWindow(outputID string, event Event) {
	coordinator.desired.rememberWindow(outputID, event)
}
