package main

import (
	"testing"

	"gioui.org/unit"
	"github.com/syspoe/cusus/config"
)

func TestOperatorWindowSizeRestoresLogicalDimensions(t *testing.T) {
	tests := []struct {
		name       string
		placement  config.WindowPlacement
		wantWidth  unit.Dp
		wantHeight unit.Dp
	}{
		{name: "baseline", placement: config.WindowPlacement{Width: 1300, Height: 720, DPI: 96}, wantWidth: 1300, wantHeight: 720},
		{name: "legacy baseline", placement: config.WindowPlacement{Width: 1300, Height: 720}, wantWidth: 1300, wantHeight: 720},
		{name: "two hundred percent", placement: config.WindowPlacement{Width: 2600, Height: 1440, DPI: 192}, wantWidth: 1300, wantHeight: 720},
		{name: "one hundred twenty five percent", placement: config.WindowPlacement{Width: 1625, Height: 900, DPI: 120}, wantWidth: 1300, wantHeight: 720},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			width, height := operatorWindowSize(test.placement)
			if width != test.wantWidth || height != test.wantHeight {
				t.Fatalf("operator window size = %vx%v, want %vx%v", width, height, test.wantWidth, test.wantHeight)
			}
		})
	}
}
