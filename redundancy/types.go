// TODO(micro): Add Go-style documentation for the exported redundancy types, constants, error, and summary methods.
package redundancy

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Role string
type State string

const (
	RoleOff     Role = "off"
	RolePrimary Role = "primary"
	RoleStandby Role = "standby"

	StateOff      State = "off"
	StateWaiting  State = "waiting"
	StateReady    State = "ready"
	StateActive   State = "active"
	StateMismatch State = "mismatch"
	StateFailed   State = "failed"
)

var ErrInterlockBusy = errors.New("redundancy interlock is already owned")

type Config struct {
	Role              Role
	NodeID            string
	ListenAddress     string
	PeerAddress       string
	SharedKey         string
	InterlockPath     string
	HeartbeatInterval time.Duration
	PeerTimeout       time.Duration
}

type Fingerprint struct {
	Show    string `json:"show"`
	Media   string `json:"media"`
	Routing string `json:"routing"`
	Ready   bool   `json:"ready"`
}

func (f Fingerprint) Complete() bool {
	return f.Ready && strings.TrimSpace(f.Show) != "" && strings.TrimSpace(f.Media) != "" && strings.TrimSpace(f.Routing) != ""
}

func (f Fingerprint) Equal(other Fingerprint) bool {
	return f.Show == other.Show && f.Media == other.Media && f.Routing == other.Routing && f.Ready == other.Ready
}

type Status struct {
	Role               Role
	State              State
	NodeID             string
	Authority          bool
	CanIssueCommands   bool
	Local              Fingerprint
	Peer               Fingerprint
	PeerNodeID         string
	PeerRole           Role
	PeerSeen           bool
	PeerFresh          bool
	PeerActive         bool
	FingerprintsMatch  bool
	InterlockMatch     bool
	LastPeerHeartbeat  time.Time
	LastLocalHeartbeat time.Time
	LastError          string
	InterlockPath      string
}

// TODO(macro): Summary() embeds operator-facing copy in the domain Status type
// (also consumed by health_service). Keep Status machine-readable fields only
// and move prose formatting to UI/health presentation so protocol/state changes
// do not require editing user-visible strings here.
func (s Status) Summary() string {
	if s.Role == RoleOff {
		return "Redundancy is disabled; this node has local command authority"
	}
	if s.LastError != "" && s.State == StateFailed {
		return "Redundancy failed: " + s.LastError
	}
	if s.Authority && s.CanIssueCommands {
		if s.PeerFresh {
			return fmt.Sprintf("%s owns command authority; peer %s is validated", titleRole(s.Role), s.PeerNodeID)
		}
		// TODO(micro): Use string concatenation for this fixed-prefix/suffix message instead of fmt.Sprintf.
		return fmt.Sprintf("%s owns command authority; validated peer heartbeat is stale", titleRole(s.Role))
	}
	if s.Authority {
		// TODO(micro): Use string concatenation for this fixed-prefix/suffix message instead of fmt.Sprintf.
		return fmt.Sprintf("%s owns the interlock but command issue is blocked until both nodes match", titleRole(s.Role))
	}
	if s.PeerActive && s.PeerFresh {
		return fmt.Sprintf("Warm spare ready; %s owns command authority", s.PeerNodeID)
	}
	if s.PeerSeen && !s.FingerprintsMatch {
		return "Warm-spare fingerprints do not match"
	}
	if s.PeerSeen && !s.InterlockMatch {
		return "Warm-spare nodes do not use the same interlock"
	}
	return "Waiting for a validated peer and command authority"
}

func titleRole(role Role) string {
	switch role {
	case RolePrimary:
		return "Primary"
	case RoleStandby:
		return "Standby"
	default:
		return "Node"
	}
}

func normalizeConfig(config Config) Config {
	switch config.Role {
	case RolePrimary, RoleStandby:
	default:
		config.Role = RoleOff
	}
	config.NodeID = strings.TrimSpace(config.NodeID)
	config.ListenAddress = strings.TrimSpace(config.ListenAddress)
	config.PeerAddress = strings.TrimSpace(config.PeerAddress)
	config.SharedKey = strings.TrimSpace(config.SharedKey)
	config.InterlockPath = strings.TrimSpace(config.InterlockPath)
	if config.InterlockPath != "" {
		if absolute, err := filepath.Abs(config.InterlockPath); err == nil {
			config.InterlockPath = filepath.Clean(absolute)
		}
	}
	if config.HeartbeatInterval <= 0 {
		// TODO(micro): name default heartbeat interval (500ms) and peer-timeout multipliers (3x/5x) as constants
		config.HeartbeatInterval = 500 * time.Millisecond
	}
	if config.PeerTimeout < config.HeartbeatInterval*3 {
		config.PeerTimeout = config.HeartbeatInterval * 5
	}
	return config
}

func validateConfig(config Config) error {
	if config.Role == RoleOff {
		return nil
	}
	var missing []string
	if config.NodeID == "" {
		missing = append(missing, "node ID")
	}
	if config.ListenAddress == "" {
		missing = append(missing, "listen address")
	}
	if config.PeerAddress == "" {
		missing = append(missing, "peer address")
	}
	// TODO(micro): name minimum shared-key length (16) as a constant
	if len(config.SharedKey) < 16 {
		missing = append(missing, "shared key (minimum 16 characters)")
	}
	if config.InterlockPath == "" {
		missing = append(missing, "shared interlock path")
	}
	if len(missing) > 0 {
		return fmt.Errorf("redundancy configuration requires %s", strings.Join(missing, ", "))
	}
	return nil
}
