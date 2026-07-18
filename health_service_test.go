package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/health"
	"github.com/syspoe/cusus/media"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/redundancy"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/timecode"
)

type healthBackendStub struct {
	audioWarning string
	mixers       []media.AudioMixerMetrics
	displays     []media.VideoDisplay
	displayErr   error
}

func TestFailedRouteHealthIsWarningUntilAnAffectedCueIsIdentified(t *testing.T) {
	active := show.NewSoundCue()
	active.CueNumber = "12"
	snapshot := health.NewSnapshot([]health.Component{
		{ID: "unused", Kind: "output", Name: "Unused stage", State: health.Failed, Summary: "display disconnected", Details: map[string]any{}},
		{ID: "active", Kind: "audio", Name: "Playback route", State: health.Failed, Summary: "endpoint disconnected", Details: map[string]any{"affectedCues": []string{"12"}, "affectedCueIDs": []show.CueID{active.ID}}},
	})
	checks := healthPreflightChecks(snapshot, show.Show{Cues: []show.Cue{active}}, config.Defaults())
	severities := make(map[string]operatorlog.Severity, len(checks))
	for _, check := range checks {
		severities[check.Source] = check.Severity
	}
	if severities["Health · Unused stage"] != operatorlog.Warning || severities["Health · Playback route"] != operatorlog.ShowStopping {
		t.Fatalf("route health checks = %#v", checks)
	}
}

func TestMissingTimecodeServiceReportsFailedHealth(t *testing.T) {
	component := timecodeHealth(nil)
	if component.State != health.Failed || component.ID != "timecode" {
		t.Fatalf("timecode health = %+v", component)
	}
}

func (b *healthBackendStub) AudioDevices() ([]media.AudioDevice, error) { return nil, nil }
func (b *healthBackendStub) AudioDeviceWarning() string                 { return b.audioWarning }
func (b *healthBackendStub) AudioMixerMetrics() []media.AudioMixerMetrics {
	return append([]media.AudioMixerMetrics(nil), b.mixers...)
}
func (b *healthBackendStub) RefreshAudioDeviceStatus() {}
func (b *healthBackendStub) VideoDisplays() ([]media.VideoDisplay, error) {
	return append([]media.VideoDisplay(nil), b.displays...), b.displayErr
}
func (b *healthBackendStub) VideoOutputWarning() string { return "" }
func (b *healthBackendStub) RefreshVideoOutputStatus()  {}
func (b *healthBackendStub) EnsureOutputs([]string)     {}
func (b *healthBackendStub) SyncOutputs([]string)       {}
func (b *healthBackendStub) Close() error               { return nil }

func TestHealthComponentsExposeIdentityRecoveryAndAction(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	settings := store.Snapshot()
	settings.VideoOutputs = []config.VideoOutput{{Stage: "stage", DisplayID: "missing", DisplayConfirmed: true}}
	engine := playback.NewEngine(show.NewShowManager(), store)
	engine.Start()
	defer engine.Close()
	backend := &healthBackendStub{
		audioWarning: "selected endpoint disconnected",
		mixers: []media.AudioMixerMetrics{{
			EndpointID: "usb-interface", Recovering: true, LastCallback: time.Now().Add(-time.Second),
		}},
		displays: []media.VideoDisplay{{ID: "other"}},
	}

	timeline := timecode.NewService(timecode.Config{Source: timecode.SourceInternal}, "")
	defer timeline.Close()
	spare := redundancy.NewService(redundancy.Config{Role: redundancy.RoleOff})
	defer spare.Close()
	collectors := newHealthCollectors(
		engine, backend, timeline, spare,
		func() config.Settings { return settings },
		func() (string, bool) { return "show.cusus", false },
	)
	components := health.CollectAll(collectors...)
	snapshot := health.NewSnapshot(components)
	if snapshot.Overall != health.Failed {
		t.Fatalf("overall health = %s", snapshot.Overall)
	}
	checks := healthPreflightChecks(snapshot, show.Show{}, settings)
	foundAudio, foundOutput := false, false
	for _, check := range checks {
		if strings.Contains(check.Source, "usb-interface") {
			foundAudio = strings.Contains(check.Message, "RECOVERING") && check.Fix != ""
		}
		if strings.Contains(check.Source, "stage") {
			foundOutput = strings.Contains(check.Message, "FAILED") && check.Fix != ""
		}
	}
	if !foundAudio || !foundOutput {
		t.Fatalf("actionable health checks = %+v", checks)
	}
}

