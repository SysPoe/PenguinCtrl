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
	"github.com/syspoe/cusus/playback"
	"github.com/syspoe/cusus/show"
	"github.com/syspoe/cusus/timecode"
)

type healthBackendStub struct {
	audioWarning string
	mixers       []media.AudioMixerMetrics
	displays     []media.VideoDisplay
	displayErr   error
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
func (b *healthBackendStub) Close()                     {}

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
	components := collectHealthComponents(engine, backend, timeline, settings, "show.cusus", false)
	snapshot := health.NewSnapshot(components)
	if snapshot.Overall != health.Failed {
		t.Fatalf("overall health = %s", snapshot.Overall)
	}
	checks := healthPreflightChecks(snapshot)
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
	components := outputHealth(backend, config.Settings{VideoOutputs: []config.VideoOutput{{Stage: "main"}}})
	if len(components) != 1 || components[0].State != health.Degraded || !strings.Contains(components[0].Summary, "display service offline") {
		t.Fatalf("output health = %+v", components)
	}
}
