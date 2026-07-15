package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeVideoOutputs(t *testing.T) {
	settings := normalize(Settings{
		MediaSettings: MediaSettings{DefaultMediaOutput: "main"},
		OutputSettings: OutputSettings{VideoOutputs: []VideoOutput{
			{Stage: " main ", Scaling: "invalid", IdleBehavior: "hold", SafeAreaPercent: 99, Layers: 20},
			{Stage: "main", Scaling: "cover"},
		}},
	})
	if len(settings.VideoOutputs) != 1 {
		t.Fatalf("got %d stages, want 1", len(settings.VideoOutputs))
	}
	output := settings.VideoOutputs[0]
	if output.Stage != "main" || output.Scaling != "contain" || output.IdleBehavior != "hold" {
		t.Fatalf("unexpected normalized output: %+v", output)
	}
	if output.Width != 960 || output.Height != 540 || output.ResolutionWidth != 1920 || output.ResolutionHeight != 1080 {
		t.Fatalf("unexpected dimensions: %+v", output)
	}
	if output.SafeAreaPercent != 20 || output.Layers != 8 {
		t.Fatalf("unexpected limits: %+v", output)
	}
}

func TestLockedFullscreenAndRefreshNormalize(t *testing.T) {
	settings := normalize(Settings{MediaSettings: MediaSettings{DefaultMediaOutput: "main"}, OutputSettings: OutputSettings{VideoOutputs: []VideoOutput{{Stage: "main", LockedFullscreen: true, ExpectedRefresh: 5000}}}})
	output := settings.VideoOutputs[0]
	if !output.Fullscreen || output.ExpectedRefresh != 0 {
		t.Fatalf("normalized kiosk output = %#v", output)
	}
}

func TestNormalizeAddsDefaultVideoStage(t *testing.T) {
	settings := normalize(Settings{MediaSettings: MediaSettings{DefaultMediaOutput: "projection"}})
	output := VideoOutputFor(settings, "projection")
	if output.Stage != "projection" || !output.Fullscreen {
		t.Fatalf("default stage not created: %+v", output)
	}
}

func TestUpdateVideoOutputGeometryRejectsUnknownStage(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = store.UpdateVideoOutputGeometry("missing", 1, 2, 640, 480)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unknown stage error = %v", err)
	}
}
