package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/health"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/preflight"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/redundancy"
	"github.com/syspoe/cusus/timecode"
)

const (
	readinessRefreshInterval = 2 * time.Second
	healthComponentCapacity  = 10
	cacheReserveWarnFactor   = 2
)

// TODO(macro): collectHealthComponents reaches across playback, media, timecode, redundancy,
// config, and document dirty state — a composition-root concern that forces package main to
// import every subsystem. Define a health.Provider/Collector interface implemented per package
// and register collectors at newApp so this fan-in stays at the wiring boundary only.
func collectHealthComponents(engine *playback.Engine, backend media.Backend, timeline *timecode.Service, spare *redundancy.Service, settings config.Settings, documentPath string, dirty bool) []health.Component {
	components := make([]health.Component, 0, healthComponentCapacity)
	components = append(components, engineHealth(engine), archiveHealth(documentPath, dirty), timecodeHealth(timeline), redundancyHealth(spare))
	components = append(components, audioHealth(engine, backend, settings)...)
	components = append(components, outputHealth(backend, settings)...)
	components = append(components, decoderHealth(engine)...)
	components = append(components, remoteTargetHealth(engine)...)
	components = append(components, diskHealth(settings))
	return components
}

func redundancyHealth(service *redundancy.Service) health.Component {
	if service == nil {
		return health.Component{ID: "redundancy", Kind: "redundancy", Name: "Warm spare", State: health.Failed, Summary: "Redundancy service is unavailable", Action: "Restart CuSus and inspect the Event Log"}
	}
	status := service.Status()
	component := health.Component{
		ID: "redundancy", Kind: "redundancy", Name: "Warm spare", State: health.Normal,
		Summary: status.Summary(), Action: "Open Settings > Warm-spare redundancy and follow the handoff status",
		Details: map[string]any{
			"role": status.Role, "nodeId": status.NodeID, "authority": status.Authority, "canIssueCommands": status.CanIssueCommands,
			"peerNodeId": status.PeerNodeID, "peerRole": status.PeerRole, "peerSeen": status.PeerSeen, "peerFresh": status.PeerFresh,
			"peerActive": status.PeerActive, "fingerprintsMatch": status.FingerprintsMatch, "interlockMatch": status.InterlockMatch,
			"lastPeerHeartbeat": status.LastPeerHeartbeat, "lastLocalHeartbeat": status.LastLocalHeartbeat, "interlockPath": status.InterlockPath,
		},
	}
	if status.Role == redundancy.RoleOff {
		return component
	}
	switch {
	case status.State == redundancy.StateFailed:
		component.State, component.Action = health.Failed, "Correct redundancy addresses, key, and shared interlock path, then save settings"
	case status.State == redundancy.StateMismatch:
		component.State, component.Action = health.Failed, "Load the identical verified show/media and align audio, display, remote, variable, and timecode routing on both nodes"
	case status.Authority && !status.CanIssueCommands:
		component.State, component.Action = health.Failed, "Keep GO stopped until both nodes report matching production fingerprints"
	case !status.Authority && (!status.PeerFresh || !status.PeerActive):
		component.State, component.Action = health.Failed, "Verify the active node; use Take command authority only after lease expiry or planned release"
	case status.Authority && !status.PeerFresh:
		component.State, component.Action = health.Degraded, "Restore or replace the warm spare while this node retains the shared command interlock"
	}
	return component
}

func timecodeHealth(service *timecode.Service) health.Component {
	if service == nil {
		return health.Component{ID: "timecode", Kind: "timecode", Name: "Master timeline", State: health.Failed, Summary: "Timecode service is unavailable", Action: "Restart CuSus and inspect the Event Log"}
	}
	status := service.Coordinator().Status()
	component := health.Component{ID: "timecode", Kind: "timecode", Name: "Master timeline", State: health.Normal, Summary: "Manual cue stacks use the internal monotonic timeline", Action: "Open Settings > External timecode and verify source, rate, and discontinuity policy"}
	if err := service.LastError(); err != nil {
		component.State, component.Summary = health.Failed, "External timecode input failed: "+err.Error()
	} else if status.Source != timecode.SourceInternal {
		switch status.State {
		case timecode.StateDiscontinuity:
			component.State, component.Summary, component.Action = health.Failed, "External timecode jumped; timeline is held", "Verify the master position, then acknowledge and resync from the interruption banner"
		case timecode.StateChasing:
			component.State, component.Summary = health.Recovering, "Timeline is chasing an external discontinuity"
		case timecode.StateStopped:
			component.State, component.Summary = health.Recovering, "Waiting for external timecode"
		default:
			component.Summary = "External timecode is running"
		}
	}
	component.Details = map[string]any{"source": status.Source, "policy": status.Policy, "position": status.Position, "generation": status.Generation, "lastUpdate": status.LastUpdate, "jump": status.Discontinuity}
	return component
}

