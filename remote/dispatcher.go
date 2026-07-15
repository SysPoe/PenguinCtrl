package remote

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

const (
	dispatchTimeout     = 750 * time.Millisecond
	acknowledgedRetries = 3
)

type settingsProvider interface {
	Snapshot() config.Settings
}

type Dispatcher struct {
	settings         settingsProvider
	transport        transport
	monitor          *TargetMonitor
	ownsMonitor      bool
	nextID           atomic.Uint64
	lastAcknowledged atomic.Bool
}

type TargetHealth struct {
	Name                string
	Host                string
	Known               bool
	Reachable           bool
	Acknowledged        bool
	LastChecked         time.Time
	LastSuccess         time.Time
	LastError           string
	RoundTrip           time.Duration
	ConsecutiveFailures int
}

// DispatchResult describes how a remote command was sent. Protocols contains
// the concrete transports selected for the configured targets, including the
// transport chosen when the cue requested Auto.
type DispatchResult struct {
	Acknowledged bool
	Protocols    []show.RemoteProtocol
}

func NewDispatcher(settings settingsProvider) *Dispatcher {
	io := &networkTransport{}
	monitor := newTargetMonitor(settings, io)
	return &Dispatcher{settings: settings, transport: io, monitor: monitor, ownsMonitor: true}
}

// NewDispatcherWithMonitor constructs the command path with an independently
// owned target monitor.
func NewDispatcherWithMonitor(settings settingsProvider, monitor *TargetMonitor) *Dispatcher {
	return &Dispatcher{settings: settings, transport: &networkTransport{}, monitor: monitor}
}

func (d *Dispatcher) Close() {
	if d.ownsMonitor {
		d.monitor.Close()
	}
}

func (d *Dispatcher) Health() []TargetHealth { return d.monitor.Health() }

func (d *Dispatcher) LastDispatchAcknowledged() bool { return d.lastAcknowledged.Load() }

func (d *Dispatcher) Dispatch(ctx context.Context, play show.RemotePlay, cue show.Cue) error {
	_, err := d.DispatchWithResult(ctx, play, cue)
	return err
}

func (d *Dispatcher) DispatchWithResult(ctx context.Context, play show.RemotePlay, cue show.Cue) (DispatchResult, error) {
	if play.Action == show.RemoteActionNone {
		return DispatchResult{}, nil
	}
	settings := d.settings.Snapshot()
	resolved := resolvePlay(play, settings, cue.CueNumber)
	if len(settings.RemoteTargets) == 0 {
		return DispatchResult{}, errors.New("no remote control targets are configured")
	}

	d.lastAcknowledged.Store(false)
	type result struct {
		protocol     show.RemoteProtocol
		acknowledged bool
		err          error
	}
	results := make(chan result, len(settings.RemoteTargets))
	for _, target := range settings.RemoteTargets {
		go func() {
			protocol, acknowledged, err := d.dispatchTarget(ctx, target, play.Protocol, resolved)
			results <- result{protocol: protocol, acknowledged: acknowledged, err: err}
		}()
	}
	var dispatchErrors []error
	protocols := map[show.RemoteProtocol]struct{}{}
	successes := 0
	acknowledgedSuccesses := 0
	for range settings.RemoteTargets {
		result := <-results
		if result.protocol == show.RemoteProtocolOSC || result.protocol == show.RemoteProtocolERC {
			protocols[result.protocol] = struct{}{}
		}
		if result.err != nil {
			dispatchErrors = append(dispatchErrors, result.err)
		} else {
			successes++
			if result.acknowledged {
				acknowledgedSuccesses++
			}
		}
	}
	dispatchResult := DispatchResult{Protocols: orderedProtocols(protocols)}
	if settings.RemoteSuccessPolicy == config.RemoteSuccessAny && successes > 0 {
		dispatchResult.Acknowledged = acknowledgedSuccesses > 0
		d.lastAcknowledged.Store(dispatchResult.Acknowledged)
		return dispatchResult, nil
	}
	if len(dispatchErrors) == 0 {
		dispatchResult.Acknowledged = acknowledgedSuccesses == successes && successes > 0
		d.lastAcknowledged.Store(dispatchResult.Acknowledged)
	}
	return dispatchResult, errors.Join(dispatchErrors...)
}

