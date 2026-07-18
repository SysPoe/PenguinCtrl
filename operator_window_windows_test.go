//go:build windows

package main

import (
	"testing"

	"github.com/syspoe/cusus/config"
)

func TestFitOperatorPlacement(t *testing.T) {
	tests := []struct {
		name      string
		placement config.WindowPlacement
		work      operatorRect
		want      config.WindowPlacement
	}{
		{
			name:      "preserves placement on secondary display",
			placement: config.WindowPlacement{X: -1800, Y: 80, Width: 1300, Height: 720},
			work:      operatorRect{Left: -1920, Top: 0, Right: 0, Bottom: 1040},
			want:      config.WindowPlacement{X: -1800, Y: 80, Width: 1300, Height: 720},
		},
		{
			name:      "recovers placement beyond display edge",
			placement: config.WindowPlacement{X: 2500, Y: 1200, Width: 1300, Height: 720},
			work:      operatorRect{Left: 0, Top: 0, Right: 1920, Bottom: 1040},
			want:      config.WindowPlacement{X: 620, Y: 320, Width: 1300, Height: 720},
		},
		{
			name:      "fits oversized placement to work area",
			placement: config.WindowPlacement{X: -500, Y: -500, Width: 7680, Height: 4320},
			work:      operatorRect{Left: 100, Top: 40, Right: 1700, Bottom: 940},
			want:      config.WindowPlacement{X: 100, Y: 40, Width: 1600, Height: 900},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fitOperatorPlacement(test.placement, test.work); got != test.want {
				t.Fatalf("fitOperatorPlacement() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestScaleOperatorPlacementPreservesLogicalSizeAcrossDPIs(t *testing.T) {
	tests := []struct {
		name      string
		placement config.WindowPlacement
		targetDPI int
		want      config.WindowPlacement
	}{
		{
			name:      "baseline to one hundred fifty percent",
			placement: config.WindowPlacement{X: 20, Y: 30, Width: 1300, Height: 720, DPI: 96},
			targetDPI: 144,
			want:      config.WindowPlacement{X: 20, Y: 30, Width: 1950, Height: 1080, DPI: 144},
		},
		{
			name:      "one hundred fifty percent to baseline",
			placement: config.WindowPlacement{X: 20, Y: 30, Width: 1950, Height: 1080, DPI: 144},
			targetDPI: 96,
			want:      config.WindowPlacement{X: 20, Y: 30, Width: 1300, Height: 720, DPI: 96},
		},
		{
			name:      "legacy placement uses baseline",
			placement: config.WindowPlacement{Width: 1300, Height: 720},
			targetDPI: 192,
			want:      config.WindowPlacement{Width: 2600, Height: 1440, DPI: 192},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := scaleOperatorPlacement(test.placement, test.targetDPI); got != test.want {
				t.Fatalf("scaleOperatorPlacement() = %+v, want %+v", got, test.want)
			}
		})
	}
}
