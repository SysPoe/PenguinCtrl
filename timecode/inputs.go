package timecode

import (
	"context"
	"sync"
)

type InputState string

const (
	InputStopped InputState = "stopped"
	InputRunning InputState = "running"
	InputFailed  InputState = "failed"
)

type InputStatus struct {
	Source    Source
	State     InputState
	LastError error
}

type ServiceStatus struct {
	Selected Source
	Closed   bool
	Inputs   []InputStatus
}

type inputDriver struct {
	run func(context.Context, string) error
}

type inputRegistry struct {
	mu        sync.RWMutex
	drivers   map[Source]inputDriver
	statuses  map[Source]InputStatus
	order     []Source
	cancel    context.CancelFunc
	active    Source
	wg        sync.WaitGroup
	lastError error
	ltc       *LTCAdapter
	mtc       *MTCAdapter
}

func newInputRegistry(coordinator *Coordinator) *inputRegistry {
	registry := &inputRegistry{
		drivers:  make(map[Source]inputDriver, 3),
		statuses: make(map[Source]InputStatus, 3),
		order:    []Source{SourceLTC, SourceMTC, SourceOSC},
		ltc:      NewLTCAdapter(coordinator),
		mtc:      NewMTCAdapter(coordinator),
	}
	registry.drivers[SourceLTC] = inputDriver{run: waitForInputClose}
	registry.drivers[SourceMTC] = inputDriver{run: waitForInputClose}
	registry.drivers[SourceOSC] = inputDriver{run: func(ctx context.Context, address string) error {
		return ListenOSC(ctx, address, coordinator)
	}}
	for _, source := range registry.order {
		registry.statuses[source] = InputStatus{Source: source, State: InputStopped}
	}
	return registry
}

func waitForInputClose(ctx context.Context, _ string) error {
	<-ctx.Done()
	return nil
}

func (registry *inputRegistry) Start(source Source, listenAddress string) {
	registry.Close()
	driver, ok := registry.drivers[source]
	registry.mu.Lock()
	registry.lastError = nil
	if !ok {
		registry.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	registry.cancel = cancel
	registry.active = source
	registry.statuses[source] = InputStatus{Source: source, State: InputRunning}
	registry.wg.Add(1)
	registry.mu.Unlock()

	go func() {
		defer registry.wg.Done()
		err := driver.run(ctx, listenAddress)
		registry.mu.Lock()
		defer registry.mu.Unlock()
		if ctx.Err() == nil && err != nil {
			registry.statuses[source] = InputStatus{Source: source, State: InputFailed, LastError: err}
			registry.lastError = err
			return
		}
		registry.statuses[source] = InputStatus{Source: source, State: InputStopped}
	}()
}

func (registry *inputRegistry) Close() {
	registry.mu.Lock()
	cancel := registry.cancel
	active := registry.active
	registry.cancel = nil
	registry.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	registry.wg.Wait()
	registry.mu.Lock()
	if active != "" {
		registry.statuses[active] = InputStatus{Source: active, State: InputStopped}
	}
	if registry.active == active {
		registry.active = ""
	}
	registry.mu.Unlock()
}

func (registry *inputRegistry) Statuses() []InputStatus {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]InputStatus, 0, len(registry.order))
	for _, source := range registry.order {
		result = append(result, registry.statuses[source])
	}
	return result
}

func (registry *inputRegistry) LastError() error {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return registry.lastError
}
