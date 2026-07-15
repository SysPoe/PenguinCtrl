package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

type preflightSnapshot struct {
	Key        [sha256.Size]byte
	ShowDigest [sha256.Size]byte
	Signature  [sha256.Size]byte
	Generated  time.Time
	Checks     []operatorlog.PreflightCheck
	SignError  string
}

const (
	packagingSpaceFactor  = 2
	packagingSpaceReserve = uint64(2 << 30)
)

// TODO(macro): preflightService mixes infrastructure (async cache, HMAC freshness gate) with
// domain checks (disk forecast, remote health) and cue-link graph reachability. Extract the
// signed gate/cache into preflight or playback as a reusable service; leave check builders as
// pure functions in that package, and move reachableCueIDs next to show link semantics.
type preflightService struct {
	mu      sync.RWMutex
	key     [sha256.Size]byte
	latest  preflightSnapshot
	running bool
	secret  [sha256.Size]byte
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func newPreflightService() (*preflightService, error) {
	ctx, cancel := context.WithCancel(context.Background())
	service := &preflightService{ctx: ctx, cancel: cancel}
	if _, err := rand.Read(service.secret[:]); err != nil {
		cancel()
		return nil, fmt.Errorf("generate preflight signing secret: %w", err)
	}
	return service, nil
}

func (s *preflightService) Close() { s.cancel(); s.wg.Wait() }

func (s *preflightService) Request(showState show.Show, settings config.Settings, audioWarning, videoWarning string, health []remote.TargetHealth, problems func(show.Cue) []show.CueProblem) []operatorlog.PreflightCheck {
	cues := showState.Cues
	showDigest, err := showState.Digest()
	if err != nil {
		return preflightEncodingFailure("show", err)
	}
	environment, err := json.Marshal(struct {
		Settings     config.Settings
		Audio, Video string
		Health       []remote.TargetHealth
	}{settings, audioWarning, videoWarning, health})
	if err != nil {
		return preflightEncodingFailure("environment", err)
	}
	keyInput := append(append([]byte(nil), showDigest[:]...), environment...)
	key := sha256.Sum256(keyInput)

	s.mu.Lock()
	s.key = key
	if s.latest.Key == key && time.Since(s.latest.Generated) < readinessRefreshInterval {
		checks := append([]operatorlog.PreflightCheck(nil), s.latest.Checks...)
		s.mu.Unlock()
		return checks
	}
	if !s.running {
		s.running = true
		s.wg.Add(1)
		go s.compute(key, showDigest, cues, settings, audioWarning, videoWarning, health, problems)
	}
	if s.latest.Key == key {
		checks := append([]operatorlog.PreflightCheck(nil), s.latest.Checks...)
		s.mu.Unlock()
		return checks
	}
	s.mu.Unlock()
	return []operatorlog.PreflightCheck{{Severity: operatorlog.ShowStopping, Code: "preflight.pending", Source: "Preflight", Message: "Preflight is computing a signed result for the current show"}}
}

func (s *preflightService) compute(key, showDigest [sha256.Size]byte, cues []show.Cue, settings config.Settings, audioWarning, videoWarning string, health []remote.TargetHealth, problems func(show.Cue) []show.CueProblem) {
	defer s.wg.Done()
	checks := buildPreflightWithProblems(cues, settings, audioWarning, videoWarning, problems)
	checks = append(checks, diskPreflight(cues, settings)...)
	checks = append(checks, remoteHealthPreflight(cues, settings, health)...)
	snapshot := preflightSnapshot{Key: key, ShowDigest: showDigest, Generated: time.Now().UTC(), Checks: checks}
	var err error
	snapshot.Signature, err = s.sign(snapshot)
	if err != nil {
		snapshot.SignError = err.Error()
	}
	s.mu.Lock()
	if s.ctx.Err() == nil && s.key == key {
		s.latest = snapshot
	}
	s.running = false
	s.mu.Unlock()
}

func (s *preflightService) Gate(current show.Show, selected show.Cue) error {
	digest, err := current.Digest()
	if err != nil {
		return fmt.Errorf("compute show identity for preflight gate: %w", err)
	}
	s.mu.RLock()
	snapshot, expected := s.latest, s.key
	s.mu.RUnlock()
	if snapshot.SignError != "" {
		return fmt.Errorf("sign preflight snapshot: %s", snapshot.SignError)
	}
	expectedSignature, err := s.sign(snapshot)
	if err != nil {
		return fmt.Errorf("verify preflight snapshot: %w", err)
	}
	if snapshot.Key != expected || snapshot.ShowDigest != digest || !hmac.Equal(snapshot.Signature[:], expectedSignature[:]) {
		return errors.New("signed preflight is stale or still computing")
	}
	reachable := reachableCueIDs(current.Cues, selected.ID)
	for _, check := range snapshot.Checks {
		if check.Severity == operatorlog.ShowStopping && preflightCheckApplies(check, reachable) {
			return fmt.Errorf("preflight blocked: %s: %s", check.Source, check.Message)
		}
	}
	return nil
}

func preflightCheckApplies(check operatorlog.PreflightCheck, reachable map[show.CueID]struct{}) bool {
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

func reachableCueIDs(cues []show.Cue, start show.CueID) map[show.CueID]struct{} {
	result := make(map[show.CueID]struct{})
	indexByID := make(map[show.CueID]int, len(cues))
	for index := range cues {
		indexByID[cues[index].ID] = index
	}
	index, ok := indexByID[start]
	for ok && index >= 0 && index < len(cues) {
		cue := cues[index]
		if _, seen := result[cue.ID]; seen {
			break
		}
		result[cue.ID] = struct{}{}
		switch cue.Link.Mode {
		case show.CueLinkStartPlay, show.CueLinkFadeInPlay, show.CueLinkFadeOutPlay, show.CueLinkEndPlay:
		default:
			return result
		}
		switch cue.Link.Target.Kind {
		case show.CueTargetNone, show.CueTargetNext:
			index++
		case show.CueTargetPrevious:
			index--
		case show.CueTargetCue:
			index, ok = indexByID[cue.Link.Target.CueID]
		default:
			return result
		}
	}
	return result
}

func (s *preflightService) sign(snapshot preflightSnapshot) ([sha256.Size]byte, error) {
	mac := hmac.New(sha256.New, s.secret[:])
	_, _ = mac.Write(snapshot.Key[:])
	_, _ = mac.Write(snapshot.ShowDigest[:])
	raw, err := json.Marshal(snapshot.Checks)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	_, _ = mac.Write(raw)
	var signature [sha256.Size]byte
	copy(signature[:], mac.Sum(nil))
	return signature, nil
}

func preflightEncodingFailure(subject string, err error) []operatorlog.PreflightCheck {
	return []operatorlog.PreflightCheck{{
		Severity: operatorlog.ShowStopping,
		Code:     "preflight.encode.failed",
		Source:   "Preflight",
		Message:  fmt.Sprintf("Could not encode the %s for a signed preflight result: %v", subject, err),
	}}
}

func diskPreflight(cues []show.Cue, settings config.Settings) []operatorlog.PreflightCheck {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return diskCaution(err.Error())
	}
	cacheRoot = filepath.Join(cacheRoot, "CuSus")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return diskCaution("Cache is not writable: " + err.Error())
	}
	probe, err := os.CreateTemp(cacheRoot, ".preflight-write-*")
	if err != nil {
		return diskCaution("Cache is not writable: " + err.Error())
	}
	probePath := probe.Name()
	_ = probe.Close()
	_ = os.Remove(probePath)
	var sourceBytes uint64
	for _, cue := range cues {
		for _, source := range cueMediaSources(cue, settings) {
			if info, statErr := os.Stat(source); statErr == nil && info.Mode().IsRegular() {
				sourceBytes += uint64(info.Size())
			}
		}
	}
	available, err := diskAvailableBytes(cacheRoot)
	if err != nil {
		return diskCaution("Free space could not be measured: " + err.Error())
	}
	required := sourceBytes*packagingSpaceFactor + packagingSpaceReserve
	if available < required {
		return diskCaution(fmt.Sprintf("Only %.1f GiB free; packaging/cache forecast requires %.1f GiB", float64(available)/(1<<30), float64(required)/(1<<30)))
	}
	return nil
}