func orderedProtocols(protocols map[show.RemoteProtocol]struct{}) []show.RemoteProtocol {
	result := make([]show.RemoteProtocol, 0, len(protocols))
	for _, protocol := range []show.RemoteProtocol{show.RemoteProtocolOSC, show.RemoteProtocolERC} {
		if _, ok := protocols[protocol]; ok {
			result = append(result, protocol)
		}
	}
	return result
}

func (d *Dispatcher) dispatchTarget(parent context.Context, target config.RemoteTarget, requested show.RemoteProtocol, play show.RemotePlay) (show.RemoteProtocol, bool, error) {
	protocol, port, err := selectTransport(requested, target)
	if err != nil {
		return protocol, false, fmt.Errorf("%s: %w", targetLabel(target), err)
	}
	payload, err := buildPayload(protocol, play)
	if err != nil {
		return protocol, false, fmt.Errorf("%s: %w", targetLabel(target), err)
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, dispatchTimeout)
	defer cancel()
	acknowledged := false
	if target.AckPort > 0 {
		id := fmt.Sprintf("%d-%d", time.Now().UnixMilli(), d.nextID.Add(1))
		for attempt := 0; attempt < acknowledgedRetries; attempt++ {
			err = d.transport.SendAcknowledged(ctx, target.Host, target.AckPort, id, payload)
			if err == nil {
				acknowledged = true
				break
			}
			if ctx.Err() != nil {
				break
			}
		}
	} else {
		err = d.transport.Send(ctx, target.Host, port, payload)
	}
	d.monitor.RecordDispatch(target, err, acknowledged, time.Since(started))
	if err != nil {
		return protocol, false, fmt.Errorf("%s: %w", targetLabel(target), err)
	}
	return protocol, acknowledged, nil
}

func resolvePlay(play show.RemotePlay, settings config.Settings, cueNumber string) show.RemotePlay {
	play.Playback = config.Resolve(play.Playback, settings, cueNumber)
	play.CueNumber = config.Resolve(play.CueNumber, settings, cueNumber)
	play.Level = config.Resolve(play.Level, settings, cueNumber)
	play.Custom = config.Resolve(play.Custom, settings, cueNumber)
	for i := range play.Values {
		play.Values[i].Value = config.Resolve(play.Values[i].Value, settings, cueNumber)
	}
	if strings.TrimSpace(play.Playback) == "" {
		play.Playback = settings.DefaultPlayback
	}
	if strings.TrimSpace(play.CueNumber) == "" {
		play.CueNumber = cueNumber
	}
	return play
}

func selectTransport(protocol show.RemoteProtocol, target config.RemoteTarget) (show.RemoteProtocol, int, error) {
	switch protocol {
	case show.RemoteProtocolAuto:
		if target.ERCPort > 0 {
			return show.RemoteProtocolERC, target.ERCPort, nil
		}
		if target.OSCPort > 0 {
			return show.RemoteProtocolOSC, target.OSCPort, nil
		}
	case show.RemoteProtocolERC:
		if target.ERCPort > 0 {
			return show.RemoteProtocolERC, target.ERCPort, nil
		}
	case show.RemoteProtocolOSC:
		if target.OSCPort > 0 {
			return show.RemoteProtocolOSC, target.OSCPort, nil
		}
	default:
		return protocol, 0, fmt.Errorf("unknown remote protocol %d", protocol)
	}
	return protocol, 0, errors.New("the selected protocol has no configured port")
}

func targetLabel(target config.RemoteTarget) string {
	if target.Name != "" {
		return target.Name
	}
	return target.Host
}