func engineHealth(engine *playback.Engine) health.Component {
	component := health.Component{ID: "engine", Kind: "engine", Name: "Playback engine", State: health.Normal, Summary: "Command coordinator is armed", Action: "Open Event Log for command and failure history"}
	if reason := engine.SafetyLatchReason(); reason != "" {
		component.State, component.Summary, component.Action = health.Failed, reason, "Acknowledge the interruption banner, verify outputs, then re-arm GO"
	} else if message := engine.LastError(); message != "" {
		component.State, component.Summary = health.Degraded, message
	}
	component.Details = map[string]any{"activeCues": len(engine.ActiveInstances()), "activeExecutions": len(engine.ActiveExecutions())}
	return component
}

func archiveHealth(path string, dirty bool) health.Component {
	component := health.Component{ID: "archive", Kind: "archive", Name: "Show archive", State: health.Normal, Summary: "Current show matches its durable checkpoint", Action: "Use Save or Save As to create a verified archive"}
	if dirty && strings.TrimSpace(path) == "" {
		component.State, component.Summary = health.Degraded, "Show has unsaved edits and has not been published to a .cusus archive"
	} else if dirty {
		component.State, component.Summary = health.Degraded, "Show has edits newer than the last verified archive"
	}
	component.Details = map[string]any{"path": path, "dirty": dirty}
	return component
}

func audioHealth(engine *playback.Engine, backend media.Backend, settings config.Settings) []health.Component {
	instances := engine.ActiveInstances()
	affected := func(endpoint string) []string {
		var result []string
		for _, instance := range instances {
			if instance.MediaType != "audio" && instance.MediaType != "video" {
				continue
			}
			selected, _, _ := config.AudioRoute(settings, instance.Preview)
			if selected == endpoint {
				result = append(result, instance.CueNumber)
			}
		}
		return result
	}
	metrics := backend.AudioMixerMetrics()
	result := make([]health.Component, 0, len(metrics)+1)
	if warning := backend.AudioDeviceWarning(); warning != "" {
		result = append(result, health.Component{ID: "audio-route", Kind: "audio", Name: "Audio routing", State: health.Failed, Summary: warning, Action: "Open Settings > Audio devices and select an available primary/backup route"})
	}
	for _, metric := range metrics {
		endpoint := metric.EndpointID
		name := endpoint
		if name == "" {
			name = "Windows default endpoint"
		}
		component := health.Component{ID: "audio-" + endpoint, Kind: "audio", Name: name, State: health.Normal, Summary: "Audio callback is running", Action: "Open Settings > Audio devices; STOP affected cues if recovery fails"}
		switch {
		case metric.Failed:
			component.State, component.Summary = health.Failed, "Endpoint recovery failed"
		case metric.Recovering:
			component.State, component.Summary = health.Recovering, "Endpoint stopped; bounded recovery is in progress"
		case metric.TotalUnderruns > 0:
			component.State, component.Summary = health.Degraded, fmt.Sprintf("Endpoint has %d audio underruns", metric.TotalUnderruns)
		}
		component.Details = map[string]any{"endpointId": endpoint, "affectedCues": affected(endpoint), "activeSources": metric.ActiveSources, "lastSuccessfulCallback": metric.LastCallback, "recoveryCount": metric.RecoveryCount, "underruns": metric.TotalUnderruns}
		result = append(result, component)
	}
	if len(result) == 0 {
		result = append(result, health.Component{ID: "audio-idle", Kind: "audio", Name: "Audio system", State: health.Normal, Summary: "No active endpoint mixer; configured routes are available", Action: "Play the pre-show test tone on every route"})
	}
	return result
}

func outputHealth(backend media.Backend, settings config.Settings) []health.Component {
	displays, err := backend.VideoDisplays()
	available := map[string]bool{"": true}
	for _, display := range displays {
		available[display.ID] = true
	}
	result := make([]health.Component, 0, len(settings.VideoOutputs))
	for _, output := range settings.VideoOutputs {
		component := health.Component{ID: "output-" + output.Stage, Kind: "output", Name: output.Stage, State: health.Normal, Summary: "Display mapping is available and confirmed", Action: "Open Settings > Video outputs, identify the display, and confirm mapping", Details: map[string]any{"displayId": output.DisplayID, "fullscreen": output.Fullscreen, "confirmed": output.DisplayConfirmed}}
		switch {
		case err != nil:
			component.State, component.Summary = health.Degraded, "Display topology could not be enumerated: "+err.Error()
		case !available[output.DisplayID]:
			component.State, component.Summary = health.Failed, "Assigned display is disconnected"
		case !output.DisplayConfirmed:
			component.State, component.Summary = health.Degraded, "Physical display mapping has not been operator-confirmed"
			component.Details["operatorConfirmationPending"] = true
		}
		result = append(result, component)
	}
	return result
}

