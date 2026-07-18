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
	"github.com/syspoe/cusus/show"
)

// Builder assembles show/settings checks for a service refresh. Runtime
// observations are supplied independently through RuntimeReadiness.
type Builder func(show.Show, config.Settings, func(show.Cue) []show.CueProblem) []Check

type staticSnapshot struct {
	key        [sha256.Size]byte
	showDigest [sha256.Size]byte
	generated  time.Time
	checks     []Check
}

type preparedRuntime struct {
	checks    []Check
	expiresAt time.Time
}

type snapshot struct {
	key            [sha256.Size]byte
	showDigest     [sha256.Size]byte
	signature      [sha256.Size]byte
	runtimeExpires time.Time
	checks         []Check
	signError      string
}

// Service asynchronously caches and signs readiness checks for the current
// show and machine environment.
type Service struct {
	mu                    sync.RWMutex
	key                   [sha256.Size]byte
	staticKey             [sha256.Size]byte
	runtime               preparedRuntime
	static                staticSnapshot
	latest                snapshot
	running               bool
	secret                [sha256.Size]byte
	staticRefreshInterval time.Duration
	builder               Builder
	ctx                   context.Context
	cancel                context.CancelFunc
	wg                    sync.WaitGroup
}

// NewService constructs a signed preflight service using builder for
// show/settings analysis. staticRefreshInterval controls only that analysis;
// runtime readiness has its own observation timestamp and freshness bound.
func NewService(staticRefreshInterval time.Duration, builder Builder) (*Service, error) {
	if staticRefreshInterval <= 0 {
		staticRefreshInterval = time.Second
	}
	if builder == nil {
		return nil, errors.New("preflight builder is required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{ctx: ctx, cancel: cancel, staticRefreshInterval: staticRefreshInterval, builder: builder}
	if _, err := rand.Read(service.secret[:]); err != nil {
		cancel()
		return nil, fmt.Errorf("generate preflight signing secret: %w", err)
	}
	return service, nil
}

// Close stops refresh work and waits for an in-flight builder.
func (s *Service) Close() { s.cancel(); s.wg.Wait() }

// Request returns one signed result containing cached show/settings analysis
// and the supplied point-in-time runtime observations. A changed runtime
// snapshot is signed immediately when the static analysis remains fresh.
func (s *Service) Request(showState show.Show, settings config.Settings, runtime RuntimeReadiness, problems func(show.Cue) []show.CueProblem) []Check {
	showDigest, err := showState.Digest()
	if err != nil {
		return encodingFailure("show", err)
	}
	settingsBytes, err := json.Marshal(settings)
	if err != nil {
		return encodingFailure("settings", err)
	}
	staticInput := make([]byte, 0, len(showDigest)+len(settingsBytes))
	staticInput = append(staticInput, showDigest[:]...)
	staticInput = append(staticInput, settingsBytes...)
	staticKey := sha256.Sum256(staticInput)
	prepared := preparedRuntime{checks: runtime.checksAt(time.Now()), expiresAt: runtime.expiresAt()}
	runtimeBytes, err := json.Marshal(struct {
		ObservedAt time.Time
		FreshFor   time.Duration
		Checks     []Check
	}{runtime.ObservedAt, runtime.FreshFor, prepared.checks})
	if err != nil {
		return encodingFailure("runtime readiness", err)
	}
	keyInput := make([]byte, 0, len(staticKey)+len(runtimeBytes))
	keyInput = append(keyInput, staticKey[:]...)
	keyInput = append(keyInput, runtimeBytes...)
	key := sha256.Sum256(keyInput)

	s.mu.Lock()
	s.key = key
	s.staticKey = staticKey
	s.runtime = prepared
	if s.static.key == staticKey && time.Since(s.static.generated) < s.staticRefreshInterval {
		s.composeLocked(key, s.static, prepared)
		checks := append([]Check(nil), s.latest.checks...)
		s.mu.Unlock()
		return checks
	}
	if !s.running {
		s.running = true
		s.wg.Add(1)
		go s.compute(staticKey, showDigest, showState, settings, problems)
	}
	if s.latest.key == key {
		checks := append([]Check(nil), s.latest.checks...)
		s.mu.Unlock()
		return checks
	}
	s.mu.Unlock()
	return []Check{{Severity: operatorlog.ShowStopping, Code: "preflight.pending", Source: "Preflight", Message: "Preflight is computing a signed result for the current show"}}
}

func (s *Service) compute(key, showDigest [sha256.Size]byte, showState show.Show, settings config.Settings, problems func(show.Cue) []show.CueProblem) {
	defer s.wg.Done()
	result := staticSnapshot{key: key, showDigest: showDigest, generated: time.Now().UTC(), checks: s.builder(showState, settings, problems)}
	s.mu.Lock()
	if s.ctx.Err() == nil {
		s.static = result
		if s.staticKey == key {
			s.composeLocked(s.key, result, s.runtime)
		}
	}
	s.running = false
	s.mu.Unlock()
}

func (s *Service) composeLocked(key [sha256.Size]byte, static staticSnapshot, runtime preparedRuntime) {
	checks := make([]Check, 0, len(static.checks)+len(runtime.checks))
	checks = append(checks, static.checks...)
	checks = append(checks, runtime.checks...)
	result := snapshot{key: key, showDigest: static.showDigest, runtimeExpires: runtime.expiresAt, checks: checks}
	var err error
	result.signature, err = s.sign(result)
	if err != nil {
		result.signError = err.Error()
	}
	s.latest = result
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
	if !currentSnapshot.runtimeExpires.IsZero() && !time.Now().Before(currentSnapshot.runtimeExpires) {
		return errors.New("preflight blocked: Runtime readiness: runtime health observations are stale")
	}
	reachable := show.ReachableCueIDs(current.Cues, selected.ID)
	for _, check := range currentSnapshot.checks {
		if check.Severity == operatorlog.ShowStopping && checkApplies(check, reachable) {
			return fmt.Errorf("preflight blocked: %s: %s", check.Source, check.Message)
		}
	}
	return nil
}

func checkApplies(check Check, reachable map[show.CueID]struct{}) bool {
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
	raw, err := json.Marshal(struct {
		Checks         []Check
		RuntimeExpires time.Time
	}{current.checks, current.runtimeExpires})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	_, _ = mac.Write(raw)
	var signature [sha256.Size]byte
	copy(signature[:], mac.Sum(nil))
	return signature, nil
}

func encodingFailure(subject string, err error) []Check {
	return []Check{{
		Severity: operatorlog.ShowStopping,
		Code:     "preflight.encode.failed",
		Source:   "Preflight",
		Message:  fmt.Sprintf("Could not encode the %s for a signed preflight result: %v", subject, err),
	}}
}
