package redundancy

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

const protocolVersion = 1

type heartbeat struct {
	Version      int         `json:"version"`
	NodeID       string      `json:"nodeId"`
	Role         Role        `json:"role"`
	BootID       string      `json:"bootId"`
	Sequence     uint64      `json:"sequence"`
	SentUnixNano int64       `json:"sentUnixNano"`
	Active       bool        `json:"active"`
	Fingerprint  Fingerprint `json:"fingerprint"`
	InterlockID  string      `json:"interlockId"`
	Signature    string      `json:"signature,omitempty"`
}

// TODO(macro): Extract a pure authority/peer state machine from the UDP
// heartbeat transport and OS interlock ownership. Keeping protocol I/O,
// reconciliation policy, and takeover state under one mutex makes exhaustive
// failover testing and future transport changes unnecessarily coupled.
type Service struct {
	mu sync.RWMutex

	config      Config
	fingerprint Fingerprint
	bootID      string
	sequence    uint64
	interlockID string
	lock        *systemInterlock
	authority   bool
	released    bool
	closed      bool
	lastError   string
	lastSent    time.Time

	peer             heartbeat
	peerSeen         bool
	lastPeer         time.Time
	peerSentUnixNano int64

	cancel context.CancelFunc
	conn   *net.UDPConn
	wg     sync.WaitGroup
}

func NewService(config Config) *Service {
	service := &Service{bootID: randomID()}
	_ = service.Configure(config)
	return service
}

func (s *Service) Configure(config Config) error {
	config = normalizeConfig(config)
	s.mu.RLock()
	unchanged := !s.closed && config == s.config && (config.Role == RoleOff || s.conn != nil)
	s.mu.RUnlock()
	if unchanged {
		return nil
	}
	s.stopListener()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("redundancy service is closed")
	}
	s.releaseAuthorityLocked()
	s.config = config
	s.interlockID = interlockIdentity(config.InterlockPath)
	s.peer, s.peerSeen, s.lastPeer, s.peerSentUnixNano = heartbeat{}, false, time.Time{}, 0
	s.released, s.lastError = false, ""
	if err := validateConfig(config); err != nil {
		s.lastError = err.Error()
		return err
	}
	if config.Role == RoleOff {
		return nil
	}
	listenAddress, err := net.ResolveUDPAddr("udp", config.ListenAddress)
	if err != nil {
		s.lastError = "resolve redundancy listen address: " + err.Error()
		return errors.New(s.lastError)
	}
	peerAddress, err := net.ResolveUDPAddr("udp", config.PeerAddress)
	if err != nil {
		s.lastError = "resolve redundancy peer address: " + err.Error()
		return errors.New(s.lastError)
	}
	conn, err := net.ListenUDP("udp", listenAddress)
	if err != nil {
		s.lastError = "listen for redundancy heartbeats: " + err.Error()
		return errors.New(s.lastError)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.conn, s.cancel = conn, cancel
	s.wg.Add(1)
	go s.run(ctx, conn, peerAddress)
	return nil
}

func (s *Service) UpdateFingerprint(fingerprint Fingerprint) {
	s.mu.Lock()
	s.fingerprint = fingerprint
	s.reconcileLocked(time.Now())
	s.mu.Unlock()
}

func (s *Service) Gate() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gateLocked()
}

// WithAuthority keeps the command interlock owned for the complete action.
// Release, takeover, fingerprint changes, and heartbeat reconciliation wait
// until the action returns, closing the check-then-dispatch race for remote GO.
func (s *Service) WithAuthority(action func() error) error {
	if action == nil {
		return errors.New("authority action is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.gateLocked(); err != nil {
		return err
	}
	return action()
}

func (s *Service) gateLocked() error {
	if s.config.Role == RoleOff {
		return nil
	}
	if s.lastError != "" {
		return errors.New(s.lastError)
	}
	match := s.peerSeen && s.fingerprint.Equal(s.peer.Fingerprint)
	interlockMatch := s.peerSeen && s.interlockID != "" && s.peer.InterlockID == s.interlockID
	if s.authority && s.fingerprint.Complete() && match && interlockMatch {
		return nil
	}
	if !s.authority {
		return errors.New("this node does not own the redundancy command interlock")
	}
	if !s.fingerprint.Complete() {
		return errors.New("local show, media, and routing fingerprint is not ready")
	}
	if !s.peerSeen {
		return errors.New("warm spare has not completed fingerprint validation")
	}
	if !interlockMatch {
		return errors.New("warm-spare nodes are configured with different interlocks")
	}
	return errors.New("warm-spare show, media, or routing fingerprints do not match")
}

func (s *Service) RequestTakeover() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.config.Role == RoleOff {
		return errors.New("redundancy is disabled")
	}
	if s.authority {
		return nil
	}
	if !s.fingerprint.Complete() {
		return errors.New("local show, media, and routing fingerprint is not ready")
	}
	if !s.peerSeen {
		return errors.New("takeover requires a previously validated peer heartbeat")
	}
	if s.peer.InterlockID != s.interlockID {
		return errors.New("takeover refused because the peer uses a different interlock")
	}
	if !s.fingerprint.Equal(s.peer.Fingerprint) {
		return errors.New("takeover refused because show, media, or routing fingerprints differ")
	}
	if s.peerFreshLocked(time.Now()) && s.peer.Active {
		return errors.New("takeover refused while the peer still owns command authority; release it on the active node first")
	}
	s.released = false
	if err := s.acquireAuthorityLocked(); err != nil {
		if errors.Is(err, ErrInterlockBusy) {
			return errors.New("takeover refused because the shared interlock remains owned")
		}
		s.lastError = "acquire redundancy interlock: " + err.Error()
		return errors.New(s.lastError)
	}
	return nil
}