func decoderHealth(engine *playback.Engine) []health.Component {
	var result []health.Component
	for _, instance := range engine.ActiveInstances() {
		if instance.MediaType != "audio" && instance.MediaType != "video" {
			continue
		}
		component := health.Component{ID: "decoder-" + instance.ID, Kind: "decoder", Name: "Cue " + instance.CueNumber, State: health.Normal, Summary: "Decoder is playing", Action: "STOP the cue, inspect Event Log, then retry or skip", Details: map[string]any{"instanceId": instance.ID, "loadState": instance.LoadState, "startLatencyMs": instance.StartLatencyMs}}
		switch instance.LoadState {
		case string(media.LoadFailed):
			component.State, component.Summary = health.Failed, "Decoder failed"
		case string(media.LoadLoading), string(media.LoadBuffering):
			component.State, component.Summary = health.Recovering, "Decoder is loading or buffering"
		}
		result = append(result, component)
	}
	if len(result) == 0 {
		result = append(result, health.Component{ID: "decoder-idle", Kind: "decoder", Name: "Media decoders", State: health.Normal, Summary: "No active decoder failures", Action: "Run full preflight before GO"})
	}
	return result
}

func remoteTargetHealth(engine *playback.Engine) []health.Component {
	states := engine.RemoteHealth()
	result := make([]health.Component, 0, len(states))
	for _, state := range states {
		component := health.Component{ID: "remote-" + state.Name, Kind: "remote", Name: state.Name, State: health.Normal, Summary: "Target is reachable", Action: "Check target power/network, address and acknowledgement relay", Details: map[string]any{"host": state.Host, "lastSuccess": state.LastSuccess, "roundTrip": state.RoundTrip, "acknowledged": state.Acknowledged}}
		switch {
		case !state.Known:
			component.State, component.Summary = health.Recovering, "Waiting for first target health probe"
		case !state.Reachable:
			component.State, component.Summary = health.Failed, "Target is unreachable: "+state.LastError
		case !state.Acknowledged:
			component.State, component.Summary = health.Degraded, "Target is reachable but delivery is unacknowledged"
		}
		result = append(result, component)
	}
	if len(result) == 0 {
		result = append(result, health.Component{ID: "remote-none", Kind: "remote", Name: "Remote targets", State: health.Normal, Summary: "No remote targets are configured", Action: "Configure targets before adding remote cues"})
	}
	return result
}

func diskHealth(settings config.Settings) health.Component {
	component := health.Component{ID: "disk-cache", Kind: "disk", Name: "Cache volume", State: health.Normal, Summary: "Free-space reserve is available", Action: "Close playback, clear unreferenced cache, or move shows to a volume with more free space"}
	root, err := os.UserCacheDir()
	if err == nil {
		root = filepath.Join(root, "CuSus")
		if err = os.MkdirAll(root, 0o755); err == nil {
			var available uint64
			available, err = project.AvailableBytes(root)
			reserve := uint64(settings.CacheReserveGB) << 30
			component.Details = map[string]any{"path": root, "availableBytes": available, "reserveBytes": reserve}
			if err == nil && available < reserve {
				component.State, component.Summary = health.Failed, "Free space is below the configured reserve"
			} else if err == nil && available < reserve*cacheReserveWarnFactor {
				component.State, component.Summary = health.Degraded, "Free space is approaching the configured reserve"
			}
		}
	}
	if err != nil {
		component.State, component.Summary = health.Degraded, "Free space could not be measured: "+err.Error()
	}
	return component
}

func healthPreflightChecks(snapshot health.Snapshot) []preflight.Check {
	var result []preflight.Check
	for _, component := range snapshot.Components {
		if component.State == health.Normal {
			continue
		}
		severity := healthPreflightSeverity(component)
		affectedCues, _ := component.Details["affectedCues"].([]string)
		message := component.State.String() + ": " + component.Summary
		if len(affectedCues) > 0 {
			message += "; affected cues " + strings.Join(affectedCues, ", ")
		}
		if callback, ok := component.Details["lastSuccessfulCallback"].(time.Time); ok && !callback.IsZero() {
			message += "; last callback " + callback.Format(time.RFC3339Nano)
		}
		result = append(result, preflight.Check{
			Severity: severity, Code: "health." + component.Kind + "." + component.ID,
			Source: "Health · " + component.Name, Message: message,
			Consequence: "The component is not in its normal show-ready state", Fix: component.Action,
			Fingerprint: "health:" + component.ID + ":" + component.State.String() + ":" + component.Summary,
		})
	}
	return result
}

func healthPreflightSeverity(component health.Component) operatorlog.Severity {
	affectedCues, _ := component.Details["affectedCues"].([]string)
	if component.State == health.Failed && (component.Kind == "engine" || component.Kind == "timecode" || len(affectedCues) > 0) {
		return operatorlog.ShowStopping
	}
	if pending, _ := component.Details["operatorConfirmationPending"].(bool); pending {
		return operatorlog.Info
	}
	return operatorlog.Warning
}

// operatorHealthState excludes health observations that preflight classifies as
// informational. They remain visible in preflight, but must not put the main
// operator banner into a degraded state when no action is required.
func operatorHealthState(snapshot health.Snapshot) health.State {
	overall := health.Normal
	for _, component := range snapshot.Components {
		if component.State == health.Normal || healthPreflightSeverity(component) == operatorlog.Info {
			continue
		}
		if component.State > overall {
			overall = component.State
		}
	}
	return overall
}
