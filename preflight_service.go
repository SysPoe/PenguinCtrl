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
}

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

func newPreflightService() *preflightService {
	ctx, cancel := context.WithCancel(context.Background())
	service := &preflightService{ctx: ctx, cancel: cancel}
	// TODO(micro): Handle rand.Read failure rather than silently continuing with a predictable all-zero secret.
	_, _ = rand.Read(service.secret[:])
	return service
}

func (s *preflightService) Close() { s.cancel(); s.wg.Wait() }

func (s *preflightService) Request(showState show.Show, settings config.Settings, audioWarning, videoWarning string, health []remote.TargetHealth, problems func(show.Cue) []show.CueProblem) []operatorlog.PreflightCheck {
	cues := showState.Cues
	showDigest := showDigest(showState)
	environment, _ := json.Marshal(struct {
		Settings     config.Settings
		Audio, Video string
		Health       []remote.TargetHealth
	}{settings, audioWarning, videoWarning, health})
	keyInput := append(append([]byte(nil), showDigest[:]...), environment...)
	key := sha256.Sum256(keyInput)

	s.mu.Lock()
	s.key = key
	if s.latest.Key == key && time.Since(s.latest.Generated) < 2*time.Second {
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
	snapshot.Signature = s.sign(snapshot)
	s.mu.Lock()
	if s.ctx.Err() == nil && s.key == key {
		s.latest = snapshot
	}
	s.running = false
	s.mu.Unlock()
}

func (s *preflightService) Gate(current show.Show, selected show.Cue) error {
	digest := showDigest(current)
	s.mu.RLock()
	snapshot, expected := s.latest, s.key
	s.mu.RUnlock()
	expectedSignature := s.sign(snapshot)
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

func (s *preflightService) sign(snapshot preflightSnapshot) [sha256.Size]byte {
	mac := hmac.New(sha256.New, s.secret[:])
	mac.Write(snapshot.Key[:])
	mac.Write(snapshot.ShowDigest[:])
	raw, _ := json.Marshal(snapshot.Checks)
	mac.Write(raw)
	var signature [sha256.Size]byte
	copy(signature[:], mac.Sum(nil))
	return signature
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
	required := sourceBytes*2 + 2<<30
	if available < required {
		return diskCaution(fmt.Sprintf("Only %.1f GiB free; packaging/cache forecast requires %.1f GiB", float64(available)/(1<<30), float64(required)/(1<<30)))
	}
	return nil
}

func diskCaution(message string) []operatorlog.PreflightCheck {
	return []operatorlog.PreflightCheck{{Severity: operatorlog.Warning, Source: "Disk / cache", Message: message, Fingerprint: "disk:" + message}}
}

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
	hasRemote := false
	var affected []show.CueID
	for _, cue := range cues {
		hasRemote = hasRemote || cue.Type == show.CueTypeRemote
		if cue.Type == show.CueTypeRemote {
			affected = append(affected, cue.ID)
		}
	}
	if !hasRemote {
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