func (s *Service) ReleaseAuthority() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.config.Role == RoleOff {
		return errors.New("redundancy is disabled")
	}
	s.released = true
	return s.releaseAuthorityLocked()
}

func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	peerFresh := s.peerFreshLocked(now)
	match := s.peerSeen && s.fingerprint.Equal(s.peer.Fingerprint)
	interlockMatch := s.peerSeen && s.interlockID != "" && s.peer.InterlockID == s.interlockID
	canIssue := s.config.Role == RoleOff || (s.authority && s.fingerprint.Complete() && match && interlockMatch)
	state := StateWaiting
	switch {
	case s.config.Role == RoleOff:
		state = StateOff
	case s.lastError != "":
		state = StateFailed
	case s.peerSeen && (!match || !interlockMatch):
		state = StateMismatch
	case s.authority:
		state = StateActive
	case peerFresh && s.peer.Active && match && interlockMatch:
		state = StateReady
	}
	return Status{
		Role: s.config.Role, State: state, NodeID: s.config.NodeID, Authority: s.authority, CanIssueCommands: canIssue,
		Local: s.fingerprint, Peer: s.peer.Fingerprint, PeerNodeID: s.peer.NodeID, PeerRole: s.peer.Role,
		PeerSeen: s.peerSeen, PeerFresh: peerFresh, PeerActive: s.peer.Active, FingerprintsMatch: match,
		InterlockMatch: interlockMatch, LastPeerHeartbeat: s.lastPeer, LastLocalHeartbeat: s.lastSent,
		LastError: s.lastError, InterlockPath: s.config.InterlockPath,
	}
}

func (s *Service) Close() {
	s.stopListener()
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		_ = s.releaseAuthorityLocked()
	}
	s.mu.Unlock()
}

func (s *Service) stopListener() {
	s.mu.Lock()
	cancel, conn := s.cancel, s.conn
	s.cancel, s.conn = nil, nil
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Service) run(ctx context.Context, conn *net.UDPConn, peerAddress *net.UDPAddr) {
	defer s.wg.Done()
	nextHeartbeat := time.Now()
	buffer := make([]byte, 64<<10)
	for {
		s.mu.RLock()
		interval := s.config.HeartbeatInterval
		s.mu.RUnlock()
		if wait := time.Until(nextHeartbeat); wait <= 0 {
			s.sendHeartbeat(conn, peerAddress)
			nextHeartbeat = time.Now().Add(interval)
		}
		_ = conn.SetReadDeadline(nextHeartbeat)
		n, address, err := conn.ReadFromUDP(buffer)
		if err == nil {
			s.handleHeartbeat(address, peerAddress, buffer[:n])
			continue
		}
		if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
			return
		}
		if netError, ok := err.(net.Error); ok && netError.Timeout() {
			continue
		}
		s.mu.Lock()
		s.lastError = "receive redundancy heartbeat: " + err.Error()
		s.mu.Unlock()
	}
}

func (s *Service) sendHeartbeat(conn *net.UDPConn, peerAddress *net.UDPAddr) {
	s.mu.Lock()
	now := time.Now()
	s.reconcileLocked(now)
	s.sequence++
	message := heartbeat{
		Version: protocolVersion, NodeID: s.config.NodeID, Role: s.config.Role, BootID: s.bootID,
		Sequence: s.sequence, SentUnixNano: now.UnixNano(), Active: s.authority,
		Fingerprint: s.fingerprint, InterlockID: s.interlockID,
	}
	message.Signature = signHeartbeat(message, s.config.SharedKey)
	raw, err := json.Marshal(message)
	s.lastSent = now
	s.mu.Unlock()
	if err == nil {
		_, err = conn.WriteToUDP(raw, peerAddress)
	}
	if err != nil {
		s.mu.Lock()
		s.lastError = "send redundancy heartbeat: " + err.Error()
		s.mu.Unlock()
	}
}

