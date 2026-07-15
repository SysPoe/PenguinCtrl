package playback

import (
	"sync"

	"github.com/syspoe/cusus/operatorlog"
)

// engineHooks owns mutable integration callbacks. Keeping these outside the
// runtime lock means logging and UI notification cannot delay media lifecycle
// transitions.
type engineHooks struct {
	mu              sync.RWMutex
	onChange        func()
	operatorLog     *operatorlog.Store
	remoteAuthority func(func() error) error
}

func (hooks *engineHooks) setOnChange(callback func()) {
	hooks.mu.Lock()
	hooks.onChange = callback
	hooks.mu.Unlock()
}

func (hooks *engineHooks) changed() {
	hooks.mu.RLock()
	callback := hooks.onChange
	hooks.mu.RUnlock()
	if callback != nil {
		callback()
	}
}

func (hooks *engineHooks) setOperatorLog(store *operatorlog.Store) {
	hooks.mu.Lock()
	hooks.operatorLog = store
	hooks.mu.Unlock()
}

func (hooks *engineHooks) operatorLogStore() *operatorlog.Store {
	hooks.mu.RLock()
	defer hooks.mu.RUnlock()
	return hooks.operatorLog
}

func (hooks *engineHooks) setRemoteAuthority(executor func(func() error) error) {
	hooks.mu.Lock()
	hooks.remoteAuthority = executor
	hooks.mu.Unlock()
}

func (hooks *engineHooks) remoteAuthorityExecutor() func(func() error) error {
	hooks.mu.RLock()
	defer hooks.mu.RUnlock()
	return hooks.remoteAuthority
}