func TestOutputHealthReportsEnumerationFailureAsDegraded(t *testing.T) {
	backend := &healthBackendStub{displayErr: errors.New("display service offline")}
	components := outputHealth(backend, config.Settings{OutputSettings: config.OutputSettings{VideoOutputs: []config.VideoOutput{{Stage: "main"}}}})
	if len(components) != 1 || components[0].State != health.Degraded || !strings.Contains(components[0].Summary, "display service offline") {
		t.Fatalf("output health = %+v", components)
	}
}

func TestUnconfirmedDisplayMappingIsInformational(t *testing.T) {
	backend := &healthBackendStub{displays: []media.VideoDisplay{{ID: "main-display"}}}
	components := outputHealth(backend, config.Settings{OutputSettings: config.OutputSettings{VideoOutputs: []config.VideoOutput{{Stage: "main", DisplayID: "main-display"}}}})
	checks := healthPreflightChecks(health.NewSnapshot(components), show.Show{}, config.Defaults())
	if len(checks) != 1 || checks[0].Severity != operatorlog.Info {
		t.Fatalf("display confirmation checks = %+v", checks)
	}
	if got := operatorHealthState(health.NewSnapshot(components)); got != health.Normal {
		t.Fatalf("operator health = %s, want NORMAL", got)
	}
}

func TestDisplayEnumerationFailureRemainsWarning(t *testing.T) {
	backend := &healthBackendStub{displayErr: errors.New("display service offline")}
	components := outputHealth(backend, config.Settings{OutputSettings: config.OutputSettings{VideoOutputs: []config.VideoOutput{{Stage: "main"}}}})
	checks := healthPreflightChecks(health.NewSnapshot(components), show.Show{}, config.Defaults())
	if len(checks) != 1 || checks[0].Severity != operatorlog.Warning {
		t.Fatalf("display enumeration checks = %+v", checks)
	}
	if got := operatorHealthState(health.NewSnapshot(components)); got != health.Degraded {
		t.Fatalf("operator health = %s, want DEGRADED", got)
	}
}

func TestPreviewOnlyAudioRouteFailureDoesNotBlockPlaybackCues(t *testing.T) {
	sound := show.NewSoundCue()
	sound.CueNumber = "1"
	components := []health.Component{{
		ID: "audio-route", Kind: "audio", Name: "Audio routing", State: health.Failed,
		Summary: "The selected preview audio device is disconnected.",
		Details: map[string]any{"previewOnly": true},
	}}
	checks := healthPreflightChecks(health.NewSnapshot(components), show.Show{Cues: []show.Cue{sound}}, config.Defaults())
	if len(checks) != 1 || checks[0].Severity != operatorlog.Warning || len(checks[0].AffectedCues) != 0 {
		t.Fatalf("preview-only audio checks = %#v", checks)
	}
}

func TestTypedAffectedCueIDsDoNotExpandAcrossDuplicateCueNumbers(t *testing.T) {
	first, second := show.NewSoundCue(), show.NewSoundCue()
	first.CueNumber, second.CueNumber = "1", "1"
	components := []health.Component{{
		ID: "audio-endpoint", Kind: "audio", Name: "Endpoint", State: health.Failed, Summary: "Endpoint recovery failed",
		Details: map[string]any{"affectedCues": []string{"1"}, "affectedCueIDs": []show.CueID{first.ID}},
	}}
	checks := healthPreflightChecks(health.NewSnapshot(components), show.Show{Cues: []show.Cue{first, second}}, config.Defaults())
	if len(checks) != 1 || len(checks[0].AffectedCues) != 1 || checks[0].AffectedCues[0] != first.ID {
		t.Fatalf("typed affected cues = %#v", checks)
	}
}

func TestPreviewMixerCueNumbersRemainDiagnosticOnly(t *testing.T) {
	preview := show.NewSoundCue()
	preview.CueNumber = "1"
	components := []health.Component{{
		ID: "audio-preview", Kind: "audio", Name: "Preview endpoint", State: health.Failed, Summary: "Endpoint recovery failed",
		Details: map[string]any{"affectedCues": []string{"1"}, "affectedCueIDs": []show.CueID{}},
	}}
	checks := healthPreflightChecks(health.NewSnapshot(components), show.Show{Cues: []show.Cue{preview}}, config.Defaults())
	if len(checks) != 1 || checks[0].Severity != operatorlog.Warning || len(checks[0].AffectedCues) != 0 {
		t.Fatalf("preview mixer checks = %#v", checks)
	}
}
