package remote

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/show"
)

type settingsProvider interface {
	Snapshot() config.Settings
}

type packetSender interface {
	Send(context.Context, string, int, []byte) error
}

type udpSender struct{}

func (udpSender) Send(ctx context.Context, host string, port int, payload []byte) error {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", address)
	if err != nil {
		return err
	}
	// TODO(micro): Explicitly mark this UDP close as best effort or return its error when no earlier send error exists.
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
	}
	_, err = conn.Write(payload)
	return err
}

// TODO(macro): Separate protocol codecs/transports, dispatch success policy,
// and target-health monitoring behind small interfaces. Dispatcher currently
// owns all three concerns and their goroutines, so adding a protocol or health
// strategy expands the same lifecycle-sensitive type.
type Dispatcher struct {
	settings         settingsProvider
	sender           packetSender
	ctx              context.Context
	cancel           context.CancelFunc
	done             chan struct{}
	nextID           atomic.Uint64
	lastAcknowledged atomic.Bool
	healthMu         sync.RWMutex
	health           map[string]TargetHealth
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
	ctx, cancel := context.WithCancel(context.Background())
	dispatcher := &Dispatcher{settings: settings, sender: udpSender{}, ctx: ctx, cancel: cancel, done: make(chan struct{}), health: map[string]TargetHealth{}}
	go dispatcher.monitor()
	return dispatcher
}

func (d *Dispatcher) Close() {
	if d.cancel == nil {
		return
	}
	d.cancel()
	<-d.done
}

func (d *Dispatcher) Health() []TargetHealth {
	d.healthMu.RLock()
	defer d.healthMu.RUnlock()
	result := make([]TargetHealth, 0, len(d.health))
	for _, health := range d.health {
		result = append(result, health)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

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
		// TODO(micro): Remove this obsolete loop-variable copy; target is already distinct per iteration on the required Go version.
		target := target
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
	ctx, cancel := context.WithTimeout(parent, 750*time.Millisecond)
	defer cancel()
	acknowledged := false
	if target.AckPort > 0 {
		id := fmt.Sprintf("%d-%d", time.Now().UnixMilli(), d.nextID.Add(1))
		for attempt := 0; attempt < 3; attempt++ {
			err = sendAcknowledged(ctx, target.Host, target.AckPort, id, payload)
			if err == nil {
				acknowledged = true
				break
			}
			if ctx.Err() != nil {
				break
			}
		}
	} else {
		err = d.sender.Send(ctx, target.Host, port, payload)
	}
	d.recordHealth(target, err, acknowledged, time.Since(started))
	if err != nil {
		return protocol, false, fmt.Errorf("%s: %w", targetLabel(target), err)
	}
	return protocol, acknowledged, nil
}

func sendAcknowledged(ctx context.Context, host string, port int, id string, payload []byte) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	// TODO(micro): Explicitly mark this TCP close as best effort or combine it with the result.
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	request := struct {
		ID      string `json:"id"`
		Payload string `json:"payload"`
	}{ID: id, Payload: base64.StdEncoding.EncodeToString(payload)}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return err
	}
	response, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(response) != "ACK "+id {
		return fmt.Errorf("unexpected acknowledgement %q", strings.TrimSpace(response))
	}
	return nil
}

func (d *Dispatcher) recordHealth(target config.RemoteTarget, err error, acknowledged bool, roundTrip time.Duration) {
	key := targetLabel(target)
	d.healthMu.Lock()
	health := d.health[key]
	health.Name, health.Host, health.Known, health.LastChecked, health.RoundTrip = key, target.Host, true, time.Now(), roundTrip
	health.Acknowledged = acknowledged
	if err == nil {
		health.Reachable, health.LastSuccess, health.LastError, health.ConsecutiveFailures = true, time.Now(), "", 0
	} else {
		health.Reachable, health.LastError, health.ConsecutiveFailures = false, err.Error(), health.ConsecutiveFailures+1
	}
	d.health[key] = health
	d.healthMu.Unlock()
}

func (d *Dispatcher) monitor() {
	defer close(d.done)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		d.probeTargets()
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) probeTargets() {
	var probes sync.WaitGroup
	for _, target := range d.settings.Snapshot().RemoteTargets {
		if target.HealthPort <= 0 {
			continue
		}
		// TODO(micro): Remove this obsolete loop-variable copy; the goroutine can capture the per-iteration target directly.
		target := target
		probes.Add(1)
		go func() {
			defer probes.Done()
			started := time.Now()
			ctx, cancel := context.WithTimeout(d.ctx, 500*time.Millisecond)
			defer cancel()
			conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(target.Host, strconv.Itoa(target.HealthPort)))
			if err == nil {
				_ = conn.Close()
			}
			d.recordProbeHealth(target, err, time.Since(started))
		}()
	}
	probes.Wait()
}

func (d *Dispatcher) recordProbeHealth(target config.RemoteTarget, err error, roundTrip time.Duration) {
	key := targetLabel(target)
	d.healthMu.Lock()
	health := d.health[key]
	health.Name, health.Host, health.Known, health.LastChecked, health.RoundTrip = key, target.Host, true, time.Now(), roundTrip
	if err == nil {
		health.Reachable, health.LastSuccess, health.LastError, health.ConsecutiveFailures = true, time.Now(), "", 0
	} else {
		health.Reachable, health.LastError, health.ConsecutiveFailures = false, err.Error(), health.ConsecutiveFailures+1
	}
	d.health[key] = health
	d.healthMu.Unlock()
}

