package redundancy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type authorityInterlock interface {
	Touch([]byte) error
	Close() error
}

type interlockAdapter interface {
	Acquire(string) (authorityInterlock, error)
}

type systemInterlockAdapter struct{}

func (systemInterlockAdapter) Acquire(path string) (authorityInterlock, error) {
	return acquireSystemInterlock(path)
}

// Service coordinates peer heartbeats, fingerprint fencing, and command authority.
type Service struct {
	mu sync.RWMutex

	config    Config
	bootID    string
	sequence  uint64
	policy    authorityPolicy
	interlock interlockAdapter
	lock      authorityInterlock
	closed    bool
	lastSent  time.Time
	transport *peerTransport
}

// NewService constructs and configures a redundancy service.
func NewService(config Config) *Service {
	service := &Service{bootID: randomID(), interlock: systemInterlockAdapter{}}
	_ = service.Configure(config)
	return service
}

// Configure replaces the active redundancy configuration and peer transport.
func (s *Service) Configure(config Config) error {
	config = normalizeConfig(config)
	s.mu.RLock()
	unchanged := !s.closed && config == s.config && (config.Role == RoleOff || s.transport != nil)
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
	if err := s.releaseAuthorityLocked(); err != nil {
		s.policy.fail("release redundancy interlock before reconfigure: " + err.Error())
		return errors.New(s.policy.lastError)
	}
	s.config = config
	s.policy.configure(config, interlockIdentity(config.InterlockPath))
	if err := validateConfig(config); err != nil {
		s.policy.fail(err.Error())
		return err
	}
	if config.Role == RoleOff {
		return nil
	}
	transport, err := openPeerTransport(config, s.nextHeartbeat, s.receiveHeartbeat, s.failTransport)
	if err != nil {
		s.policy.fail(err.Error())
		return err
	}
	s.transport = transport
	return nil
}

// UpdateFingerprint replaces the local readiness fingerprint and reconciles authority.
func (s *Service) UpdateFingerprint(fingerprint Fingerprint) {
	s.mu.Lock()
	s.policy.fingerprint = fingerprint
	s.reconcileLocked(time.Now())
	s.mu.Unlock()
}

// Gate reports whether this node may currently issue commands.
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
	return s.policy.gate()
}

// RequestTakeover attempts to acquire command authority for this node.
func (s *Service) RequestTakeover() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	effect, err := s.policy.requestTakeover(time.Now())
	if err != nil || effect == authorityNoEffect {
		return err
	}
	if err = s.acquireAuthorityLocked(); err != nil {
		if errors.Is(err, ErrInterlockBusy) {
			return errors.New("takeover refused because the shared interlock remains owned")
		}
		s.policy.fail("acquire redundancy interlock: " + err.Error())
		return errors.New(s.policy.lastError)
	}
	return nil
}

// ReleaseAuthority relinquishes this node's command interlock.
func (s *Service) ReleaseAuthority() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	effect, err := s.policy.requestRelease()
	if err != nil || effect == authorityNoEffect {
		return err
	}
	return s.releaseAuthorityLocked()
}

// Status returns an immutable snapshot of current redundancy state.
func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy.status(s.config, time.Now(), s.lastSent)
}

// Close stops peer transport and releases any owned authority.
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
	transport := s.transport
	s.transport = nil
	s.mu.Unlock()
	transport.close()
}

func (s *Service) nextHeartbeat(now time.Time) heartbeat {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileLocked(now)
	s.sequence++
	message := heartbeat{
		Version: protocolVersion, NodeID: s.config.NodeID, Role: s.config.Role, BootID: s.bootID,
		Sequence: s.sequence, SentUnixNano: now.UnixNano(), Active: s.policy.authority,
		Fingerprint: s.policy.fingerprint, InterlockID: s.policy.interlockID,
	}
	s.lastSent = now
	return message
}

func (s *Service) receiveHeartbeat(message heartbeat, receivedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.policy.acceptPeer(message, receivedAt) {
		s.reconcileLocked(receivedAt)
	}
}

func (s *Service) failTransport(message string) {
	s.mu.Lock()
	s.policy.fail(message)
	s.mu.Unlock()
}

func (s *Service) reconcileLocked(now time.Time) {
	switch s.policy.reconcile(now) {
	case authorityNoEffect:
		return
	case authorityRelease:
		_ = s.releaseAuthorityLocked()
		return
	case authorityRefresh:
		record, err := json.Marshal(struct {
			NodeID      string      `json:"nodeId"`
			BootID      string      `json:"bootId"`
			Updated     time.Time   `json:"updated"`
			Fingerprint Fingerprint `json:"fingerprint"`
		}{s.config.NodeID, s.bootID, now.UTC(), s.policy.fingerprint})
		if err != nil {
			s.policy.fail("encode redundancy interlock record: " + err.Error())
			_ = s.releaseAuthorityLocked()
			return
		}
		if err = s.lock.Touch(append(record, '\n')); err != nil {
			s.policy.fail("refresh redundancy interlock: " + err.Error())
			_ = s.releaseAuthorityLocked()
		}
		return
	case authorityAcquire:
		if err := s.acquireAuthorityLocked(); err != nil && !errors.Is(err, ErrInterlockBusy) {
			s.policy.fail("acquire redundancy interlock: " + err.Error())
		}
	}
}

func (s *Service) acquireAuthorityLocked() error {
	if s.policy.authority {
		return nil
	}
	adapter := s.interlock
	if adapter == nil {
		adapter = systemInterlockAdapter{}
	}
	lock, err := adapter.Acquire(s.config.InterlockPath)
	if err != nil {
		return err
	}
	s.lock = lock
	s.policy.authorityAcquired()
	s.reconcileLocked(time.Now())
	if !s.policy.authority {
		if s.policy.lastError != "" {
			return errors.New(s.policy.lastError)
		}
		return errors.New("redundancy interlock ownership was lost")
	}
	return nil
}

func (s *Service) releaseAuthorityLocked() error {
	lock := s.lock
	s.lock = nil
	s.policy.authorityReleased()
	if lock != nil {
		return lock.Close()
	}
	return nil
}

func interlockIdentity(path string) string {
	if path == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToLower(path)))
	return hex.EncodeToString(sum[:])
}

func randomID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("boot-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)
}
