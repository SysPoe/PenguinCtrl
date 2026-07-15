// Package redundancy coordinates command authority between primary and standby nodes.
package redundancy

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Role identifies a node's configured redundancy responsibility.
type Role string

// State summarizes a node's current redundancy readiness.
type State string

const (
	// RoleOff disables redundancy coordination.
	RoleOff Role = "off"
	// RolePrimary identifies the preferred command-authority node.
	RolePrimary Role = "primary"
	// RoleStandby identifies the warm-spare node.
	RoleStandby Role = "standby"

	// StateOff indicates redundancy is disabled.
	StateOff State = "off"
	// StateWaiting indicates the node is waiting for peer readiness.
	StateWaiting State = "waiting"
	// StateReady indicates the node is synchronized but does not own authority.
	StateReady State = "ready"
	// StateActive indicates the node owns command authority.
	StateActive State = "active"
	// StateMismatch indicates the peer fingerprints differ.
	StateMismatch State = "mismatch"
	// StateFailed indicates redundancy coordination encountered an error.
	StateFailed State = "failed"

	defaultHeartbeatInterval     = 500 * time.Millisecond
	minimumPeerTimeoutHeartbeats = 3
	defaultPeerTimeoutHeartbeats = 5
	minimumSharedKeyLength       = 16
)

// ErrInterlockBusy indicates another node owns the shared command interlock.
var ErrInterlockBusy = errors.New("redundancy interlock is already owned")

// Config describes peer transport, authentication, and fencing settings.
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

// Fingerprint identifies the show, media, and routing state used for command fencing.
type Fingerprint struct {
	Show    string `json:"show"`
	Media   string `json:"media"`
	Routing string `json:"routing"`
	Ready   bool   `json:"ready"`
}

// Complete reports whether all fingerprint components are populated and ready.
func (f Fingerprint) Complete() bool {
	return f.Ready && strings.TrimSpace(f.Show) != "" && strings.TrimSpace(f.Media) != "" && strings.TrimSpace(f.Routing) != ""
}

// Equal reports whether f and other describe identical readiness state.
func (f Fingerprint) Equal(other Fingerprint) bool {
	return f.Show == other.Show && f.Media == other.Media && f.Routing == other.Routing && f.Ready == other.Ready
}

// Status is an immutable snapshot of local and peer redundancy state.
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
		config.HeartbeatInterval = defaultHeartbeatInterval
	}
	if config.PeerTimeout < config.HeartbeatInterval*minimumPeerTimeoutHeartbeats {
		config.PeerTimeout = config.HeartbeatInterval * defaultPeerTimeoutHeartbeats
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
	if len(config.SharedKey) < minimumSharedKeyLength {
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
