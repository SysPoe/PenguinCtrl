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

func (s *preflightService) Gate(current show.Show) error {
	digest := showDigest(current)
	s.mu.RLock()
	snapshot, expected := s.latest, s.key
	s.mu.RUnlock()
	expectedSignature := s.sign(snapshot)
	if snapshot.Key != expected || snapshot.ShowDigest != digest || !hmac.Equal(snapshot.Signature[:], expectedSignature[:]) {
		return errors.New("signed preflight is stale or still computing")
	}
	for _, check := range snapshot.Checks {
		if check.Severity == operatorlog.ShowStopping {
			return fmt.Errorf("preflight blocked: %s: %s", check.Source, check.Message)
		}
	}
	return nil
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
		return []operatorlog.PreflightCheck{{Severity: operatorlog.ShowStopping, Source: "Disk / cache", Message: err.Error()}}
	}
	cacheRoot = filepath.Join(cacheRoot, "CuSus")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return []operatorlog.PreflightCheck{{Severity: operatorlog.ShowStopping, Source: "Disk / cache", Message: "Cache is not writable: " + err.Error()}}
	}
	probe, err := os.CreateTemp(cacheRoot, ".preflight-write-*")
	if err != nil {
		return []operatorlog.PreflightCheck{{Severity: operatorlog.ShowStopping, Source: "Disk / cache", Message: "Cache is not writable: " + err.Error()}}
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
		return []operatorlog.PreflightCheck{{Severity: operatorlog.Warning, Source: "Disk / cache", Message: "Free space could not be measured: " + err.Error()}}
	}
	required := sourceBytes*2 + 2<<30
	if available < required {
		return []operatorlog.PreflightCheck{{Severity: operatorlog.ShowStopping, Source: "Disk / cache", Message: fmt.Sprintf("Only %.1f GiB free; packaging/cache forecast requires %.1f GiB", float64(available)/(1<<30), float64(required)/(1<<30))}}
	}
	return nil
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
	for _, cue := range cues {
		hasRemote = hasRemote || cue.Type == show.CueTypeRemote
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
			checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "Remote health", Message: name + " has not completed a health probe"})
		} else if !state.Reachable {
			checks = append(checks, operatorlog.PreflightCheck{Severity: operatorlog.ShowStopping, Source: "Remote health", Message: name + " is unreachable: " + state.LastError})
		}
	}
	return checks
}
