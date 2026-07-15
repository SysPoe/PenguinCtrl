package redundancy

import (
	"errors"
	"fmt"
	"time"
)

type authorityEffect uint8

const (
	authorityNoEffect authorityEffect = iota
	authorityAcquire
	authorityRefresh
	authorityRelease
)

// authorityPolicy is the pure redundancy decision state. It records peer lease
// state and decides which interlock effect the Service adapter must perform.
// It deliberately contains no sockets, locks, clocks, or goroutines.
type authorityPolicy struct {
	role          Role
	nodeID        string
	peerTimeout   time.Duration
	fingerprint   Fingerprint
	interlockID   string
	authority     bool
	released      bool
	lastError     string
	peer          heartbeat
	peerSeen      bool
	lastPeer      time.Time
	peerSentNanos int64
}

func (p *authorityPolicy) configure(config Config, interlockID string) {
	p.role = config.Role
	p.nodeID = config.NodeID
	p.peerTimeout = config.PeerTimeout
	p.interlockID = interlockID
	p.authority = false
	p.released = false
	p.lastError = ""
	p.peer = heartbeat{}
	p.peerSeen = false
	p.lastPeer = time.Time{}
	p.peerSentNanos = 0
}

func (p *authorityPolicy) acceptPeer(message heartbeat, receivedAt time.Time) bool {
	if message.NodeID == "" || message.NodeID == p.nodeID {
		return false
	}
	expectedRole := RolePrimary
	if p.role == RolePrimary {
		expectedRole = RoleStandby
	}
	if message.Role != expectedRole {
		p.lastError = fmt.Sprintf("peer %s is configured as %s; expected %s", message.NodeID, message.Role, expectedRole)
		return false
	}
	if p.peerSeen {
		if message.BootID == p.peer.BootID && message.Sequence <= p.peer.Sequence {
			return false
		}
		if message.BootID != p.peer.BootID && message.SentUnixNano <= p.peerSentNanos {
			return false
		}
	}
	p.peer = message
	p.peerSeen = true
	p.lastPeer = receivedAt
	p.peerSentNanos = message.SentUnixNano
	p.lastError = ""
	return true
}

func (p *authorityPolicy) reconcile(now time.Time) authorityEffect {
	if p.role == RoleOff || !p.fingerprint.Complete() {
		if p.authority {
			return authorityRelease
		}
		return authorityNoEffect
	}
	if p.authority {
		if p.role == RoleStandby && p.peerFresh(now) && p.peer.Active {
			p.lastError = "split-brain heartbeat detected; standby command authority was fenced"
			return authorityRelease
		}
		return authorityRefresh
	}
	if p.role == RolePrimary && !p.released && !(p.peerFresh(now) && p.peer.Active) {
		return authorityAcquire
	}
	return authorityNoEffect
}

func (p *authorityPolicy) gate() error {
	if p.role == RoleOff {
		return nil
	}
	if p.lastError != "" {
		return errors.New(p.lastError)
	}
	match := p.peerSeen && p.fingerprint.Equal(p.peer.Fingerprint)
	interlockMatch := p.peerSeen && p.interlockID != "" && p.peer.InterlockID == p.interlockID
	if p.authority && p.fingerprint.Complete() && match && interlockMatch {
		return nil
	}
	if !p.authority {
		return errors.New("this node does not own the redundancy command interlock")
	}
	if !p.fingerprint.Complete() {
		return errors.New("local show, media, and routing fingerprint is not ready")
	}
	if !p.peerSeen {
		return errors.New("warm spare has not completed fingerprint validation")
	}
	if !interlockMatch {
		return errors.New("warm-spare nodes are configured with different interlocks")
	}
	return errors.New("warm-spare show, media, or routing fingerprints do not match")
}

func (p *authorityPolicy) requestTakeover(now time.Time) (authorityEffect, error) {
	if p.role == RoleOff {
		return authorityNoEffect, errors.New("redundancy is disabled")
	}
	if p.authority {
		return authorityNoEffect, nil
	}
	if !p.fingerprint.Complete() {
		return authorityNoEffect, errors.New("local show, media, and routing fingerprint is not ready")
	}
	if !p.peerSeen {
		return authorityNoEffect, errors.New("takeover requires a previously validated peer heartbeat")
	}
	if p.peer.InterlockID != p.interlockID {
		return authorityNoEffect, errors.New("takeover refused because the peer uses a different interlock")
	}
	if !p.fingerprint.Equal(p.peer.Fingerprint) {
		return authorityNoEffect, errors.New("takeover refused because show, media, or routing fingerprints differ")
	}
	if p.peerFresh(now) && p.peer.Active {
		return authorityNoEffect, errors.New("takeover refused while the peer still owns command authority; release it on the active node first")
	}
	p.released = false
	return authorityAcquire, nil
}

func (p *authorityPolicy) requestRelease() (authorityEffect, error) {
	if p.role == RoleOff {
		return authorityNoEffect, errors.New("redundancy is disabled")
	}
	p.released = true
	if p.authority {
		return authorityRelease, nil
	}
	return authorityNoEffect, nil
}

func (p *authorityPolicy) authorityAcquired() {
	p.authority = true
	p.lastError = ""
}

func (p *authorityPolicy) authorityReleased() {
	p.authority = false
}

func (p *authorityPolicy) fail(message string) {
	p.lastError = message
}

func (p *authorityPolicy) peerFresh(now time.Time) bool {
	return p.peerSeen && !p.lastPeer.IsZero() && now.Sub(p.lastPeer) <= p.peerTimeout
}

func (p *authorityPolicy) status(config Config, now, lastSent time.Time) Status {
	peerFresh := p.peerFresh(now)
	match := p.peerSeen && p.fingerprint.Equal(p.peer.Fingerprint)
	interlockMatch := p.peerSeen && p.interlockID != "" && p.peer.InterlockID == p.interlockID
	canIssue := p.role == RoleOff || (p.authority && p.fingerprint.Complete() && match && interlockMatch)
	state := StateWaiting
	switch {
	case p.role == RoleOff:
		state = StateOff
	case p.lastError != "":
		state = StateFailed
	case p.peerSeen && (!match || !interlockMatch):
		state = StateMismatch
	case p.authority:
		state = StateActive
	case peerFresh && p.peer.Active && match && interlockMatch:
		state = StateReady
	}
	return Status{
		Role: p.role, State: state, NodeID: p.nodeID, Authority: p.authority, CanIssueCommands: canIssue,
		Local: p.fingerprint, Peer: p.peer.Fingerprint, PeerNodeID: p.peer.NodeID, PeerRole: p.peer.Role,
		PeerSeen: p.peerSeen, PeerFresh: peerFresh, PeerActive: p.peer.Active, FingerprintsMatch: match,
		InterlockMatch: interlockMatch, LastPeerHeartbeat: p.lastPeer, LastLocalHeartbeat: lastSent,
		LastError: p.lastError, InterlockPath: config.InterlockPath,
	}
}
