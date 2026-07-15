// Package preflight owns signed readiness snapshots and GO admission checks.
package preflight

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

// Builder assembles pure readiness checks for a service refresh.
type Builder func(show.Show, config.Settings, string, string, []remote.TargetHealth, func(show.Cue) []show.CueProblem) []operatorlog.PreflightCheck

type snapshot struct {
	key        [sha256.Size]byte
	showDigest [sha256.Size]byte
	signature  [sha256.Size]byte
	generated  time.Time
	checks     []operatorlog.PreflightCheck
	signError  string
}

// Service asynchronously caches and signs readiness checks for the current
// show and machine environment.
type Service struct {
	mu              sync.RWMutex
	key             [sha256.Size]byte
	latest          snapshot
	running         bool
	secret          [sha256.Size]byte
	refreshInterval time.Duration
	builder         Builder
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// NewService constructs a signed preflight service using builder for domain
// checks. refreshInterval controls how long an identical result remains fresh.
func NewService(refreshInterval time.Duration, builder Builder) (*Service, error) {
	if refreshInterval <= 0 {
		refreshInterval = time.Second
	}
	if builder == nil {
		return nil, errors.New("preflight builder is required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{ctx: ctx, cancel: cancel, refreshInterval: refreshInterval, builder: builder}
	if _, err := rand.Read(service.secret[:]); err != nil {
		cancel()
		return nil, fmt.Errorf("generate preflight signing secret: %w", err)
	}
	return service, nil
}

// Close stops refresh work and waits for an in-flight builder.
func (s *Service) Close() { s.cancel(); s.wg.Wait() }

// Request returns the current checks or a fail-closed pending check while a
// changed show/environment snapshot is rebuilt.
func (s *Service) Request(showState show.Show, settings config.Settings, audioWarning, videoWarning string, health []remote.TargetHealth, problems func(show.Cue) []show.CueProblem) []operatorlog.PreflightCheck {
	showDigest, err := showState.Digest()
	if err != nil {
		return encodingFailure("show", err)
	}
	environment, err := json.Marshal(struct {
		Settings     config.Settings
		Audio, Video string
		Health       []remote.TargetHealth
	}{settings, audioWarning, videoWarning, health})
	if err != nil {
		return encodingFailure("environment", err)
	}
	keyInput := append(append([]byte(nil), showDigest[:]...), environment...)
	key := sha256.Sum256(keyInput)

	s.mu.Lock()
	s.key = key
	if s.latest.key == key && time.Since(s.latest.generated) < s.refreshInterval {
		checks := append([]operatorlog.PreflightCheck(nil), s.latest.checks...)
		s.mu.Unlock()
		return checks
	}
	if !s.running {
		s.running = true
		s.wg.Add(1)
		go s.compute(key, showDigest, showState, settings, audioWarning, videoWarning, health, problems)
	}
	if s.latest.key == key {
		checks := append([]operatorlog.PreflightCheck(nil), s.latest.checks...)
		s.mu.Unlock()
		return checks
	}
	s.mu.Unlock()
	return []operatorlog.PreflightCheck{{Severity: operatorlog.ShowStopping, Code: "preflight.pending", Source: "Preflight", Message: "Preflight is computing a signed result for the current show"}}
}

func (s *Service) compute(key, showDigest [sha256.Size]byte, showState show.Show, settings config.Settings, audioWarning, videoWarning string, health []remote.TargetHealth, problems func(show.Cue) []show.CueProblem) {
	defer s.wg.Done()
	checks := s.builder(showState, settings, audioWarning, videoWarning, health, problems)
	result := snapshot{key: key, showDigest: showDigest, generated: time.Now().UTC(), checks: checks}
	var err error
	result.signature, err = s.sign(result)
	if err != nil {
		result.signError = err.Error()
	}
	s.mu.Lock()
	if s.ctx.Err() == nil && s.key == key {
		s.latest = result
	}
	s.running = false
	s.mu.Unlock()
}

// Gate verifies the signed snapshot and rejects show-stopping checks reachable
// from selected through automatic play links.
func (s *Service) Gate(current show.Show, selected show.Cue) error {
	digest, err := current.Digest()
	if err != nil {
		return fmt.Errorf("compute show identity for preflight gate: %w", err)
	}
	s.mu.RLock()
	currentSnapshot, expected := s.latest, s.key
	s.mu.RUnlock()
	if currentSnapshot.signError != "" {
		return fmt.Errorf("sign preflight snapshot: %s", currentSnapshot.signError)
	}
	expectedSignature, err := s.sign(currentSnapshot)
	if err != nil {
		return fmt.Errorf("verify preflight snapshot: %w", err)
	}
	if currentSnapshot.key != expected || currentSnapshot.showDigest != digest || !hmac.Equal(currentSnapshot.signature[:], expectedSignature[:]) {
		return errors.New("signed preflight is stale or still computing")
	}
	reachable := show.ReachableCueIDs(current.Cues, selected.ID)
	for _, check := range currentSnapshot.checks {
		if check.Severity == operatorlog.ShowStopping && checkApplies(check, reachable) {
			return fmt.Errorf("preflight blocked: %s: %s", check.Source, check.Message)
		}
	}
	return nil
}

func checkApplies(check operatorlog.PreflightCheck, reachable map[show.CueID]struct{}) bool {
	if check.CueID != (show.CueID{}) {
		_, ok := reachable[check.CueID]
		return ok
	}
	if len(check.AffectedCues) == 0 {
		return true
	}
	for _, cueID := range check.AffectedCues {
		if _, ok := reachable[cueID]; ok {
			return true
		}
	}
	return false
}

func (s *Service) sign(current snapshot) ([sha256.Size]byte, error) {
	mac := hmac.New(sha256.New, s.secret[:])
	_, _ = mac.Write(current.key[:])
	_, _ = mac.Write(current.showDigest[:])
	raw, err := json.Marshal(current.checks)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	_, _ = mac.Write(raw)
	var signature [sha256.Size]byte
	copy(signature[:], mac.Sum(nil))
	return signature, nil
}

func encodingFailure(subject string, err error) []operatorlog.PreflightCheck {
	return []operatorlog.PreflightCheck{{
		Severity: operatorlog.ShowStopping,
		Code:     "preflight.encode.failed",
		Source:   "Preflight",
		Message:  fmt.Sprintf("Could not encode the %s for a signed preflight result: %v", subject, err),
	}}
}
