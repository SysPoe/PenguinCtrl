package config

import "testing"

func TestNormalizeVideoOutputs(t *testing.T) {
	settings := normalize(Settings{
		DefaultMediaOutput: "main",
		VideoOutputs: []VideoOutput{
			{Stage: " main ", Scaling: "invalid", IdleBehavior: "hold", SafeAreaPercent: 99, Layers: 20},
			{Stage: "main", Scaling: "cover"},
		},
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

func TestNormalizeAddsDefaultVideoStage(t *testing.T) {
	settings := normalize(Settings{DefaultMediaOutput: "projection"})
	output := VideoOutputFor(settings, "projection")
	if output.Stage != "projection" || !output.Fullscreen {
		t.Fatalf("default stage not created: %+v", output)
	}
}
