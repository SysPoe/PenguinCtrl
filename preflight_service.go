package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/preflight"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/remote"
	"github.com/syspoe/cusus/show"
)

const (
	packagingSpaceFactor  = 2
	packagingSpaceReserve = uint64(2 << 30)
)

func newPreflightService() (*preflight.Service, error) {
	return preflight.NewService(readinessRefreshInterval, func(showState show.Show, settings config.Settings, audioWarning, videoWarning string, health []remote.TargetHealth, problems func(show.Cue) []show.CueProblem) []operatorlog.PreflightCheck {
		checks := buildPreflightWithProblems(showState.Cues, settings, audioWarning, videoWarning, problems)
		checks = append(checks, diskPreflight(showState.Cues, settings)...)
		checks = append(checks, remoteHealthPreflight(showState.Cues, settings, health)...)
		return checks
	})
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
		for _, source := range project.ResolvedMediaSources(cue, settings) {
			if info, statErr := os.Stat(source); statErr == nil && info.Mode().IsRegular() {
				sourceBytes += uint64(info.Size())
			}
		}
	}
	available, err := project.AvailableBytes(cacheRoot)
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
