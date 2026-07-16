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
