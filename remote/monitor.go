package remote

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/syspoe/cusus/config"
)

const (
	targetProbeInterval = 2 * time.Second
	targetProbeTimeout  = 500 * time.Millisecond
)

// TargetMonitor continuously probes configured remote targets and records the
// observations made by command dispatches.
type TargetMonitor struct {
	settings  settingsProvider
	transport transport
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}

	mu     sync.RWMutex
	health map[string]TargetHealth
}

// NewTargetMonitor starts monitoring the targets supplied by settings.
func NewTargetMonitor(settings settingsProvider) *TargetMonitor {
	return newTargetMonitor(settings, &networkTransport{})
}

func newTargetMonitor(settings settingsProvider, io transport) *TargetMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	monitor := &TargetMonitor{
		settings:  settings,
		transport: io,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
		health:    make(map[string]TargetHealth),
	}
	go monitor.run()
	return monitor
}

// Close stops the probe loop.
func (m *TargetMonitor) Close() {
	if m == nil || m.cancel == nil {
		return
	}
	m.cancel()
	<-m.done
	m.cancel = nil
}

// Health returns a stable snapshot ordered by target name.
func (m *TargetMonitor) Health() []TargetHealth {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]TargetHealth, 0, len(m.health))
	for _, health := range m.health {
		result = append(result, health)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (m *TargetMonitor) record(target config.RemoteTarget, err error, acknowledged *bool, roundTrip time.Duration) {
	if m == nil {
		return
	}
	key := targetLabel(target)
	now := time.Now()
	m.mu.Lock()
	health := m.health[key]
	health.Name, health.Host, health.Known, health.LastChecked, health.RoundTrip = key, target.Host, true, now, roundTrip
	if acknowledged != nil {
		health.Acknowledged = *acknowledged
	}
	if err == nil {
		health.Reachable, health.LastSuccess, health.LastError, health.ConsecutiveFailures = true, now, "", 0
	} else {
		health.Reachable, health.LastError, health.ConsecutiveFailures = false, err.Error(), health.ConsecutiveFailures+1
	}
	m.health[key] = health
	m.mu.Unlock()
}

func (m *TargetMonitor) RecordDispatch(target config.RemoteTarget, err error, acknowledged bool, roundTrip time.Duration) {
	m.record(target, err, &acknowledged, roundTrip)
}

func (m *TargetMonitor) run() {
	defer close(m.done)
	ticker := time.NewTicker(targetProbeInterval)
	defer ticker.Stop()
	for {
		m.probeTargets()
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *TargetMonitor) probeTargets() {
	var probes sync.WaitGroup
	for _, target := range m.settings.Snapshot().RemoteTargets {
		if target.HealthPort <= 0 {
			continue
		}
		probes.Add(1)
		go func() {
			defer probes.Done()
			started := time.Now()
			ctx, cancel := context.WithTimeout(m.ctx, targetProbeTimeout)
			defer cancel()
			err := m.transport.Probe(ctx, target.Host, target.HealthPort)
			m.record(target, err, nil, time.Since(started))
		}()
	}
	probes.Wait()
}
