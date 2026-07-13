package config

import "testing"

func TestNormalizeOperatorWindowSupportsEmergencyFallbackSizes(t *testing.T) {
	settings := normalize(Settings{OperatorWindow: WindowPlacement{Width: 100, Height: 100}})
	if settings.OperatorWindow.Width != 480 || settings.OperatorWindow.Height != 320 {
		t.Fatalf("minimum placement = %+v", settings.OperatorWindow)
	}
	settings = normalize(Settings{OperatorWindow: WindowPlacement{Width: 9000, Height: 9000}})
	if settings.OperatorWindow.Width != 7680 || settings.OperatorWindow.Height != 4320 {
		t.Fatalf("maximum placement = %+v", settings.OperatorWindow)
	}
}