func (s *Service) handleHeartbeat(address, expected *net.UDPAddr, raw []byte) {
	if !sameUDPAddress(address, expected) {
		return
	}
	var message heartbeat
	if err := json.Unmarshal(raw, &message); err != nil || message.Version != protocolVersion {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !verifyHeartbeat(message, s.config.SharedKey) || message.NodeID == "" || message.NodeID == s.config.NodeID {
		return
	}
	expectedRole := RolePrimary
	if s.config.Role == RolePrimary {
		expectedRole = RoleStandby
	}
	if message.Role != expectedRole {
		s.lastError = fmt.Sprintf("peer %s is configured as %s; expected %s", message.NodeID, message.Role, expectedRole)
		return
	}
	if s.peerSeen {
		if message.BootID == s.peer.BootID && message.Sequence <= s.peer.Sequence {
			return
		}
		if message.BootID != s.peer.BootID && message.SentUnixNano <= s.peerSentUnixNano {
			return
		}
	}
	s.peer, s.peerSeen, s.lastPeer, s.peerSentUnixNano = message, true, time.Now(), message.SentUnixNano
	s.lastError = ""
	s.reconcileLocked(s.lastPeer)
}

func (s *Service) reconcileLocked(now time.Time) {
	if s.config.Role == RoleOff || !s.fingerprint.Complete() {
		_ = s.releaseAuthorityLocked()
		return
	}
	if s.authority {
		if s.peerFreshLocked(now) && s.peer.Active && s.config.Role == RoleStandby {
			s.lastError = "split-brain heartbeat detected; standby command authority was fenced"
			_ = s.releaseAuthorityLocked()
			return
		}
		record, _ := json.Marshal(struct {
			NodeID      string      `json:"nodeId"`
			BootID      string      `json:"bootId"`
			Updated     time.Time   `json:"updated"`
			Fingerprint Fingerprint `json:"fingerprint"`
		}{s.config.NodeID, s.bootID, now.UTC(), s.fingerprint})
		if err := s.lock.Touch(append(record, '\n')); err != nil {
			s.lastError = "refresh redundancy interlock: " + err.Error()
			_ = s.releaseAuthorityLocked()
		}
		return
	}
	if s.config.Role == RolePrimary && !s.released && !(s.peerFreshLocked(now) && s.peer.Active) {
		if err := s.acquireAuthorityLocked(); err != nil && !errors.Is(err, ErrInterlockBusy) {
			s.lastError = "acquire redundancy interlock: " + err.Error()
		}
	}
}

func (s *Service) acquireAuthorityLocked() error {
	if s.authority {
		return nil
	}
	lock, err := acquireSystemInterlock(s.config.InterlockPath)
	if err != nil {
		return err
	}
	s.lock, s.authority = lock, true
	s.lastError = ""
	s.reconcileLocked(time.Now())
	if !s.authority {
		if s.lastError != "" {
			return errors.New(s.lastError)
		}
		return errors.New("redundancy interlock ownership was lost")
	}
	return nil
}

func (s *Service) releaseAuthorityLocked() error {
	lock := s.lock
	s.lock, s.authority = nil, false
	if lock != nil {
		return lock.Close()
	}
	return nil
}

func (s *Service) peerFreshLocked(now time.Time) bool {
	return s.peerSeen && !s.lastPeer.IsZero() && now.Sub(s.lastPeer) <= s.config.PeerTimeout
}

func interlockIdentity(path string) string {
	if path == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToLower(path)))
	return hex.EncodeToString(sum[:])
}

func signHeartbeat(message heartbeat, key string) string {
	message.Signature = ""
	raw, _ := json.Marshal(message)
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyHeartbeat(message heartbeat, key string) bool {
	expected, err := hex.DecodeString(signHeartbeat(message, key))
	if err != nil {
		return false
	}
	actual, err := hex.DecodeString(message.Signature)
	return err == nil && hmac.Equal(actual, expected)
}

func sameUDPAddress(actual, expected *net.UDPAddr) bool {
	if actual == nil || expected == nil || actual.Port != expected.Port {
		return false
	}
	return actual.IP.Equal(expected.IP)
}

func randomID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("boot-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)
}
