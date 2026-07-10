package remote

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

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
	defer conn.Close()
	_, err = conn.Write(payload)
	return err
}

type Dispatcher struct {
	settings settingsProvider
	sender   packetSender
}

func NewDispatcher(settings settingsProvider) *Dispatcher {
	return &Dispatcher{settings: settings, sender: udpSender{}}
}

func (d *Dispatcher) Dispatch(ctx context.Context, play show.RemotePlay, cue show.Cue) error {
	if play.Action == show.RemoteActionNone {
		return nil
	}
	settings := d.settings.Snapshot()
	resolved := resolvePlay(play, settings, cue.CueNumber)
	if len(settings.RemoteTargets) == 0 {
		return errors.New("no remote control targets are configured")
	}

	var dispatchErrors []error
	for _, target := range settings.RemoteTargets {
		protocol, port, err := selectTransport(play.Protocol, target)
		if err != nil {
			dispatchErrors = append(dispatchErrors, fmt.Errorf("%s: %w", targetLabel(target), err))
			continue
		}
		payload, err := buildPayload(protocol, resolved)
		if err == nil {
			err = d.sender.Send(ctx, target.Host, port, payload)
		}
		if err != nil {
			dispatchErrors = append(dispatchErrors, fmt.Errorf("%s: %w", targetLabel(target), err))
		}
	}
	return errors.Join(dispatchErrors...)
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
