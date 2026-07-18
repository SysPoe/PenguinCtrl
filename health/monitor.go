package health

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Provider collects the current component observations for a Monitor refresh.
type Provider func() []Component

// Monitor periodically snapshots a Provider until it is closed.
type Monitor struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	latest Snapshot
}

// NewMonitor starts a monitor that refreshes immediately and then at interval.
func NewMonitor(provider Provider, interval time.Duration) *Monitor {
	if provider == nil {
		provider = func() []Component { return nil }
	}
	if interval <= 0 {
		interval = time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	// Leave Generated zero until the provider publishes its first complete
	// collection so readiness consumers can distinguish pending from empty.
	monitor := &Monitor{cancel: cancel}
	monitor.wg.Add(1)
	go monitor.run(ctx, provider, interval)
	return monitor
}

func (m *Monitor) run(ctx context.Context, provider Provider, interval time.Duration) {
	defer m.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		snapshot := NewSnapshot(provider())
		m.mu.Lock()
		m.latest = snapshot
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Snapshot returns an isolated copy of the latest health snapshot.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copyOf := m.latest
	copyOf.Components = slices.Clone(m.latest.Components)
	return copyOf
}

// Close stops the monitor and waits for its collector goroutine to exit.
func (m *Monitor) Close() {
	m.cancel()
	m.wg.Wait()
}
