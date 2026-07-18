package config

import "testing"

func TestNormalizeOperatorWindowSupportsEmergencyFallbackSizes(t *testing.T) {
	settings := normalizeOperatorUI(OperatorUISettings{OperatorWindow: WindowPlacement{Width: 100, Height: 100}})
	if settings.OperatorWindow.Width != 480 || settings.OperatorWindow.Height != 320 {
		t.Fatalf("minimum placement = %+v", settings.OperatorWindow)
	}
	if settings.OperatorWindow.DPI != 96 {
		t.Fatalf("legacy placement DPI = %d, want 96", settings.OperatorWindow.DPI)
	}
	settings = normalizeOperatorUI(OperatorUISettings{OperatorWindow: WindowPlacement{Width: 9000, Height: 9000}})
	if settings.OperatorWindow.Width != 7680 || settings.OperatorWindow.Height != 4320 {
		t.Fatalf("maximum placement = %+v", settings.OperatorWindow)
	}
	settings = normalizeOperatorUI(OperatorUISettings{OperatorWindow: WindowPlacement{Width: 1300, Height: 720, DPI: 1200}})
	if settings.OperatorWindow.DPI != 768 {
		t.Fatalf("clamped placement DPI = %d, want 768", settings.OperatorWindow.DPI)
	}
}