func diskCaution(message string) []operatorlog.PreflightCheck {
	return []operatorlog.PreflightCheck{{Severity: operatorlog.Warning, Source: "Disk / cache", Message: message, Fingerprint: "disk:" + message}}
}

// TODO(macro): cueMediaSources is shared domain knowledge used by preflight disk checks, cache
// maintenance (window_loop), and redundancy fingerprints, yet lives under preflight_service.
// Move resolved media-source extraction onto show.Cue (or project media inventory) so packages
// do not depend on preflight for path enumeration.
func cueMediaSources(cue show.Cue, settings config.Settings) []string {
	var source string
	switch cue.Type {
	case show.CueTypeSound:
		if cue.Play.Sound != nil {
			source = config.Resolve(cue.Play.Sound.File, settings, cue.CueNumber)
		}
	case show.CueTypeVideo:
		if cue.Play.Video != nil {
			source = config.Resolve(cue.Play.Video.File, settings, cue.CueNumber)
		}
	case show.CueTypeImage:
		if cue.Play.Image != nil {
			source = config.Resolve(cue.Play.Image.File, settings, cue.CueNumber)
		}
	}
	if strings.TrimSpace(source) == "" || strings.Contains(source, "{") {
		return nil
	}
	return []string{source}
}

func remoteHealthPreflight(cues []show.Cue, settings config.Settings, health []remote.TargetHealth) []operatorlog.PreflightCheck {
	var affected []show.CueID
	for _, cue := range cues {
		if cue.Type == show.CueTypeRemote {
			affected = append(affected, cue.ID)
		}
	}
	if len(affected) == 0 {
		return nil
	}
	byName := make(map[string]remote.TargetHealth, len(health))
	for _, target := range health {
		byName[target.Name] = target
	}
	var checks []operatorlog.PreflightCheck
	for _, target := range settings.RemoteTargets {
		if target.HealthPort <= 0 {
			continue
		}
		name := target.Name
		if name == "" {
			name = target.Host
		}
		state, ok := byName[name]
		if !ok || !state.Known {
			checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "Remote health", Message: name + " has not completed a health probe", AffectedCues: affected})
		} else if !state.Reachable {
			checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "Remote health", Message: name + " is unreachable: " + state.LastError, AffectedCues: affected})
		}
	}
	return checks
}