func resolvePlay(play show.RemotePlay, settings config.Settings, cueNumber string) show.RemotePlay {
	play.Playback = config.Resolve(play.Playback, settings, cueNumber)
	play.CueNumber = config.Resolve(play.CueNumber, settings, cueNumber)
	play.Level = config.Resolve(play.Level, settings, cueNumber)
	play.Custom = config.Resolve(play.Custom, settings, cueNumber)
	for i := range play.Values {
		play.Values[i].Value = config.Resolve(play.Values[i].Value, settings, cueNumber)
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

func buildPayload(protocol show.RemoteProtocol, play show.RemotePlay) ([]byte, error) {
	if protocol == show.RemoteProtocolERC {
		command, err := buildERCCommand(play)
		return []byte(command), err
	}
	address, values, err := buildOSCMessage(play)
	if err != nil {
		return nil, err
	}
	return encodeOSC(address, values)
}

func buildERCCommand(play show.RemotePlay) (string, error) {
	playback, err := normalizePlayback(play.Playback)
	if err != nil {
		return "", err
	}
	switch play.Action {
	case show.RemoteActionGo:
		return playback + "G", nil
	case show.RemoteActionGoto:
		cueNumber, err := normalizeCueNumber(play.CueNumber)
		if err != nil {
			return "", err
		}
		return playback + "," + cueNumber + "J", nil
	case show.RemoteActionBack:
		return playback + "S", nil
	case show.RemoteActionRelease:
		return playback + "R", nil
	case show.RemoteActionLevel:
		level, err := normalizeLevel(play.Level)
		if err != nil {
			return "", err
		}
		return playback + "," + level + "L", nil
	case show.RemoteActionActivate:
		return playback + "A", nil
	case show.RemoteActionFlash:
		level, err := normalizeLevel(play.Level)
		if err != nil {
			return "", err
		}
		if level == "0" {
			return playback + "U", nil
		}
		return playback + "T", nil
	case show.RemoteActionCustom:
		if strings.TrimSpace(play.Custom) == "" {
			return "", errors.New("custom ERC command is empty")
		}
		return play.Custom, nil
	default:
		return "", fmt.Errorf("ERC does not support action %d", play.Action)
	}
}

func buildOSCMessage(play show.RemotePlay) (string, []show.RemoteValue, error) {
	if play.Action == show.RemoteActionCustom {
		if strings.TrimSpace(play.Custom) == "" {
			return "", nil, errors.New("custom OSC address is empty")
		}
		return play.Custom, play.Values, nil
	}
	playback, err := normalizePlayback(play.Playback)
	if err != nil {
		return "", nil, err
	}
	one := []show.RemoteValue{{Type: show.RemoteValueInt, Value: "1"}}
	switch play.Action {
	case show.RemoteActionGo:
		return "/pb/" + playback + "/go", one, nil
	case show.RemoteActionGoto:
		cueNumber, err := normalizeCueNumber(play.CueNumber)
		if err != nil {
			return "", nil, err
		}
		return "/pb/" + playback + "/" + cueNumber, one, nil
	case show.RemoteActionRelease:
		return "/pb/" + playback + "/release", one, nil
	case show.RemoteActionLevel:
		level, err := normalizeLevel(play.Level)
		if err != nil {
			return "", nil, err
		}
		return "/pb/" + playback, []show.RemoteValue{{Type: show.RemoteValueInt, Value: level}}, nil
	case show.RemoteActionActivate:
		return "", nil, errors.New("OSC transport does not support Activate; configure an ERC port or choose ERC")
	case show.RemoteActionFlash:
		level, err := normalizeLevel(play.Level)
		if err != nil {
			return "", nil, err
		}
		value := "1"
		if level == "0" {
			value = "0"
		}
		return "/pb/" + playback + "/flash", []show.RemoteValue{{Type: show.RemoteValueInt, Value: value}}, nil
	case show.RemoteActionBack:
		return "", nil, errors.New("OSC transport does not support Back; configure an ERC port or choose ERC")
	default:
		return "", nil, fmt.Errorf("OSC does not support action %d", play.Action)
	}
}

func normalizePlayback(value string) (string, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return "", fmt.Errorf("invalid playback %q", value)
	}
	return strconv.Itoa(parsed), nil
}

func normalizeCueNumber(value string) (string, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) == 0 || len(parts) > 2 || parts[0] == "" {
		return "", fmt.Errorf("invalid cue number %q", value)
	}
	whole, err := strconv.Atoi(parts[0])
	if err != nil || whole < 0 || whole > 65536 {
		return "", fmt.Errorf("invalid cue number %q", value)
	}
	if len(parts) == 1 {
		return strconv.Itoa(whole), nil
	}
	if len(parts[1]) == 0 || len(parts[1]) > 2 {
		return "", fmt.Errorf("invalid cue number %q", value)
	}
	fraction, err := strconv.Atoi(parts[1])
	if err != nil || fraction < 0 {
		return "", fmt.Errorf("invalid cue number %q", value)
	}
	fractionText := strings.TrimRight(fmt.Sprintf("%-2s", parts[1]), "0 ")
	if fractionText == "" || fraction == 0 {
		return strconv.Itoa(whole), nil
	}
	return strconv.Itoa(whole) + "." + fractionText, nil
}

func normalizeLevel(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "100", nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || parsed > 100 {
		return "", fmt.Errorf("invalid level %q; expected 0..100", value)
	}
	return strconv.Itoa(int(parsed + 0.5)), nil
}

func targetLabel(target config.RemoteTarget) string {
	if target.Name != "" {
		return target.Name
	}
	return target.Host
}
